package core

import (
	"strings"
	"sync"
	"unicode"
)

// ============================================================================
// 文本标准化器 — 反清洗识别的核心预处理模块
//
// 功能（v3.0 扩展）：
//  1. 组合字符剥离（Combining Marks Stripping）
//  2. 零宽/格式控制字符过滤
//  3. 空白字符过滤
//  4. Emoji + Tag 字符过滤
//  5. Leet Speak 映射
//  6. 标点符号过滤
//  7. CJK 间杂字符剥离
//  8. 形近字/同音字 + 数学字母符号归一化映射
//  9. CJK 连续重复字符压缩（Deduplication）
//  10. 全角字符 → 半角字符转换
//  11. Unicode 大小写折叠
//  12. 位置映射追踪（标准化后位置 → 原始文本位置）
// ============================================================================

// NormalizerConfig 标准化器配置
// 使用结构体避免构造函数参数过多，保持接口可扩展性
type NormalizerConfig struct {
	// EnableLeet 是否启用 Leet Speak 映射
	EnableLeet bool
	// EnableCJKInterstitial 是否启用 CJK 间杂字符剥离
	EnableCJKInterstitial bool
	// EnableDedup 是否启用 CJK 连续重复字符压缩
	// 默认开启，仅作用于输入文本（不影响词库标准化）
	EnableDedup bool
}

// Normalizer 文本标准化器
// 将经过干扰处理的文本还原为可用于 DFA 匹配的标准形式
type Normalizer struct {
	// confusableMap 形近字/同音字映射表：干扰字符 → 标准字符
	// v3.0 合并了数学字母数字符号映射（math alphanum）
	confusableMap map[rune]rune
	// LeetMap Leet Speak 映射表：数字/符号 → 拉丁字母（启用时非 nil）
	LeetMap map[rune]rune
	// CJKInterstitialSkippable 是否启用 CJK 间杂字符剥离
	CJKInterstitialSkippable bool
	// EnableDedup 是否启用 CJK 连续重复字符压缩
	// 启用后 "敏敏敏感词" → "敏感词"
	EnableDedup bool
}

// NewNormalizer 创建标准化器实例
//
// 参数：
//
//	cfg - 标准化器配置（零值表示默认行为：反清洗全开）
//
// 默认行为：
//   - Leet Speak: 关闭（需显式启用）
//   - CJK 间杂剥离: 开启
//   - CJK 重复压缩: 开启
func NewNormalizer(cfg NormalizerConfig) *Normalizer {
	n := &Normalizer{
		confusableMap:            BuildConfusableMapWithMath(),
		CJKInterstitialSkippable: cfg.EnableCJKInterstitial,
		EnableDedup:              cfg.EnableDedup,
	}
	// Leet Speak 映射仅在显式启用时构建（零开销原则）
	if cfg.EnableLeet {
		n.LeetMap = buildLeetMap()
	}
	return n
}

// NormalizedText 标准化后的文本及位置映射
type NormalizedText struct {
	// Text 标准化后的纯文本（已去除干扰字符、完成字符归一化）
	Text string
	// Runes 标准化后文本的 rune 切片
	Runes []rune
	// PosMap 位置映射表：标准化后位置 → 原始文本字节偏移
	// 索引为标准化后文本的 rune 索引，值为原始 rune 的起始字节偏移
	PosMap []int
	// OrigRunes 原始文本的 rune 切片（含干扰字符）
	OrigRunes []rune
	// OrigBytePos 原始 rune 索引 → 原始文本字节偏移
	OrigBytePos []int
}

// normTextPool NormalizedText 对象池（减少高并发场景下的 GC 压力）
var normTextPool = sync.Pool{
	New: func() interface{} {
		return &NormalizedText{}
	},
}

// newNormText 从对象池获取 NormalizedText（复用底层切片）
func newNormText() *NormalizedText {
	return normTextPool.Get().(*NormalizedText)
}

// releaseNormText 归还 NormalizedText 到对象池（重置字段以避免脏数据泄漏）
func releaseNormText(nt *NormalizedText) {
	nt.Text = ""
	nt.Runes = nil
	nt.PosMap = nil
	nt.OrigRunes = nil
	nt.OrigBytePos = nil
	normTextPool.Put(nt)
}

// Normalize 对输入文本进行标准化处理
//
// 处理流程（按顺序）：
//  1. 组合字符剥离（Combining Marks：U+0300-U+036F 等）
//  2. 零宽/格式控制字符过滤（IsZeroWidth：零宽空格、Bidi 覆盖等）
//  3. 空白字符过滤（IsSpace）
//  4. Emoji + Tag 字符过滤（IsEmoji）
//  5. Leet Speak 映射（可选：数字/符号 → 拉丁字母，在标点检测前执行）
//  6. 标点符号过滤（IsPunct，但已被 Leet 映射的字符除外）
//  7. CJK 间杂字符剥离（可选：非 CJK 字符序列嵌在两 CJK 字符间时整段移除）
//  8. 形近字/同音字 + 数学字母符号映射（confusableMap + mathAlphanum）
//  9. CJK 连续重复字符压缩（Dedup："敏敏敏" → "敏"，仅作用于输入文本）
//  10. 全角→半角转换（ToHalfwidth）
//  11. Unicode 大小写折叠（ToLower）
//
// 参数：
//
//	text - 待标准化的原始文本
//
// 返回值：标准化后的文本结构体（含位置映射）
func (n *Normalizer) Normalize(text string) *NormalizedText {
	origRunes := []rune(text)

	// 步骤 0：组合字符剥离（在零宽检测之前）
	// 剥离组合变音标记，实现 NFD → 基础字符的等效转换
	origRunes = StripCombiningMarks(origRunes)

	// 预分配容量（假设大部分字符会被保留）
	normalizedRunes := make([]rune, 0, len(origRunes))
	posMap := make([]int, 0, len(origRunes))
	origBytePos := make([]int, len(origRunes))

	// 计算每个原始 rune 的字节偏移
	byteOffset := 0
	for i, r := range origRunes {
		origBytePos[i] = byteOffset
		byteOffset += RuneByteLen(r)
	}

	// 遍历标准化
	for i, r := range origRunes {
		// 步骤 2：零宽/格式控制字符 → 永远跳过
		if IsZeroWidth(r) {
			continue
		}
		// 步骤 3：空白字符 → 永远跳过
		if unicode.IsSpace(r) {
			continue
		}

		// 步骤 3.5：形近字/同音字 + 数学字母符号映射（在 Emoji/标点过滤之前）
		// 提前应用 confusableMap 确保特殊符号变体（如丁贝符数字➀➁➂、上标数字等）
		// 不会被后续 Emoji/标点分类误过滤，它们映射后是合法的标准字符
		normalized := r
		confusableMapped := false
		if mapped, ok := n.confusableMap[r]; ok {
			normalized = mapped
			confusableMapped = true
		}

		// 以下过滤仅在字符未命中 confusableMap 映射时执行
		// 已映射的字符视为有意义内容，不再按 Emoji/标点过滤
		if !confusableMapped {
			// 步骤 4：Emoji + Tag 字符 → 永远跳过
			if IsEmoji(r) {
				continue
			}

			// 步骤 5：Leet Speak 映射（在标点检查之前，确保 @→a、5→s 不被误删）
			leetMapped := false
			if n.LeetMap != nil {
				if mapped, ok := n.LeetMap[r]; ok {
					normalized = mapped
					leetMapped = true
				}
			}

			// 步骤 6：标点符号过滤
			// 已被 Leet 映射的字符不视为标点（如 @→a 是有意义的字母替换）
			if !leetMapped && n.IsPunct(r) {
				continue
			}
		}

		// 步骤 7：CJK 间杂字符剥离（可选）
		// 检测从当前位置开始的连续非 CJK 字符段，看其是否被两 CJK 包围
		// 已命中 confusableMap 的字符不参与 CJK 间杂剥离（本身是合法语义字符）
		if !confusableMapped && n.CJKInterstitialSkippable && n.IsInCJKInterstitialRun(origRunes, i) {
			continue
		}

		// 步骤 10-11：全角转半角 + 大小写折叠
		normalized = ToHalfwidth(normalized)
		normalized = unicode.ToLower(normalized)

		normalizedRunes = append(normalizedRunes, normalized)
		posMap = append(posMap, origBytePos[i])
	}

	// 步骤 9（后置）：CJK 连续重复字符压缩
	// 在映射/转换之后执行，因为需要完整的 CJK 字符序列
	// 仅作用于输入文本（Normalize），不影响词库标准化（NormalizeWord）
	if n.EnableDedup {
		normalizedRunes = Dedup(normalizedRunes)
		// Dedup 后 rune 数量可能减少，需要同步裁剪 posMap
		// 注意：由于 Dedup 是后置操作，位置映射可能不精确
		// 因此只保留被压缩字符的最后一个位置
		if len(posMap) > len(normalizedRunes) {
			posMap = posMap[:len(normalizedRunes)]
		}
	}

	return &NormalizedText{
		Text:        string(normalizedRunes),
		Runes:       normalizedRunes,
		PosMap:      posMap,
		OrigRunes:   origRunes,
		OrigBytePos: origBytePos,
	}
}

// ============================================================================
// 字符分类与转换
// ============================================================================

// isSkippable 判断字符是否可跳过（干扰字符）
// 跳过规则：空白字符、零宽字符、标点符号、Emoji、非语言符号
// 重点覆盖反清洗常见手段：零宽空格(U+200B)、零宽非连接符(U+200C)、BOM(U+FEFF)等
func (n *Normalizer) isSkippable(r rune) bool {
	// 零宽/不可见格式字符（反清洗核心向量，Cf 类别）
	// 这些字符在视觉上完全不可见，攻击者常用来拆分敏感词
	if IsZeroWidth(r) {
		return true
	}
	// 空白字符：空格、换行、制表符等
	if unicode.IsSpace(r) {
		return true
	}
	// 标点符号：，。！？""''；：、【】等
	if unicode.IsPunct(r) {
		return true
	}
	// 广义标点（含全角标点）
	if unicode.Is(unicode.Po, r) || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r) {
		return true
	}
	// 各类分隔符
	if unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || unicode.Is(unicode.Zs, r) {
		return true
	}
	// Emoji 范围（含变体选择器）
	if IsEmoji(r) {
		return true
	}
	return false
}

// IsZeroWidth 检测零宽/不可见格式字符（导出供测试访问）
// 覆盖 Unicode Cf（格式控制）类别、Bidi 覆盖字符及常见的反清洗分隔符
func IsZeroWidth(r rune) bool {
	// 零宽空格 — ZERO WIDTH SPACE（最常见的反清洗分隔符）
	if r == 0x200B {
		return true
	}
	// 零宽非连接符 — ZERO WIDTH NON-JOINER
	if r == 0x200C {
		return true
	}
	// 软连字符 — SOFT HYPHEN（不可见换行提示符）
	if r == 0x00AD {
		return true
	}
	// 词连接符 — WORD JOINER（不可见的分隔控制符）
	if r == 0x2060 {
		return true
	}
	// 从左到右标记 / 从右到左标记（文字方向控制符）
	if r == 0x200E || r == 0x200F {
		return true
	}
	// 零宽非断空格 — ZERO WIDTH NO-BREAK SPACE / BOM 反转
	if r == 0xFEFF || r == 0xFFFE {
		return true
	}
	// 蒙古文元音分隔符
	if r == 0x180E {
		return true
	}
	// 不可见的分隔控制符 U+2061-U+2064
	if r >= 0x2061 && r <= 0x2064 {
		return true
	}
	// 行分隔符 / 段分隔符
	if r == 0x2028 || r == 0x2029 {
		return true
	}

	// --- Bidi 覆盖字符（双向文本攻击） ---
	// 攻击者可利用这些字符改变文本渲染顺序，隐藏真实内容
	// U+202A LEFT-TO-RIGHT EMBEDDING
	// U+202B RIGHT-TO-LEFT EMBEDDING
	// U+202C POP DIRECTIONAL FORMATTING
	// U+202D LEFT-TO-RIGHT OVERRIDE
	// U+202E RIGHT-TO-LEFT OVERRIDE
	if r >= 0x202A && r <= 0x202E {
		return true
	}
	// U+2066 LEFT-TO-RIGHT ISOLATE
	// U+2067 RIGHT-TO-LEFT ISOLATE
	// U+2068 FIRST STRONG ISOLATE
	// U+2069 POP DIRECTIONAL ISOLATE
	if r >= 0x2066 && r <= 0x2069 {
		return true
	}

	// 通用 Cf 类别格式字符（涵盖所有未被上述特定检测覆盖的格式控制字符）
	if unicode.Is(unicode.Cf, r) {
		return true
	}
	return false
}

// normalizeRune 对单个字符执行形近字/同音字映射
func (n *Normalizer) normalizeRune(r rune) rune {
	if mapped, ok := n.confusableMap[r]; ok {
		return mapped
	}
	return r
}

// ============================================================================
// 全角 ↔ 半角转换
// ============================================================================

// ToHalfwidth 全角字符转半角（导出供测试访问）
// 转换范围：
//   - 全角标点/字母/数字 FF01-FF5E → 0021-007E（偏移量 0xFEE0）
//   - 全角空格 3000 → 0020
//   - 全角假名 FF61-FF9F 本身已是半角形式，无需转换
func ToHalfwidth(r rune) rune {
	// 全角空格特殊处理
	if r == 0x3000 {
		return ' '
	}
	// 全角字符转半角：FF01(！)-FF5E(～) → 0021(!)-007E(~)
	// 偏移量固定为 0xFEE0，涵盖全角标点、数字、大小写字母
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	return r
}

// ============================================================================
// Emoji 检测
// ============================================================================

// IsEmoji 判断字符是否属于 Emoji 或 Unicode Tag 范围（导出供测试访问）
// 涵盖：表情符号、杂项符号、交通标志、补充符号、Tag 字符等
func IsEmoji(r rune) bool {
	// 表情符号 U+1F600-U+1F64F
	if r >= 0x1F600 && r <= 0x1F64F {
		return true
	}
	// 杂项符号和象形文字 U+1F300-U+1F5FF
	if r >= 0x1F300 && r <= 0x1F5FF {
		return true
	}
	// 交通和地图符号 U+1F680-U+1F6FF
	if r >= 0x1F680 && r <= 0x1F6FF {
		return true
	}
	// 补充符号和象形文字 U+1F900-U+1F9FF
	if r >= 0x1F900 && r <= 0x1F9FF {
		return true
	}
	// 装饰符号 U+2700-U+27BF
	if r >= 0x2700 && r <= 0x27BF {
		return true
	}
	// 杂项符号 U+2600-U+26FF
	if r >= 0x2600 && r <= 0x26FF {
		return true
	}
	// 杂项符号和箭头 U+2B00-U+2BFF
	if r >= 0x2B00 && r <= 0x2BFF {
		return true
	}
	// 变体选择器
	if r == 0xFE0F || r == 0xFE0E {
		return true
	}
	// 零宽连接符（ZWJ）
	if r == 0x200D {
		return true
	}

	// --- Unicode Tag 字符（U+E0001, U+E0020-U+E007F） ---
	// Tag 字符是隐藏的元数据标签，攻击者可利用作为不可见分隔符
	// U+E0001 LANGUAGE TAG
	if r == 0xE0001 {
		return true
	}
	// U+E0020-U+E007F TAG SPACE ~ CANCEL TAG
	if r >= 0xE0020 && r <= 0xE007F {
		return true
	}

	return false
}

// ============================================================================
// 构建敏感词标准化版本
// ============================================================================

// NormalizeWord 对单个敏感词进行标准化处理
// 将词库中的敏感词也做标准化，以便与标准化后的输入文本匹配
//
// 注意：NormalizeWord 不执行 CJK dedup，
// 因为词库中的敏感词是标准形式，不应被去重。
// 仅在 Normalize（输入文本标准化）中执行 dedup。
func (n *Normalizer) NormalizeWord(word string) string {
	var sb strings.Builder
	sb.Grow(len(word))
	runes := []rune(word)

	// 步骤 0：组合字符剥离
	runes = StripCombiningMarks(runes)

	for i, r := range runes {
		// 零宽/格式控制 → 跳过
		if IsZeroWidth(r) {
			continue
		}
		// 空白字符 → 跳过
		if unicode.IsSpace(r) {
			continue
		}

		// 形近字/同音字 + 数学字母符号映射（在 Emoji/标点过滤之前）
		// 与 Normalize 保持一致：先映射再判断过滤，防止特殊符号变体被误过滤
		normalized := r
		confusableMapped := false
		if mapped, ok := n.confusableMap[r]; ok {
			normalized = mapped
			confusableMapped = true
		}

		if !confusableMapped {
			// Emoji + Tag → 跳过
			if IsEmoji(r) {
				continue
			}

			// Leet Speak 映射（在标点检查之前）
			leetMapped := false
			if n.LeetMap != nil {
				if mapped, ok := n.LeetMap[r]; ok {
					normalized = mapped
					leetMapped = true
				}
			}

			// 标点符号过滤
			if !leetMapped && n.IsPunct(r) {
				continue
			}
		}

		// CJK 间杂字符剥离
		// 已命中 confusableMap 的字符不参与剥离（本身是合法语义字符）
		if !confusableMapped && n.CJKInterstitialSkippable && n.IsInCJKInterstitialRun(runes, i) {
			continue
		}

		// 全角转半角 + 大小写折叠
		normalized = ToHalfwidth(normalized)
		normalized = unicode.ToLower(normalized)
		sb.WriteRune(normalized)
	}

	// 注意：词库标准化不执行 CJK dedup
	// dedup 仅在 Normalize（输入文本）中执行
	return sb.String()
}

// ============================================================================
// CJK 间杂字符检测 — 数字/符号嵌入中文逃避检测
//
// 攻击场景：在敏感词的 CJK 字符之间嵌入数字、拉丁字母等非 CJK 字符，
// 以绕过关键词检测。例如：
//
//	"敏1感词"  → 期望匹配 "敏感词"
//	"敏a感b词" → 期望匹配 "敏感词"
//
// 策略：只有当非 CJK 字符被前后两个 CJK 字符"包围"时，才将其视为
// 逃避检测的分隔符并剥离。独立的非 CJK 字符（如英文单词、数字编号）
// 不受影响。
// ============================================================================

// IsCJK 判断字符是否为 CJK 统一表意文字（导出供测试访问）
// 涵盖常用汉字范围：CJK Unified Ideographs（U+4E00-U+9FFF）
// 以及扩展 A 区（U+3400-U+4DBF）
func IsCJK(r rune) bool {
	if r >= 0x4E00 && r <= 0x9FFF {
		return true
	}
	if r >= 0x3400 && r <= 0x4DBF {
		return true
	}
	return false
}

// IsPunct 判断字符是否为标点符号（导出供测试访问）
// 分离自 isSkippable，用于 Leet Speak 场景下精准控制过滤时机
func (n *Normalizer) IsPunct(r rune) bool {
	// 标准标点符号类别
	if unicode.IsPunct(r) {
		return true
	}
	// 非间距修饰符（覆盖全角/半角标点变体）
	if unicode.Is(unicode.Po, r) || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r) {
		return true
	}
	// 行分隔符 / 段分隔符
	if unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || unicode.Is(unicode.Zs, r) {
		return true
	}
	// 斜杠 / 反斜杠（常见反清洗分隔符）
	if r == '/' || r == '\\' {
		return true
	}
	return false
}

// ============================================================================
// CJK 间杂字符检测 — 改进版（支持连续多字符间杂段）
// ============================================================================

// IsInCJKInterstitialRun 判断当前位置是否处于 CJK 间杂字符段中（导出供测试访问）
//
// 适用场景：当多个非 CJK 字符被嵌在两个 CJK 字符之间时，
// 仅短序列（≤3 个非 CJK 字符）视为逃避检测的间隔符。
// 超过 3 个字符的序列可能是正常的拉丁单词，不应剥离。
//
// 算法：
//  1. 确认当前字符不是 CJK
//  2. 逆转查找前一个 CJK 字符
//  3. 正转查找后一个 CJK 字符
//  4. 统计前后 CJK 之间的非 CJK 字符数量
//  5. 仅当数量 ≤ 3 且前后都是 CJK → 应剥离
func (n *Normalizer) IsInCJKInterstitialRun(runes []rune, idx int) bool {
	r := runes[idx]
	// 当前位置是 CJK → 不是间杂字符
	if IsCJK(r) {
		return false
	}

	// 逆向前查找：找到前一个 CJK 字符的索引
	prevCJKIdx := -1
	for j := idx - 1; j >= 0; j-- {
		if IsCJK(runes[j]) {
			prevCJKIdx = j
			break
		}
		// 零宽/空格/Emoji/组合标记 不影响 CJK 邻居判断，继续前找
		if IsZeroWidth(runes[j]) || unicode.IsSpace(runes[j]) || IsEmoji(runes[j]) || isCombiningMark(runes[j]) {
			continue
		}
	}
	// 前面没有 CJK 字符 → 不在间杂段中
	if prevCJKIdx < 0 {
		return false
	}

	// 正向后查找：找到后一个 CJK 字符的索引
	nextCJKIdx := -1
	for j := idx + 1; j < len(runes); j++ {
		if IsCJK(runes[j]) {
			nextCJKIdx = j
			break
		}
		if IsZeroWidth(runes[j]) || unicode.IsSpace(runes[j]) || IsEmoji(runes[j]) || isCombiningMark(runes[j]) {
			continue
		}
	}
	// 后面没有 CJK 字符 → 不在间杂段中
	if nextCJKIdx < 0 {
		return false
	}

	// 统计前后两个 CJK 字符之间的非 CJK/非格式化字符数量
	interstitialLen := 0
	for j := prevCJKIdx + 1; j < nextCJKIdx; j++ {
		rj := runes[j]
		// 零宽/空格/Emoji/组合标记 不计入间杂长度（它们已经是分离器）
		if IsCJK(rj) || IsZeroWidth(rj) || unicode.IsSpace(rj) || IsEmoji(rj) || isCombiningMark(rj) {
			continue
		}
		interstitialLen++
	}

	// 间杂字符数 ≤ 3：视为逃避检测的分隔符 → 应剥离
	// 间杂字符数 > 3：可能是正常的拉丁单词/缩写 → 保留
	return interstitialLen <= 3
}

// ============================================================================
// 辅助函数
// ============================================================================

// RuneByteLen 计算 rune 的 UTF-8 编码字节数（导出供 sensitive.go 调用）
func RuneByteLen(r rune) int {
	if r <= 0x7F {
		return 1
	}
	if r <= 0x7FF {
		return 2
	}
	if r <= 0xFFFF {
		return 3
	}
	return 4
}

// ============================================================================
// 合并 confusable 和 math alphanum 映射表
// ============================================================================

// BuildConfusableMapWithMath 构建合并后的完整映射表（导出供测试访问）
// 包含原有形近字/同音字映射 + 数学字母数字符号映射
func BuildConfusableMapWithMath() map[rune]rune {
	// 先构建基础 confusable 映射
	base := buildConfusableMap()
	// 合并数学字母数字符号映射（math alphanum → ASCII）
	mathMap := BuildMathAlphanumMap()
	for k, v := range mathMap {
		// confusable 映射优先（不覆盖已存在的映射）
		if _, exists := base[k]; !exists {
			base[k] = v
		}
	}
	return base
}
