package sensitive

import (
	"strings"
	"unicode"
)

// ============================================================================
// 文本标准化器 — 反清洗识别的核心预处理模块
//
// 功能：
//  1. 全角字符 → 半角字符转换（字母、数字、标点）
//  2. Unicode 大小写折叠
//  3. 形近字/同音字归一化映射
//  4. 空白字符、特殊符号、Emoji 剥离
//  5. 位置映射追踪（标准化后位置 → 原始文本位置）
// ============================================================================

// Normalizer 文本标准化器
// 将经过干扰处理的文本还原为可用于 DFA 匹配的标准形式
type Normalizer struct {
	// confusableMap 形近字/同音字映射表：干扰字符 → 标准字符
	confusableMap map[rune]rune
}

// NewNormalizer 创建标准化器实例
// 初始化时会加载内置的形近字/同音字映射表
func NewNormalizer() *Normalizer {
	return &Normalizer{
		confusableMap: buildConfusableMap(),
	}
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

// Normalize 对输入文本进行标准化处理
//
// 处理流程：
//  1. 遍历原始文本的每个 rune
//  2. 判断 rune 类型：保留字符 / 干扰字符 / 需转换字符
//  3. 对保留字符执行大小写折叠和全半角转换
//  4. 对干扰字符应用形近字映射
//  5. 构建位置映射表
//
// 参数：
//
//	text - 待标准化的原始文本
//
// 返回值：标准化后的文本结构体（含位置映射）
func (n *Normalizer) Normalize(text string) *NormalizedText {
	origRunes := []rune(text)
	// 预分配容量（假设大部分字符会被保留）
	normalizedRunes := make([]rune, 0, len(origRunes))
	posMap := make([]int, 0, len(origRunes))
	origBytePos := make([]int, len(origRunes))

	// 计算每个原始 rune 的字节偏移
	byteOffset := 0
	for i, r := range origRunes {
		origBytePos[i] = byteOffset
		byteOffset += runeByteLen(r)
	}

	// 遍历标准化
	for i, r := range origRunes {
		// 步骤 1：判断是否为可跳过字符（空格、特殊符号、Emoji 等）
		if n.isSkippable(r) {
			continue
		}

		// 步骤 2：应用形近字/同音字映射
		normalized := n.normalizeRune(r)

		// 步骤 3：全角转半角 + 大小写折叠
		normalized = toHalfwidth(normalized)
		normalized = unicode.ToLower(normalized)

		normalizedRunes = append(normalizedRunes, normalized)
		posMap = append(posMap, origBytePos[i])
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
// 跳过规则：空格/换行/制表符、标点符号、Emoji、非语言符号
func (n *Normalizer) isSkippable(r rune) bool {
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
	if isEmoji(r) {
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

// toHalfwidth 全角字符转半角
// 转换范围：
//   - 全角字母 FF21-FF3A → 0041-005A (Ａ-Ｚ → A-Z)
//   - 全角字母 FF41-FF5A → 0061-007A (ａ-ｚ → a-z)
//   - 全角数字 FF10-FF19 → 0030-0039 (０-９ → 0-9)
//   - 全角空格 3000 → 0020
func toHalfwidth(r rune) rune {
	// 全角空格特殊处理
	if r == 0x3000 {
		return ' '
	}
	// 全角字母数字：FF01-FF5E → 0021-007E（偏移量固定）
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	// 全角假名范围 FF61-FF9F → 半角假名
	if r >= 0xFF61 && r <= 0xFF9F {
		return halfwidthKatakanaMap[r]
	}
	return r
}

// halfwidthKatakanaMap 全角假名 → 半角假名映射
// 仅覆盖常用范围 FF61-FF9F
var halfwidthKatakanaMap = func() map[rune]rune {
	m := make(map[rune]rune, 63)
	// 半角片假名映射表（FF61-FF9F → 对应的半角形式）
	// 实际场景中假名干扰少见，此处做基础覆盖
	base := rune(0xFF61)
	for i := rune(0); i < 63; i++ {
		m[base+i] = base + i
	}
	return m
}()

// ============================================================================
// Emoji 检测
// ============================================================================

// isEmoji 判断字符是否属于 Emoji 范围
// 涵盖：表情符号、杂项符号、交通标志、补充符号等
func isEmoji(r rune) bool {
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
	return false
}

// ============================================================================
// 构建敏感词标准化版本
// ============================================================================

// NormalizeWord 对单个敏感词进行标准化处理
// 将词库中的敏感词也做标准化，以便与标准化后的输入文本匹配
func (n *Normalizer) NormalizeWord(word string) string {
	var sb strings.Builder
	sb.Grow(len(word))
	for _, r := range word {
		if n.isSkippable(r) {
			continue
		}
		normalized := n.normalizeRune(r)
		normalized = toHalfwidth(normalized)
		normalized = unicode.ToLower(normalized)
		sb.WriteRune(normalized)
	}
	return sb.String()
}

// ============================================================================
// 辅助函数
// ============================================================================

// runeByteLen 计算 rune 的 UTF-8 编码字节数
func runeByteLen(r rune) int {
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
