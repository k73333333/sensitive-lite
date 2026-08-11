package main

import (
	"fmt"

	sensitive "github.com/sensitive-lite/sensitive-lite"
)

// ============================================================================
// 示例 1：基础敏感词精确匹配
//
// 场景：用户评论敏感词检测，仅需精确匹配
// 使用方法：
//
//	go run examples/basic/main.go
// ============================================================================

func main() {
	// 步骤 1：准备自定义敏感词库（组件不内置任何词库）
	words := []string{
		"敏感词",
		"违规",
		"广告",
		"欺诈",
		"垃圾信息",
	}

	// 步骤 2：创建过滤器（关闭反清洗以提升性能）
	filter := sensitive.New(words, sensitive.WithFuzzy(false))

	// 步骤 3：检测文本是否包含敏感词
	text := "这是一条包含敏感词和违规内容的评论"
	fmt.Printf("输入文本: %s\n", text)

	// FindAll — 查找所有匹配
	matches := filter.FindAll(text)
	fmt.Printf("匹配数量: %d\n", len(matches))
	for _, m := range matches {
		fmt.Printf("  - 敏感词: %s, 位置: [%d, %d), 类型: %s\n",
			m.Word, m.Start, m.End, m.Type)
	}

	// Replace — 替换敏感词
	result := filter.Replace(text)
	fmt.Printf("替换后: %s\n", result.Text)

	// Contains — 快速判断
	fmt.Printf("包含敏感词: %t\n", filter.Contains(text))
	fmt.Printf("干净文本检查: %t\n", filter.Contains("正常的评论内容"))

	// 步骤 4：查看统计信息
	stats := filter.Stats()
	fmt.Printf("DFA 统计: 词数=%d, 节点数=%d, 原始词数=%d\n",
		stats["words"], stats["nodes"], stats["original_words"])
}
