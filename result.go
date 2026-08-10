package sensitive

import "unicode/utf8"

// MatchType 匹配类型枚举
// 用于区分精确匹配与反清洗匹配的结果类型
type MatchType uint8

const (
	// MatchExact 精确匹配 — 敏感词直接出现在原文中
	MatchExact MatchType = iota
	// MatchFuzzy 模糊匹配 — 经过文本干扰处理后识别出的敏感词
	MatchFuzzy
)

// String 返回匹配类型的可读描述
func (m MatchType) String() string {
	switch m {
	case MatchExact:
		return "exact"
	case MatchFuzzy:
		return "fuzzy"
	default:
		return "unknown"
	}
}

// MatchResult 单次敏感词匹配结果
// 包含匹配到的敏感词原文、在输入文本中的位置区间及匹配类型
type MatchResult struct {
	// Word 匹配到的原始敏感词（来自词库）
	Word string
	// Start 匹配内容在原文中的起始字节偏移
	Start int
	// End 匹配内容在原文中的结束字节偏移（不含）
	End int
	// Type 匹配类型：精确匹配或反清洗识别
	Type MatchType
}

// FilterResult 过滤操作的完整结果
// 包含替换后的文本以及所有命中敏感词的详细信息
type FilterResult struct {
	// Text 经过替换处理后的文本
	Text string
	// Matches 所有命中的敏感词匹配详情
	Matches []MatchResult
	// Count 命中敏感词总数
	Count int
}

// strLen 计算字符串的 rune 长度
func strLen(s string) int {
	return utf8.RuneCountInString(s)
}
