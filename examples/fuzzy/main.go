package main

import (
	"fmt"

	sensitive "github.com/sensitive-lite/sensitive-lite"
)

// ============================================================================
// 示例 2：反清洗敏感词识别
//
// 场景：识别经过文本干扰处理的敏感词
// 覆盖以下干扰手段：
//   - 空格拆分：敏 感 词
//   - 形近字替换：曰本人 → 日本人
//   - 全角字符：ＨＥＬＬＯ
//   - Emoji 干扰：敏😊感🎉词
//   - 标点夹杂：敏，感。词
//   - 大小写变形：HeLLo
//
// 使用方法：
//
//	go run examples/fuzzy/main.go
// ============================================================================

func main() {
	// 步骤 1：准备敏感词库
	words := []string{
		"敏感词",
		"日本人",
		"hello",
		"world",
		"测试",
		"违规内容",
	}

	// 步骤 2：创建过滤器（反清洗功能默认开启）
	filter := sensitive.New(words)

	fmt.Println("========== 反清洗敏感词识别示例 ==========")
	fmt.Println()

	// === 测试用例 1：空格拆分 ===
	testCase("空格拆分", "敏 感 词 测 试", filter)

	// === 测试用例 2：形近字替换 ===
	// 曰（yue1）→ 日（ri4），视觉上非常相似
	testCase("形近字替换", "曰本人", filter)

	// === 测试用例 3：全角字符 ===
	// 全角英文字母在视觉上几乎与半角一致
	testCase("全角字符", "ＨＥＬＬＯ世界", filter)

	// === 测试用例 4：Emoji 干扰 ===
	// 在敏感词中间插入 Emoji
	testCase("Emoji 干扰", "敏😊感🎉词检测", filter)

	// === 测试用例 5：标点夹杂 ===
	testCase("标点夹杂", "敏，感。词！", filter)

	// === 测试用例 6：大小写变形 ===
	testCase("大小写变形", "HeLLo WoRLd", filter)

	// === 测试用例 7：混合干扰 ===
	testCase("混合干扰", "敏😊 感，ｃí测  试", filter)

	fmt.Println()
	fmt.Println("========== 全部测试完成 ==========")
}

// testCase 执行单个反清洗测试用例并输出结果
func testCase(name string, text string, filter *sensitive.Filter) {
	fmt.Printf("【%s】\n", name)
	fmt.Printf("  输入: %q\n", text)

	matches := filter.FindAll(text)
	if len(matches) == 0 {
		fmt.Printf("  结果: 未检测到敏感词\n")
	} else {
		fmt.Printf("  命中 %d 个敏感词:\n", len(matches))
		for _, m := range matches {
			fmt.Printf("    - 词: %q, 位置: [%d,%d), 类型: %s\n",
				m.Word, m.Start, m.End, m.Type)
		}
	}

	// 替换后的效果
	result := filter.Replace(text)
	fmt.Printf("  替换: %q\n", result.Text)
	fmt.Println()
}
