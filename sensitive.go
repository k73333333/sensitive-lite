package sensitive

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kaidong77/sensitive-lite/internal/core"
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
	dfa *core.DFATree
	// normalizer 文本标准化器
	normalizer *core.Normalizer
	// normToOrig 标准化词 → 原始词的映射（用于结果报告时还原原始敏感词）
	normToOrig map[string]string
	// opts 过滤器配置
	opts *options
	// wordMap 原始敏感词集合（用于懒加载场景暂存词库）
	wordMap map[string]struct{}
	// logger 日志器（nil 时使用 noopLogger 零开销默认实现）
	logger Logger
	// built 标记 DFA 是否已构建完成
	built   bool
	builtMu sync.RWMutex
	// degraded 标记是否已降级（关闭反清洗）
	degraded   bool
	degradedMu sync.RWMutex
	// statsMu 统计字段保护锁
	statsMu sync.RWMutex
	// totalMatches 累计匹配次数（用于监控统计）
	totalMatches int64
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
		normalizer: core.NewNormalizer(core.NormalizerConfig{
			EnableLeet:            opts.enableLeetSpeak,
			EnableCJKInterstitial: true,
			EnableDedup:           opts.enableDedup,
		}),
		opts:       opts,
		wordMap:    make(map[string]struct{}, len(words)),
		normToOrig: make(map[string]string, len(words)),
	}

	// 初始化日志器：优先使用自定义 Logger
	if opts.logger != nil {
		f.logger = opts.logger
	} else if opts.logLevel >= LogLevelOff {
		// 用户显式关闭日志时使用 noopLogger（零开销）
		f.logger = &noopLogger{}
	} else {
		// 未注入自定义 Logger 且未关闭日志时，创建基于标准库的默认日志器
		defaultLog := newDefaultLogger()
		defaultLog.SetLogLevel(opts.logLevel)
		f.logger = defaultLog
	}

	f.logger.Debug("创建过滤器: 词库数量=%d, 反清洗=%v, 懒加载=%v",
		len(words), opts.enableFuzzy, opts.lazyBuild)

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
//
//	单 DFA 设计 — 启用反清洗时直接将敏感词标准化后存入 DFA，
//	匹配时将输入文本同步标准化后匹配。避免了双 DFA 的内存翻倍问题。
//	关闭反清洗时直接使用原始词构建 DFA。
func (f *Filter) build(words []string) {
	f.builtMu.Lock()
	defer f.builtMu.Unlock()

	if f.built {
		return
	}

	f.dfa = core.NewDFATree()

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
	f.dfa = core.NewDFATree()
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
//  2. 若关闭反清洗或已降级，直接匹配原始文本
//  3. 通过位置映射将匹配结果还原到原始文本位置
//  4. 每次操作若配置了溯源回调，异步记录审计日志
func (f *Filter) FindAll(text string) []MatchResult {
	f.ensureBuilt()

	if text == "" {
		return nil
	}

	// 溯源记录起始时间
	startTime := time.Now()
	var results []MatchResult

	// 降级检查：若反清洗模式被降级，回退到精确匹配
	if f.opts.enableFuzzy && f.isDegraded() {
		f.logger.Warn("反清洗已降级，回退到精确匹配模式")
		results = f.exactFindAll(text)
	} else if f.opts.enableFuzzy {
		results = f.fuzzyFindAll(text)
	} else {
		results = f.exactFindAll(text)
	}

	duration := time.Since(startTime)

	// 降级检测：单次匹配耗时超过阈值时触发告警
	if f.opts.degradeConfig.MaxMatchDuration > 0 &&
		duration.Milliseconds() > int64(f.opts.degradeConfig.MaxMatchDuration) {
		f.logger.Warn("匹配耗时超阈值: %dms > %dms, 文本长度=%d",
			duration.Milliseconds(), f.opts.degradeConfig.MaxMatchDuration, len(text))

		// 触发性能告警回调
		if f.opts.alertCallback != nil {
			f.opts.alertCallback(AlertRecord{
				Timestamp: time.Now(),
				Level:     AlertLevelWarn,
				Title:     "匹配耗时超阈值",
				Message: fmt.Sprintf("单次匹配耗时 %dms 超过阈值 %dms，文本长度=%d，命中数=%d",
					duration.Milliseconds(), f.opts.degradeConfig.MaxMatchDuration, len(text), len(results)),
				IsDegraded: f.isDegraded(),
			})
		}
	}

	// 记录溯源日志
	if f.opts.traceCallback != nil {
		f.recordTrace(text, results, duration)
	}

	// 更新统计
	f.statsMu.Lock()
	f.totalMatches += int64(len(results))
	f.statsMu.Unlock()

	return results
}

// exactFindAll 精确匹配模式：在原始文本中直接匹配 DFA
//
// v3.2 优化：使用 MatchAll 直接获取 rune 位置，预计算字节偏移表，
// 消除原来按词逐个 strings.Index 的 O(K×N) 二次扫描开销。
func (f *Filter) exactFindAll(text string) []MatchResult {
	textRunes := []rune(text)
	matches := f.dfa.MatchAll(text, textRunes)

	// 预计算 rune 索引 → 字节偏移映射表（O(N) 一次性计算，后续 O(1) 查询）
	byteOffsets := make([]int, len(textRunes)+1)
	off := 0
	for i, r := range textRunes {
		byteOffsets[i] = off
		off += core.RuneByteLen(r)
	}
	byteOffsets[len(textRunes)] = off // 文本末尾字节偏移

	results := make([]MatchResult, 0, len(matches))
	seen := make(map[string]struct{}) // 按词去重，保留每个词首次出现

	for _, m := range matches {
		if _, ok := seen[m.Word]; ok {
			continue
		}
		seen[m.Word] = struct{}{}

		// EndRune 是含的，End 是排他的，所以取 EndRune+1 位置的字节偏移
		results = append(results, MatchResult{
			Word:  m.Word,
			Start: byteOffsets[m.StartRune],
			End:   byteOffsets[m.EndRune+1],
			Type:  MatchExact,
		})
	}
	return results
}

// fuzzyFindAll 反清洗匹配模式：标准化输入文本后匹配 DFA，再还原位置
//
// v3.2 优化：使用 MatchAll 直接获取匹配在标准化文本中的 rune 位置，
// 通过 PosMap 映射回原始文本字节偏移，消除原来 strings.Index 循环搜索。
// 原来对每个匹配词执行 while 循环 strings.Index 的 O(K×N) 开销完全消除。
func (f *Filter) fuzzyFindAll(text string) []MatchResult {
	// 步骤 1：标准化输入文本（带位置映射）
	normalized := f.normalizer.Normalize(text)
	if normalized.Text == "" {
		return nil
	}

	// 步骤 2：位置感知的 DFA 多模式匹配（直接返回 rune 位置）
	matches := f.dfa.MatchAll(normalized.Text, normalized.Runes)

	results := make([]MatchResult, 0, len(matches))
	// 按 (原始词 + 原始起始字节偏移) 去重，确保同一敏感词在不同位置均被记录
	seen := make(map[string]struct{})

	for _, m := range matches {
		// 步骤 3：还原匹配词到原始敏感词（通过 normToOrig 映射）
		originalWord := m.Word
		if orig, ok := f.normToOrig[m.Word]; ok {
			originalWord = orig
		}

		// 步骤 4：直接通过 PosMap 获取原始文本字节偏移（无需 strings.Index 扫描）
		if m.StartRune >= len(normalized.PosMap) {
			continue
		}
		startByte := normalized.PosMap[m.StartRune]

		// 计算匹配区间在原始文本中的结束字节偏移
		wordRuneLen := m.EndRune - m.StartRune + 1
		endByte := f.calcEndByte(normalized, m.StartRune, wordRuneLen, len(text))

		// 去重 key：词内容 + 原始起始字节位置
		key := originalWord + "|" + itoa(startByte)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
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
func (f *Filter) calcEndByte(norm *core.NormalizedText, normStart, wordRuneLen, textLen int) int {
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
		return norm.OrigBytePos[origRuneIdx] + core.RuneByteLen(norm.OrigRunes[origRuneIdx])
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
// 降级策略 — 系统资源紧张时自动降级保障核心服务
// ============================================================================

// Degrade 手动触发降级（关闭反清洗，仅保留精确匹配）
// 用于运维人员在系统资源紧张时主动降级
func (f *Filter) Degrade() {
	f.degradedMu.Lock()
	alreadyDegraded := f.degraded
	if !f.degraded {
		f.degraded = true
	}
	f.degradedMu.Unlock()

	if !alreadyDegraded {
		f.logger.Warn("反清洗功能已降级，当前仅支持精确匹配")

		// 触发告警回调（通知外部告警系统）
		if f.opts.alertCallback != nil {
			f.opts.alertCallback(AlertRecord{
				Timestamp:  time.Now(),
				Level:      AlertLevelCritical,
				Title:      "反清洗功能已降级",
				Message:    "系统触发降级，反清洗功能已关闭，当前仅支持精确匹配。请检查系统资源状态。",
				IsDegraded: true,
			})
		}
	}
}

// Recover 从降级状态恢复（重新启用反清洗）
func (f *Filter) Recover() {
	f.degradedMu.Lock()
	defer f.degradedMu.Unlock()
	if f.degraded {
		f.degraded = false
		f.logger.Info("反清洗功能已从降级状态恢复")
	}
}

// isDegraded 检查是否处于降级状态
func (f *Filter) isDegraded() bool {
	f.degradedMu.RLock()
	defer f.degradedMu.RUnlock()
	return f.degraded
}

// CheckMemoryAndDegrade 检查内存使用并自动降级/恢复
//
// 传入当前系统可用内存（MB）：
//   - 低于 MaxMemoryMB 阈值时自动触发降级（关闭反清洗）
//   - 恢复到阈值以上时自动恢复反清洗功能
//   - MaxMemoryMB 为 0 时不执行任何自动操作（关闭自动降级）
//
// 手动 Degrade()/Recover() 不受影响，始终生效。
func (f *Filter) CheckMemoryAndDegrade(availableMemMB int) {
	cfg := f.opts.degradeConfig
	// 阈值未配置（0）时不触发任何自动操作
	if cfg.MaxMemoryMB <= 0 {
		return
	}

	// 内存不足 → 自动降级
	if availableMemMB < cfg.MaxMemoryMB {
		f.Degrade()
		f.logger.Warn("内存不足触发自动降级: 可用=%dMB < 阈值=%dMB",
			availableMemMB, cfg.MaxMemoryMB)
		return
	}

	// 内存恢复 → 自动恢复（仅在之前因自动降级触发的情况下）
	if f.isDegraded() {
		f.Recover()
		f.logger.Info("内存恢复，自动恢复反清洗: 可用=%dMB >= 阈值=%dMB",
			availableMemMB, cfg.MaxMemoryMB)
	}
}

// ============================================================================
// 溯源追踪 — 审计日志记录
// ============================================================================

// recordTrace 记录单次过滤操作的溯源日志
func (f *Filter) recordTrace(text string, matches []MatchResult, duration time.Duration) {
	// 构建匹配词列表（最多保留前 3 个用于审计）
	matchWords := make([]string, 0, 3)
	for i, m := range matches {
		if i >= 3 {
			break
		}
		matchWords = append(matchWords, m.Word)
	}

	record := TraceRecord{
		Timestamp:  time.Now(),
		TextHash:   hashText(text),
		TextLen:    len(text),
		MatchCount: len(matches),
		MatchWords: matchWords,
		Duration:   duration,
	}

	// 回调在调用 goroutine 中同步执行，避免异步复杂性
	// 调用方应确保回调函数轻量（不阻塞过滤流程）
	f.opts.traceCallback(record)
}

// ============================================================================
// 监控统计 API
// ============================================================================

// TotalMatches 返回累计过滤命中次数
func (f *Filter) TotalMatches() int64 {
	f.statsMu.RLock()
	defer f.statsMu.RUnlock()
	return f.totalMatches
}

// ============================================================================
// 批量匹配 API — 高并发场景性能优化
// ============================================================================

// FindAllBatch 批量查找多个文本中的敏感词
//
// 适用场景：消息队列批量消费、数据库批量扫描等需要同时检测多条文本的场景
// 相比逐个调用 FindAll，可以共享同一个过滤器实例，减少上下文切换开销
//
// 参数：
//
//	texts - 待检测文本列表
//
// 返回值：每条文本对应的匹配结果列表（顺序与输入一致）
func (f *Filter) FindAllBatch(texts []string) [][]MatchResult {
	f.ensureBuilt()

	if len(texts) == 0 {
		return nil
	}

	results := make([][]MatchResult, len(texts))
	for i, text := range texts {
		if text == "" {
			results[i] = nil
			continue
		}

		// 复用 FindAll 的单文本检测逻辑
		// 共享同一个 f 实例的 DFA 和 normalizer，利用 CPU 缓存命中
		results[i] = f.FindAll(text)
	}
	return results
}

// WordsSnapshot 导出当前活跃敏感词快照
//
// 用途：数据备份、审计合规、词库版本比对
// 注意：返回的是词库快照副本，修改返回切片不影响过滤器内部状态
//
// 返回值：当前敏感词列表（按原始词形式返回）
func (f *Filter) WordsSnapshot() []string {
	f.ensureBuilt()

	// 收集所有原始敏感词
	words := make([]string, 0, len(f.normToOrig))
	if len(f.normToOrig) > 0 {
		// 反清洗模式：从 normToOrig 映射还原原始词
		for _, orig := range f.normToOrig {
			words = append(words, orig)
		}
	} else if f.dfa != nil {
		// 精确匹配模式：DFA 中存储的就是原始词
		// 无法从 DFA 直接提取词列表，返回空切片
		// 如需精确模式的词库快照，请在 New() 时自行保留词库引用
		f.logger.Debug("精确匹配模式下无法导出完整词库快照，返回空列表")
	}

	return words
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// itoa 简易整数转字符串（避免 fmt.Sprintf 的开销，用于构建去重 key）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

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
