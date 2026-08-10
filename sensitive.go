package sensitive

import (
	"strings"
	"sync"
)

// ============================================================================
// Filter — 敏感词过滤器（对外公开 API）
//
// 组件定位：
//   完全独立的通用敏感词过滤工具，不内置任何预设敏感词库。
//   用户通过 New() 传入自定义敏感词列表完成初始化。
//
// 核心特性：
//   - DFA 多模式匹配（O(n) 时间复杂度，n = 文本长度）
//   - 反清洗识别（空格拆分、形近字替换、特殊字符干扰等）
//     → 单 DFA 架构：仅存储标准化后的敏感词，匹配时同时标准化输入文本
//   - 低内存消耗（单 DFA 设计，10 万词仅需约 30MB）
//   - 并发安全（读操作无锁，构建完成后线程安全）
// ============================================================================

// Filter 敏感词过滤器
// 构建完成后为只读状态，多个 goroutine 可并发调用 FindAll / Replace 等方法
type Filter struct {
	// dfa 核心 DFA 匹配引擎
	// 当启用反清洗时存储标准化后的敏感词；否则存储原始敏感词
	dfa *DFATree
	// normalizer 文本标准化器
	normalizer *Normalizer
	// normToOrig 标准化词 → 原始词的映射（用于结果报告时还原原始敏感词）
	normToOrig map[string]string
	// opts 过滤器配置
	opts *options
	// wordMap 原始敏感词集合（用于懒加载场景暂存词库）
	wordMap map[string]struct{}
	// built 标记 DFA 是否已构建完成
	built   bool
	builtMu sync.RWMutex
}

// New 创建敏感词过滤器实例
//
// 参数：
//
//	words    - 用户自定义敏感词列表（必填，组件不内置任何词库）
//	optFns   - 可选配置函数（WithFuzzy、WithReplacement 等）
//
// 返回值：过滤器实例
//
// 使用示例：
//
//	filter := sensitive.New(
//	    []string{"敏感词1", "敏感词2"},
//	    sensitive.WithFuzzy(true),
//	)
func New(words []string, optFns ...Option) *Filter {
	// 合并配置
	opts := defaultOptions()
	for _, fn := range optFns {
		fn(opts)
	}

	f := &Filter{
		normalizer: NewNormalizer(),
		opts:       opts,
		wordMap:    make(map[string]struct{}, len(words)),
		normToOrig: make(map[string]string, len(words)),
	}

	// 懒加载模式：延迟到首次过滤调用时才构建 DFA
	if !opts.lazyBuild {
		f.build(words)
	} else {
		// 存储词库副本供后续懒构建使用
		for _, w := range words {
			w = strings.TrimSpace(w)
			if w != "" {
				f.wordMap[w] = struct{}{}
			}
		}
	}

	return f
}

// build 构建 DFA 匹配引擎
//
// 架构说明：
//  单 DFA 设计 — 启用反清洗时直接将敏感词标准化后存入 DFA，
//  匹配时将输入文本同步标准化后匹配。避免了双 DFA 的内存翻倍问题。
//  关闭反清洗时直接使用原始词构建 DFA。
func (f *Filter) build(words []string) {
	f.builtMu.Lock()
	defer f.builtMu.Unlock()

	if f.built {
		return
	}

	f.dfa = NewDFATree()

	for _, w := range words {
		// 跳过空词
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}

		// 确定存入 DFA 的词
		var dfaWord string
		if f.opts.enableFuzzy {
			// 反清洗模式：存入标准化后的敏感词
			dfaWord = f.normalizer.NormalizeWord(w)
			if dfaWord == "" {
				continue
			}
			// 记录标准化词 → 原始词的映射（用于结果报告）
			// 若多个原始词归一化到相同标准词，保留第一个
			if _, exists := f.normToOrig[dfaWord]; !exists {
				f.normToOrig[dfaWord] = w
			}
		} else {
			// 精确匹配模式：直接存入原始词
			dfaWord = w
		}

		f.dfa.Insert(dfaWord, f.opts.maxWordLen)
	}
	f.built = true
}

// ensureBuilt 确保 DFA 已构建（懒加载时在首次调用时触发）
func (f *Filter) ensureBuilt() {
	f.builtMu.RLock()
	if f.built {
		f.builtMu.RUnlock()
		return
	}
	f.builtMu.RUnlock()

	// 需要构建，从 wordMap 中提取词库
	f.builtMu.Lock()
	defer f.builtMu.Unlock()
	if f.built {
		return // 双重检查
	}

	words := make([]string, 0, len(f.wordMap))
	for w := range f.wordMap {
		words = append(words, w)
	}
	// 清空暂存词库
	f.wordMap = nil

	// 构建 DFA
	f.dfa = NewDFATree()
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}

		var dfaWord string
		if f.opts.enableFuzzy {
			dfaWord = f.normalizer.NormalizeWord(w)
			if dfaWord == "" {
				continue
			}
			if _, exists := f.normToOrig[dfaWord]; !exists {
				f.normToOrig[dfaWord] = w
			}
		} else {
			dfaWord = w
		}
		f.dfa.Insert(dfaWord, f.opts.maxWordLen)
	}
	f.built = true
}

// ============================================================================
// 核心过滤 API
// ============================================================================

// FindAll 查找文本中的所有敏感词
//
// 参数：
//
//	text - 待检测文本
//
// 返回值：所有命中的敏感词匹配结果列表
// 如果未命中任何敏感词，返回空切片
//
// 匹配策略：
//  1. 若启用反清洗，先标准化输入文本再匹配 DFA
//  2. 若关闭反清洗，直接匹配原始文本
//  3. 通过位置映射将匹配结果还原到原始文本位置
func (f *Filter) FindAll(text string) []MatchResult {
	f.ensureBuilt()

	if text == "" {
		return nil
	}

	if f.opts.enableFuzzy {
		return f.fuzzyFindAll(text)
	}
	return f.exactFindAll(text)
}

// exactFindAll 精确匹配模式：在原始文本中直接匹配 DFA
func (f *Filter) exactFindAll(text string) []MatchResult {
	rawMatches := f.dfa.Match(text, nil)

	results := make([]MatchResult, 0, len(rawMatches))
	seen := make(map[string]struct{})

	for _, word := range rawMatches {
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}

		// 在原始文本中定位该敏感词
		idx := strings.Index(text, word)
		if idx >= 0 {
			results = append(results, MatchResult{
				Word:  word,
				Start: idx,
				End:   idx + len(word),
				Type:  MatchExact,
			})
		}
	}
	return results
}

// fuzzyFindAll 反清洗匹配模式：标准化输入文本后匹配 DFA，再还原位置
func (f *Filter) fuzzyFindAll(text string) []MatchResult {
	// 步骤 1：标准化输入文本（带位置映射）
	normalized := f.normalizer.Normalize(text)
	if normalized.Text == "" {
		return nil
	}

	// 步骤 2：在标准化文本上执行 DFA 多模式匹配
	rawMatches := f.dfa.Match(normalized.Text, normalized.Runes)

	results := make([]MatchResult, 0, len(rawMatches))
	seen := make(map[string]struct{})

	for _, word := range rawMatches {
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}

		// 步骤 3：还原匹配词到原始敏感词（通过 normToOrig 映射）
		originalWord := word
		if orig, ok := f.normToOrig[word]; ok {
			originalWord = orig
		}

		// 步骤 4：在标准化文本中定位，再映射回原始文本位置
		normIdx := strings.Index(normalized.Text, word)
		if normIdx < 0 {
			continue
		}

		normRuneIdx := byteOffsetToRuneIndex(normalized.Text, normIdx)
		wordRuneLen := len([]rune(word))

		// 通过位置映射还原原始字节偏移
		if normRuneIdx < len(normalized.PosMap) {
			startByte := normalized.PosMap[normRuneIdx]

			// 计算结束字节偏移
			endByte := f.calcEndByte(normalized, normRuneIdx, wordRuneLen, len(text))

			results = append(results, MatchResult{
				Word:  originalWord,
				Start: startByte,
				End:   endByte,
				Type:  MatchFuzzy,
			})
		}
	}
	return results
}

// calcEndByte 计算匹配区间在原始文本中的结束字节偏移
func (f *Filter) calcEndByte(norm *NormalizedText, normStart, wordRuneLen, textLen int) int {
	endNormRune := normStart + wordRuneLen - 1
	if endNormRune >= len(norm.PosMap) {
		return textLen
	}

	// 找到标准化文本中最后一个匹配 rune 对应的原始 rune 索引
	byteOff := norm.PosMap[endNormRune]
	origRuneIdx := -1
	for i, off := range norm.OrigBytePos {
		if off == byteOff {
			origRuneIdx = i
			break
		}
	}

	if origRuneIdx >= 0 && origRuneIdx < len(norm.OrigRunes) {
		// 原始 rune 的字节开始 + 该 rune 的字节长度 = 结束字节偏移
		return norm.OrigBytePos[origRuneIdx] + runeByteLen(norm.OrigRunes[origRuneIdx])
	}
	return textLen
}

// Replace 替换文本中的敏感词
//
// 参数：
//
//	text - 待处理文本
//
// 返回值：FilterResult 包含替换后的文本及匹配详情
func (f *Filter) Replace(text string) *FilterResult {
	f.ensureBuilt()

	matches := f.FindAll(text)
	if len(matches) == 0 {
		return &FilterResult{Text: text, Matches: nil, Count: 0}
	}

	// 从后往前替换避免偏移问题
	runes := []rune(text)
	for i := len(matches) - 1; i >= 0; i-- {
		m := &matches[i]
		startRune := byteOffsetToRuneIndex(text, m.Start)
		endRune := byteOffsetToRuneIndex(text, m.End)
		for j := startRune; j < endRune; j++ {
			runes[j] = f.opts.replacement
		}
	}

	return &FilterResult{
		Text:    string(runes),
		Matches: matches,
		Count:   len(matches),
	}
}

// Contains 快速判断文本是否包含任意敏感词
// 比 FindAll 更高效，命中一个即返回
func (f *Filter) Contains(text string) bool {
	f.ensureBuilt()

	if text == "" {
		return false
	}

	if f.opts.enableFuzzy {
		normalized := f.normalizer.Normalize(text)
		return f.dfa.Contains(normalized.Text)
	}
	return f.dfa.Contains(text)
}

// ============================================================================
// 统计与工具 API
// ============================================================================

// Stats 返回过滤器统计信息
func (f *Filter) Stats() map[string]int {
	f.ensureBuilt()

	stats := make(map[string]int, 2)
	if f.dfa != nil {
		wordCount, nodeCount := f.dfa.Stats()
		stats["words"] = wordCount
		stats["nodes"] = nodeCount
	}
	stats["original_words"] = len(f.normToOrig)
	if len(f.normToOrig) == 0 && f.dfa != nil {
		// 精确匹配模式：wordCount 即为原始词数
		wc, _ := f.dfa.Stats()
		stats["original_words"] = wc
	}
	return stats
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// byteOffsetToRuneIndex 将字节偏移转换为 rune 索引
func byteOffsetToRuneIndex(s string, byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff >= len(s) {
		return strLen(s)
	}
	count := 0
	for i := range s {
		if i >= byteOff {
			return count
		}
		count++
	}
	return count
}
