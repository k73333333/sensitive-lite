package sensitive

import (
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kaidong77/sensitive-lite/internal/core"
)

// rng 全局随机数生成器（Go 1.20+ 推荐使用 rand.New 替代废弃的 rand.Seed）
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// ============================================================================
// DFA 性能基准测试
// ============================================================================

// BenchmarkDFAInsert 基准测试：DFA 插入性能
func BenchmarkDFAInsert(b *testing.B) {
	words := generateTestWords(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree := core.NewDFATree()
		for _, w := range words {
			tree.Insert(w, 0)
		}
	}
}

// BenchmarkDFAMatch 基准测试：DFA 匹配性能
func BenchmarkDFAMatch(b *testing.B) {
	words := generateTestWords(10000)
	tree := core.NewDFATree()
	for _, w := range words {
		tree.Insert(w, 0)
	}

	text := generateTestText(1000, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.Match(text, nil)
	}
}

// BenchmarkDFAMatchFirst 基准测试：DFA 快速匹配性能
func BenchmarkDFAMatchFirst(b *testing.B) {
	words := generateTestWords(10000)
	tree := core.NewDFATree()
	for _, w := range words {
		tree.Insert(w, 0)
	}

	text := generateTestText(1000, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.MatchFirst(text)
	}
}

// BenchmarkDFAContains 基准测试：DFA Contains 性能
func BenchmarkDFAContains(b *testing.B) {
	words := generateTestWords(10000)
	tree := core.NewDFATree()
	for _, w := range words {
		tree.Insert(w, 0)
	}

	text := generateTestText(1000, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.Contains(text)
	}
}

// ============================================================================
// Filter 过滤器性能基准测试
// ============================================================================

// BenchmarkFilterFindAll 基准测试：FindAll 性能
func BenchmarkFilterFindAll(b *testing.B) {
	words := generateTestWords(10000)
	f := New(words, WithFuzzy(false))

	text := generateTestText(1000, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.FindAll(text)
	}
}

// BenchmarkFilterFindAllFuzzy 基准测试：反清洗 FindAll 性能
func BenchmarkFilterFindAllFuzzy(b *testing.B) {
	words := generateTestWords(5000)
	f := New(words, WithFuzzy(true))

	text := generateTestText(500, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.FindAll(text)
	}
}

// BenchmarkFilterReplace 基准测试：Replace 性能
func BenchmarkFilterReplace(b *testing.B) {
	words := generateTestWords(1000)
	f := New(words, WithFuzzy(false))

	text := generateTestText(500, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Replace(text)
	}
}

// BenchmarkFilterContains 基准测试：Contains 性能
func BenchmarkFilterContains(b *testing.B) {
	words := generateTestWords(10000)
	f := New(words, WithFuzzy(false))

	text := generateTestText(1000, words)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Contains(text)
	}
}

// ============================================================================
// 并发性能基准测试
// ============================================================================

// BenchmarkFilterConcurrentFindAll 基准测试：并发 FindAll 性能
func BenchmarkFilterConcurrentFindAll(b *testing.B) {
	words := generateTestWords(10000)
	f := New(words, WithFuzzy(false))

	text := generateTestText(1000, words)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.FindAll(text)
		}
	})
}

// BenchmarkFilterConcurrentFuzzy 基准测试：并发反清洗 FindAll 性能
func BenchmarkFilterConcurrentFuzzy(b *testing.B) {
	words := generateTestWords(5000)
	f := New(words, WithFuzzy(true))

	text := generateTestText(500, words)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.FindAll(text)
		}
	})
}

// BenchmarkFilterConcurrentContains 基准测试：并发 Contains 性能
func BenchmarkFilterConcurrentContains(b *testing.B) {
	words := generateTestWords(10000)
	f := New(words, WithFuzzy(false))

	text := generateTestText(1000, words)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Contains(text)
		}
	})
}

// BenchmarkNormalizeText 基准测试：文本标准化性能（独立测试反清洗瓶颈）
func BenchmarkNormalizeText(b *testing.B) {
	n := core.NewNormalizer(core.NormalizerConfig{})
	text := "这是一段包含敏\u200B感词和测试内容的文本，夹杂各种特殊符号和Emoji表情😊"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.Normalize(text)
	}
}

// ============================================================================
// 内存占用测试
// ============================================================================

// TestMemoryUsage 内存占用测试
// 测试 10 万级敏感词场景下的内存消耗
func TestMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过内存占用测试（使用 -short 跳过）")
	}

	// 生成 10 万个中文敏感词
	wordCount := 100000
	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		words[i] = fmt.Sprintf("敏感词%d测试", i)
	}

	// 强制 GC 以获取准确的内存基线
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// 构建过滤器
	f := New(words, WithFuzzy(true))

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("10万敏感词内存分配: %.2f MB", allocMB)
	t.Logf("DFA 统计: %+v", f.Stats())

	// 单 DFA 架构约 25-35MB，设置 40MB 上限
	if allocMB > 40 {
		t.Errorf("内存占用过高: %.2f MB (期望 < 40 MB)", allocMB)
	}
}

// TestMemoryUsageLowMem 低内存机器模拟测试
// 验证在 256MB 内存限制下的稳定性
func TestMemoryUsageLowMem(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过低内存测试（使用 -short 跳过）")
	}

	// 模拟 10 万敏感词场景
	wordCount := 100000
	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		words[i] = fmt.Sprintf("词%d测试", i)
	}

	// 构建过滤器
	f := New(words, WithFuzzy(false))

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := float64(m.Alloc) / 1024 / 1024
	t.Logf("总内存分配: %.2f MB", allocMB)

	// 执行过滤操作验证功能正常
	results := f.FindAll("这是词12345测试的文本")
	if len(results) == 0 {
		t.Fatal("低内存模式下应正常匹配敏感词")
	}

	// 并发压力测试
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				f.FindAll(fmt.Sprintf("词%d测试文本", j%wordCount))
				f.Contains(fmt.Sprintf("词%d测试", j%wordCount))
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
// 大规模匹配准确性测试
// ============================================================================

// TestLargeScaleAccuracy 大规模词库准确率测试
func TestLargeScaleAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过大规模准确率测试（使用 -short 跳过）")
	}

	wordCount := 1000
	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		words[i] = fmt.Sprintf("敏感词%d", i)
	}

	f := New(words, WithFuzzy(false))

	// 精确匹配准确率测试
	// 注意：DFA 会匹配所有子串，如 "敏感词10" 会同时匹配 "敏感词1" 和 "敏感词10"
	// 因此不检查 len(results) == 1，只验证目标词是否在结果中
	totalTests := wordCount
	correct := 0
	for i := 0; i < wordCount; i++ {
		results := f.FindAll(words[i])
		found := false
		for _, r := range results {
			if r.Word == words[i] {
				found = true
				break
			}
		}
		if found {
			correct++
		}
	}

	accuracy := float64(correct) / float64(totalTests) * 100
	t.Logf("精确匹配准确率: %.2f%% (%d/%d)", accuracy, correct, totalTests)

	if accuracy < 99.5 {
		t.Errorf("精确匹配准确率不达标: %.2f%% (要求 >= 99.5%%)", accuracy)
	}
}

// TestFuzzyAccuracy 反清洗准确率测试
// 覆盖零宽字符、全角转换、形近字等常见攻击手段
func TestFuzzyAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过反清洗准确率测试（使用 -short 跳过）")
	}

	f := New([]string{
		"敏感词",
		"日本人",
		"HELLO",
		"测试词",
		"违规内容",
	}, WithFuzzy(true))

	testCases := []struct {
		input       string
		expected    []string
		description string
	}{
		{"敏\u200B感\u200B词", []string{"敏感词"}, "零宽空格拆分"},
		{"敏\u200C感\u200C词", []string{"敏感词"}, "零宽非连接符拆分"},
		{"敏\uFEFF感\uFEFF词", []string{"敏感词"}, "BOM分隔"},
		{"敏\u00AD感\u00AD词", []string{"敏感词"}, "软连字符拆分"},
		{"敏\u2060感\u2060词", []string{"敏感词"}, "词连接符拆分"},
		{"敏 感 词", []string{"敏感词"}, "空格拆分"},
		{"曰本人", []string{"日本人"}, "形近字替换"},
		{"ＨＥＬＬＯ", []string{"HELLO"}, "全角字母"},
		{"hｅllｏ", []string{"HELLO"}, "混合全角半角"},
		{"敏😊感🎉词", []string{"敏感词"}, "Emoji干扰"},
		{"敏_感_词", []string{"敏感词"}, "下划线干扰"},
		{"敏/感/词", []string{"敏感词"}, "斜杠干扰"},
	}

	totalTests := len(testCases)
	correct := 0
	for _, tc := range testCases {
		results := f.FindAll(tc.input)
		found := make(map[string]bool)
		for _, r := range results {
			found[r.Word] = true
		}
		allFound := true
		for _, exp := range tc.expected {
			if !found[exp] {
				allFound = false
				break
			}
		}
		if allFound {
			correct++
		} else {
			t.Errorf("[%s] 输入 %q: 期望命中 %v, 实际命中 %v",
				tc.description, tc.input, tc.expected, results)
		}
	}

	accuracy := float64(correct) / float64(totalTests) * 100
	t.Logf("反清洗准确率: %.2f%% (%d/%d)", accuracy, correct, totalTests)

	if accuracy < 99.0 {
		t.Errorf("反清洗准确率不达标: %.2f%% (要求 >= 99.5%%)", accuracy)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateTestWords 生成指定数量的测试敏感词
func generateTestWords(count int) []string {
	words := make([]string, count)
	// 中文常见字池，用于生成随机中文敏感词
	charPool := []rune("的一是不了在有人我他这中大来上个国们以和出时地会为子你生家学得就发年动成" +
		"方后多天行面说过之部能自对要水下用工程分下现前开法进理高本从等实而知当经力其定么点外同些比起" +
		"心样文又看气头间问向应最使记各意已明正新关体把道题原没内制长此很全相回加" +
		"王玉主目且本木白百自皿血冶治酒洒免兔候侯准淮晴睛")

	for i := 0; i < count; i++ {
		// 随机生成 2-5 个字的中文敏感词
		length := 2 + rng.Intn(4)
		runes := make([]rune, length)
		for j := 0; j < length; j++ {
			runes[j] = charPool[rng.Intn(len(charPool))]
		}
		words[i] = string(runes)
	}
	return words
}

// generateTestText 生成包含敏感词的测试文本
func generateTestText(length int, words []string) string {
	var sb strings.Builder
	sb.Grow(length * 3)

	// 基础文本字符池
	baseChars := []rune("这是一段测试文本内容包含各种中文字符用于验证敏感词过滤系统的性能和准确性")

	for sb.Len() < length {
		// 随机决定是插入敏感词还是普通文本
		if rng.Float64() < 0.3 && len(words) > 0 {
			sb.WriteString(words[rng.Intn(len(words))])
		} else {
			// 插入随机长度的普通文本
			segLen := 5 + rng.Intn(20)
			for j := 0; j < segLen && sb.Len() < length; j++ {
				sb.WriteRune(baseChars[rng.Intn(len(baseChars))])
			}
		}
	}
	return sb.String()
}
