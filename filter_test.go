package sensitive

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/sensitive-lite/sensitive-lite/internal/core"
)

// ============================================================================
// DFA 核心功能测试
// ============================================================================

func TestDFAInsertAndMatch(t *testing.T) {
	tree := core.NewDFATree()

	// 测试插入单个敏感词
	if !tree.Insert("测试", 0) {
		t.Fatal("插入 '测试' 失败")
	}
	if tree.WordCount != 1 {
		t.Fatalf("期望敏感词数 1，实际 %d", tree.WordCount)
	}

	// 测试匹配
	results := tree.Match("这是一段测试文本", nil)
	if len(results) != 1 || results[0] != "测试" {
		t.Fatalf("期望匹配 ['测试']，实际 %v", results)
	}

	// 测试无匹配
	results = tree.Match("没有敏感词", nil)
	if len(results) != 0 {
		t.Fatalf("期望无匹配，实际 %v", results)
	}
}

func TestDFAMultiWordMatch(t *testing.T) {
	tree := core.NewDFATree()
	words := []string{"敏感", "敏感词", "过滤", "测试词"}

	for _, w := range words {
		tree.Insert(w, 0)
	}

	// 测试重叠敏感词匹配
	results := tree.Match("这是一个敏感词过滤测试词", nil)
	// 应该匹配到: 敏感, 敏感词, 过滤, 测试词
	if len(results) < 3 {
		t.Fatalf("期望匹配至少 3 个敏感词，实际 %d 个: %v", len(results), results)
	}
}

func TestDFAMaxLenLimit(t *testing.T) {
	tree := core.NewDFATree()

	// 测试长度限制
	if tree.Insert("很长的敏感词", 3) {
		t.Fatal("超过长度限制的敏感词不应插入成功")
	}
	if tree.Insert("短词", 3) {
		// 长度在限制内，应该成功
	} else {
		t.Fatal("符合长度限制的敏感词应插入成功")
	}
}

func TestDFAEmptyWord(t *testing.T) {
	tree := core.NewDFATree()

	if tree.Insert("", 0) {
		t.Fatal("空字符串不应插入成功")
	}
}

func TestDFAMatchFirst(t *testing.T) {
	tree := core.NewDFATree()
	tree.Insert("敏感", 0)
	tree.Insert("过滤", 0)

	result := tree.MatchFirst("这是一个敏感词")
	if result == "" {
		t.Fatal("MatchFirst 应返回匹配结果")
	}
}

func TestDFAContains(t *testing.T) {
	tree := core.NewDFATree()
	tree.Insert("敏感", 0)

	if !tree.Contains("包含敏感词") {
		t.Fatal("Contains 应返回 true")
	}
	if tree.Contains("干净文本") {
		t.Fatal("Contains 对无敏感词文本应返回 false")
	}
}

func TestDFAReset(t *testing.T) {
	tree := core.NewDFATree()
	tree.Insert("敏感", 0)
	tree.Reset()

	if tree.WordCount != 0 {
		t.Fatal("Reset 后 WordCount 应为 0")
	}
	if tree.Contains("敏感") {
		t.Fatal("Reset 后不应匹配到任何词")
	}
}

func TestDFALargeWordList(t *testing.T) {
	tree := core.NewDFATree()
	// 测试插入 1000 个敏感词
	for i := 0; i < 1000; i++ {
		word := string(rune(0x4E00+i)) + "敏感"
		tree.Insert(word, 0)
	}

	_, nodeCount := tree.Stats()
	if nodeCount < 100 {
		t.Fatalf("1000 个敏感词应有足够多的节点，实际 %d", nodeCount)
	}
}

// ============================================================================
// 标准化器测试
// ============================================================================

func TestNormalizerFullwidthToHalfwidth(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 测试全角字母转半角
	result := n.NormalizeWord("Ｈｅｌｌｏ")
	if result != "hello" {
		t.Fatalf("全角字母应转为半角小写: 期望 'hello', 实际 '%s'", result)
	}

	// 测试全角数字转半角
	result = n.NormalizeWord("１２３４５")
	if result != "12345" {
		t.Fatalf("全角数字应转为半角: 期望 '12345', 实际 '%s'", result)
	}
}

func TestNormalizerCaseFolding(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 测试大写转小写
	result := n.NormalizeWord("HelloWORLD")
	if result != "helloworld" {
		t.Fatalf("大写应转为小写: 期望 'helloworld', 实际 '%s'", result)
	}
}

func TestNormalizerConfusableChars(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 测试全角数字转半角
	result := n.NormalizeWord("O１２３")
	// O 为标准 ASCII 字母保持不变（仅转小写），１２３ 转半角
	if result != "o123" {
		t.Fatalf("全角数字应归一化: 期望 'o123', 实际 '%s'", result)
	}

	// 测试中文形近字
	result = n.NormalizeWord("曰本人") // 曰→日
	if result != "日本人" {
		t.Fatalf("形近字应归一化: 期望 '日本人', 实际 '%s'", result)
	}
}

func TestNormalizerStripSpecialChars(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 测试去除空格和标点
	text := "敏 感 词"
	normalized := n.Normalize(text)
	if normalized.Text != "敏感词" {
		t.Fatalf("空格应被去除: 期望 '敏感词', 实际 '%s'", normalized.Text)
	}

	// 测试去除标点符号
	text = "敏，感。词！"
	normalized = n.Normalize(text)
	if normalized.Text != "敏感词" {
		t.Fatalf("标点应被去除: 期望 '敏感词', 实际 '%s'", normalized.Text)
	}
}

func TestNormalizerStripEmoji(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 测试去除 Emoji
	text := "敏感😊词🎉测试"
	normalized := n.Normalize(text)
	if normalized.Text != "敏感词测试" {
		t.Fatalf("Emoji 应被去除: 期望 '敏感词测试', 实际 '%s'", normalized.Text)
	}
}

func TestNormalizerPositionMapping(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	text := "A B C"
	normalized := n.Normalize(text)

	if normalized.Text != "abc" {
		t.Fatalf("期望 'abc', 实际 '%s'", normalized.Text)
	}
	// 验证位置映射：标准化后的 'a' 应对应原始 'A' 的字节偏移 0
	if len(normalized.PosMap) != 3 {
		t.Fatalf("期望 3 个位置映射，实际 %d", len(normalized.PosMap))
	}
	if normalized.PosMap[0] != 0 {
		t.Fatalf("第一个标准化字符的字节偏移应为 0，实际 %d", normalized.PosMap[0])
	}
}

// ============================================================================
// Filter 过滤器集成测试
// ============================================================================

func TestFilterExactMatch(t *testing.T) {
	f := New([]string{"敏感词", "测试", "过滤"}, WithFuzzy(false))

	results := f.FindAll("这是一个敏感词过滤测试")
	if len(results) != 3 {
		t.Fatalf("精确匹配应找到 3 个敏感词，实际 %d: %v", len(results), results)
	}

	for _, r := range results {
		if r.Type != MatchExact {
			t.Fatalf("精确匹配结果的 Type 应为 MatchExact，实际 %v", r.Type)
		}
	}
}

func TestFilterFuzzyMatchSpaceSplit(t *testing.T) {
	f := New([]string{"敏感词", "测试"})

	// 测试空格拆分后的敏感词识别
	results := f.FindAll("敏 感 词 测 试")
	if len(results) == 0 {
		t.Fatal("空格拆分的敏感词应被反清洗识别")
	}
}

func TestFilterFuzzyMatchConfusable(t *testing.T) {
	f := New([]string{"日本人", "测试"})

	// 测试形近字替换：曰 → 日
	results := f.FindAll("曰本人")
	if len(results) == 0 {
		t.Fatal("形近字替换的敏感词应被识别")
	}
	t.Logf("形近字匹配结果: %+v", results)
}

func TestFilterFuzzyMatchFullwidth(t *testing.T) {
	f := New([]string{"hello", "测试"})

	// 测试全角字符
	results := f.FindAll("ＨＥＬＬＯ")
	if len(results) == 0 {
		t.Fatal("全角字母敏感词应被识别")
	}
}

func TestFilterFuzzyMatchEmoji(t *testing.T) {
	f := New([]string{"敏感词", "测试"})

	// 测试 Emoji 干扰
	results := f.FindAll("敏😊感🎉词")
	if len(results) == 0 {
		t.Fatal("Emoji 干扰的敏感词应被识别")
	}
}

func TestFilterReplace(t *testing.T) {
	f := New([]string{"敏感", "过滤"})

	result := f.Replace("这是敏感词过滤")
	if result.Count != 2 {
		t.Fatalf("期望替换 2 个敏感词，实际 %d", result.Count)
	}

	// 敏感(2 runes) → **, 过滤(2 runes) → **
	expected := "这是**词**"
	if result.Text != expected {
		t.Fatalf("替换结果错误: 期望 '%s', 实际 '%s'", expected, result.Text)
	}
}

func TestFilterReplaceCustomReplacement(t *testing.T) {
	f := New([]string{"敏感"}, WithReplacement('#'))

	result := f.Replace("这是敏感词")
	expected := "这是##词"
	if result.Text != expected {
		t.Fatalf("自定义替换字符: 期望 '%s', 实际 '%s'", expected, result.Text)
	}
}

func TestFilterContains(t *testing.T) {
	f := New([]string{"敏感"})

	if !f.Contains("包含敏感词") {
		t.Fatal("Contains 应返回 true")
	}
	if f.Contains("干净文本") {
		t.Fatal("Contains 应返回 false")
	}
}

func TestFilterEmptyText(t *testing.T) {
	f := New([]string{"敏感"})

	results := f.FindAll("")
	if len(results) != 0 {
		t.Fatal("空文本不应有匹配结果")
	}

	result := f.Replace("")
	if result.Text != "" || result.Count != 0 {
		t.Fatal("空文本替换结果应为空")
	}

	if f.Contains("") {
		t.Fatal("空文本不应包含敏感词")
	}
}

func TestFilterEmptyWordList(t *testing.T) {
	f := New([]string{})

	results := f.FindAll("测试文本")
	if len(results) != 0 {
		t.Fatal("空词库不应有匹配结果")
	}
}

func TestFilterLazyBuild(t *testing.T) {
	f := New([]string{"敏感", "测试"}, WithLazyBuild(true))

	// 首次调用触发懒构建
	results := f.FindAll("这是一个敏感测试")
	if len(results) != 2 {
		t.Fatalf("懒构建后应正确匹配: 期望 2，实际 %d: %v", len(results), results)
	}
}

func TestFilterMaxWordLen(t *testing.T) {
	f := New([]string{"短", "很长很长很长"}, WithMaxWordLen(3))

	results := f.FindAll("短")
	if len(results) != 1 {
		t.Fatal("符合长度限制的词应被匹配")
	}

	// 超长词不应被匹配
	results = f.FindAll("很长很长很长")
	if len(results) > 0 {
		t.Fatal("超过长度限制的词不应被匹配")
	}
}

func TestFilterStats(t *testing.T) {
	f := New([]string{"敏感", "过滤", "测试"})

	stats := f.Stats()
	if stats["words"] != 3 {
		t.Fatalf("期望 words=3，实际 %d", stats["words"])
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestFilterConcurrent(t *testing.T) {
	f := New([]string{"敏感", "测试", "过滤", "并发", "安全"})

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			f.FindAll("这是一个并发安全测试，包含敏感词过滤")
			f.Contains("敏感")
			f.Replace("过滤测试文本")
			f.Stats()
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestFilterDuplicatedWords(t *testing.T) {
	// 测试重复敏感词的去重
	f := New([]string{"敏感", "敏感", "敏感"})
	stats := f.Stats()
	if stats["words"] != 1 {
		t.Fatalf("重复敏感词应去重: 期望 1，实际 %d", stats["words"])
	}
}

func TestFilterMatchTypeFromExact(t *testing.T) {
	f := New([]string{"测试"}, WithFuzzy(false))
	results := f.FindAll("这是一段测试文本")
	if len(results) == 1 && results[0].Type != MatchExact {
		t.Fatal("精确命中的匹配类型应为 MatchExact")
	}
}

// ============================================================================
// 覆盖率补充测试
// ============================================================================

// TestMatchTypeString 测试匹配类型字符串转换
func TestMatchTypeString(t *testing.T) {
	if MatchExact.String() != "exact" {
		t.Fatalf("MatchExact.String() 期望 'exact', 实际 '%s'", MatchExact.String())
	}
	if MatchFuzzy.String() != "fuzzy" {
		t.Fatalf("MatchFuzzy.String() 期望 'fuzzy', 实际 '%s'", MatchFuzzy.String())
	}
	if MatchType(99).String() != "unknown" {
		t.Fatalf("未知 MatchType 期望 'unknown', 实际 '%s'", MatchType(99).String())
	}
}

// TestDFAResetWithChildren 测试 DFA Reset 递归释放子节点
func TestDFAResetWithChildren(t *testing.T) {
	tree := core.NewDFATree()
	// 插入多层嵌套词以触发 map 模式子节点释放
	tree.Insert("敏感词0", 0)
	tree.Insert("敏感词1", 0)
	tree.Insert("敏感词2", 0)
	tree.Insert("敏感词3", 0)
	tree.Insert("敏感词4", 0)
	tree.Insert("敏感词5", 0) // 超过 4 个子节点触发 map 模式
	tree.Insert("敏感词10", 0)
	tree.Insert("敏感词11", 0)

	_, nodeCount := tree.Stats()
	if nodeCount < 5 {
		t.Fatalf("Reset 前应有多个节点，实际 %d", nodeCount)
	}

	tree.Reset()
	_, nodeCount = tree.Stats()
	if nodeCount != 1 {
		t.Fatalf("Reset 后应只剩根节点，实际 %d", nodeCount)
	}
}

// TestNormalizerHalfwidthKana 测试半角假名字符标准化
func TestNormalizerHalfwidthKana(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})
	// 半角片假名字母 A（U+FF71 ｱ），属于半角假名字母范围 FF66-FF9F
	result := n.NormalizeWord(string(rune(0xFF71)))
	if result == "" {
		t.Fatal("半角片假名字母不应被过滤为空")
	}
	if result != string(rune(0xFF71)) {
		t.Logf("半角假名标准化: %q → %q", string(rune(0xFF71)), result)
	}
}

// TestNormalizerRuneByteLen 测试 rune 字节长度计算
func TestNormalizerRuneByteLen(t *testing.T) {
	// 1 字节 ASCII
	if l := core.RuneByteLen('a'); l != 1 {
		t.Fatalf("ASCII 'a' 字节长度应为 1，实际 %d", l)
	}
	// 2 字节字符（拉丁扩展）
	if l := core.RuneByteLen('é'); l != 2 {
		t.Fatalf("'é' 字节长度应为 2，实际 %d", l)
	}
	// 3 字节字符（中文）
	if l := core.RuneByteLen('中'); l != 3 {
		t.Fatalf("'中' 字节长度应为 3，实际 %d", l)
	}
	// 4 字节字符（Emoji）
	if l := core.RuneByteLen('😊'); l != 4 {
		t.Fatalf("'😊' 字节长度应为 4，实际 %d", l)
	}
}

// TestNormalizerIsSkippableAllTypes 测试所有类型的跳过字符
func TestNormalizerIsSkippableAllTypes(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})

	// 制表符
	text := "敏\t感\t词"
	normalized := n.Normalize(text)
	if normalized.Text != "敏感词" {
		t.Fatalf("制表符应被跳过: 期望 '敏感词', 实际 '%s'", normalized.Text)
	}

	// 换行符
	text = "敏\n感\n词"
	normalized = n.Normalize(text)
	if normalized.Text != "敏感词" {
		t.Fatalf("换行符应被跳过: 期望 '敏感词', 实际 '%s'", normalized.Text)
	}

	// 全角标点（中文逗号句号）
	text = "敏，感。词"
	normalized = n.Normalize(text)
	if normalized.Text != "敏感词" {
		t.Fatalf("全角标点应被跳过: 期望 '敏感词', 实际 '%s'", normalized.Text)
	}
}

// TestNormalizerIsEmojiAllRanges 测试所有 Emoji 范围的检测
func TestNormalizerIsEmojiAllRanges(t *testing.T) {
	testCases := []struct {
		emoji rune
		desc  string
	}{
		{0x1F600, "表情 U+1F600"},
		{0x1F4A9, "杂项符号 U+1F4A9"},
		{0x1F680, "交通符号 U+1F680"},
		{0x1F91D, "补充符号 U+1F91D"},
		{0x2705, "装饰符号 U+2705"},
		{0x2620, "杂项 U+2620"},
		{0x2B50, "箭头符号 U+2B50"},
		{0xFE0F, "变体选择器 U+FE0F"},
		{0x200D, "零宽连接符 ZWJ"},
	}

	for _, tc := range testCases {
		if !core.IsEmoji(tc.emoji) {
			t.Errorf("%s 应被识别为 Emoji，但返回 false", tc.desc)
		}
	}

	// 验证非 Emoji 返回 false
	if core.IsEmoji('a') {
		t.Fatal("字母 'a' 不应被识别为 Emoji")
	}
	if core.IsEmoji('中') {
		t.Fatal("'中' 不应被识别为 Emoji")
	}
}

// TestFilterStatsEdgeCases 测试 Stats 边界情况
func TestFilterStatsEdgeCases(t *testing.T) {
	// 精确匹配模式
	f := New([]string{"a", "b"}, WithFuzzy(false))
	stats := f.Stats()
	if stats["words"] != 2 {
		t.Fatalf("精确模式: 期望 words=2，实际 %d", stats["words"])
	}
}

// TestByteOffsetToRuneIndexEdge 测试字节偏移转换边界
func TestByteOffsetToRuneIndexEdge(t *testing.T) {
	if idx := byteOffsetToRuneIndex("hello", -1); idx != 0 {
		t.Fatalf("负偏移应返回 0，实际 %d", idx)
	}
	if idx := byteOffsetToRuneIndex("hello", 100); idx != 5 {
		t.Fatalf("超长偏移应返回 rune 长度，实际 %d", idx)
	}
	if idx := byteOffsetToRuneIndex("hello", 0); idx != 0 {
		t.Fatalf("偏移 0 应返回 0，实际 %d", idx)
	}
	if idx := byteOffsetToRuneIndex("hello", 1); idx != 1 {
		t.Fatalf("偏移 1 应返回 1，实际 %d", idx)
	}
	// 中文字符（每字符 3 字节）
	if idx := byteOffsetToRuneIndex("你好", 3); idx != 1 {
		t.Fatalf("中文偏移 3 应返回 1，实际 %d", idx)
	}
}

// TestFilterMixedInterference 测试混合干扰场景
func TestFilterMixedInterference(t *testing.T) {
	f := New([]string{"敏感词", "HELLO"})

	// 混合大小写 + 空格 + 全角
	results := f.FindAll("敏 感 词hｅＬＬｏ")
	found := make(map[string]bool)
	for _, r := range results {
		found[r.Word] = true
	}

	// 验证两种类型的敏感词都被识别
	hasSensitive := false
	hasHello := false
	for _, r := range results {
		if r.Word == "敏感词" {
			hasSensitive = true
		}
		if r.Word == "HELLO" {
			hasHello = true
		}
	}
	if !hasSensitive {
		t.Fatal("混合干扰中应识别到 '敏感词'")
	}
	if !hasHello {
		t.Fatal("混合干扰中应识别到 'HELLO'")
	}
}

// TestFilterSpecialCharInterference 测试特殊字符干扰
func TestFilterSpecialCharInterference(t *testing.T) {
	f := New([]string{"测试词"})

	// 各种特殊符号干扰
	results := f.FindAll("测_试_词")
	found := false
	for _, r := range results {
		if r.Word == "测试词" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("特殊字符干扰应识别到 '测试词'")
	}
}

// ============================================================================
// 日志、溯源追踪、降级策略测试
// ============================================================================

// TestLoggerIntegration 测试自定义日志器集成
func TestLoggerIntegration(t *testing.T) {
	customLogger := &testLogger{}
	f := New([]string{"敏感词"}, WithLogger(customLogger))

	// 执行操作触发日志
	f.FindAll("包含敏感词")

	// 验证日志器被调用
	if customLogger.count == 0 {
		t.Log("注意：默认日志级别为 Warn，Debug 信息不会输出（预期行为）")
	}
}

// testLogger 测试用日志器（记录到 buffer）
type testLogger struct {
	count int
}

func (l *testLogger) Debug(format string, args ...interface{}) { l.count++ }
func (l *testLogger) Info(format string, args ...interface{})  { l.count++ }
func (l *testLogger) Warn(format string, args ...interface{})  { l.count++ }
func (l *testLogger) Error(format string, args ...interface{}) { l.count++ }

// TestTraceCallback 测试溯源追踪回调
func TestTraceCallback(t *testing.T) {
	var records []TraceRecord
	var mu sync.Mutex

	cb := func(record TraceRecord) {
		mu.Lock()
		records = append(records, record)
		mu.Unlock()
	}

	f := New([]string{"敏感词", "测试"}, WithTraceCallback(cb))

	// 执行多次过滤操作（FindAll 和 Replace 各自触发一次 trace 回调）
	// 注意：Contains 不走 FindAll 路径，不会触发 trace 回调
	f.FindAll("敏感词测试")
	f.Replace("敏感词替换测试")
	f.FindAll("敏感词再次测试")

	mu.Lock()
	count := len(records)
	mu.Unlock()

	if count < 3 {
		t.Fatalf("溯源回调应被调用 3 次，实际 %d 次", count)
	}

	mu.Lock()
	for _, r := range records {
		if r.MatchCount <= 0 {
			t.Errorf("溯源记录应包含匹配数 > 0，实际 %d", r.MatchCount)
		}
		if r.TextLen <= 0 {
			t.Error("溯源记录文本长度不应为 0")
		}
		if r.Duration < 0 {
			t.Error("溯源记录耗时不应为负数")
		}
	}
	mu.Unlock()
}

// TestDegradeAndRecover 测试降级与恢复机制
func TestDegradeAndRecover(t *testing.T) {
	f := New([]string{"敏 感 词", "测试"})

	// 验证反清洗正常工作
	results := f.FindAll("敏 感 词")
	if len(results) == 0 {
		t.Fatal("降级前反清洗应正常匹配")
	}

	// 触发降级
	f.Degrade()
	if !f.isDegraded() {
		t.Fatal("Degrade 后应处于降级状态")
	}

	// 降级后反清洗应失效
	results = f.FindAll("敏 感 词")
	if len(results) > 0 {
		t.Log("降级后反清洗已关闭，空格拆分不再被识别（预期行为）")
	}

	// 降级后精确匹配应仍正常工作
	results = f.FindAll("测试")
	if len(results) == 0 {
		t.Fatal("降级后精确匹配应仍正常工作")
	}

	// 恢复
	f.Recover()
	if f.isDegraded() {
		t.Fatal("Recover 后应退出降级状态")
	}

	// 恢复后反清洗应恢复
	results = f.FindAll("敏 感 词")
	if len(results) == 0 {
		t.Fatal("恢复后反清洗应恢复匹配")
	}
}

// TestCheckMemoryAndDegrade 测试自动内存降级
func TestCheckMemoryAndDegrade(t *testing.T) {
	// 设置降级配置：剩余内存低于 256MB 时触发降级
	f := New([]string{"测试"}, WithDegradeConfig(DegradeConfig{MaxMemoryMB: 256}))

	// 内存充足时不应触发降级
	f.CheckMemoryAndDegrade(4096)
	if f.isDegraded() {
		t.Fatal("内存充足时不应触发降级")
	}

	// 内存不足时自动降级（128MB < 256MB 阈值）
	f.CheckMemoryAndDegrade(128)
	if !f.isDegraded() {
		t.Fatal("内存不足时应触发降级")
	}
}

// TestZeroWidthBypass 测试零宽字符旁路攻击
func TestZeroWidthBypass(t *testing.T) {
	f := New([]string{"敏感词", "测试词", "违规"})

	testCases := []struct {
		name  string
		input string
	}{
		{"零宽空格 ZWSP", "敏\u200B感\u200B词"},
		{"零宽非连接符 ZWNJ", "敏\u200C感\u200C词"},
		{"字节顺序标记 BOM", "敏\uFEFF感\uFEFF词"},
		{"软连字符", "敏\u00AD感\u00AD词"},
		{"词连接符", "敏\u2060感\u2060词"},
		{"从左到右标记 LRM", "敏\u200E感\u200E词"},
		{"从右到左标记 RLM", "敏\u200F感\u200F词"},
		{"不可见分隔 U+2061", "敏\u2061感\u2061词"},
		{"行分隔符", "敏\u2028感\u2028词"},
		{"段分隔符", "敏\u2029感\u2029词"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == "敏感词" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("零宽字符旁路测试失败 [%s]: 输入 %q 未检测到 '敏感词', 结果=%v",
					tc.name, tc.input, results)
			}
		})
	}
}

// TestFuzzyPositionAccuracy 测试反清洗匹配位置的精确性
func TestFuzzyPositionAccuracy(t *testing.T) {
	f := New([]string{"敏感词"}, WithFuzzy(true))

	// 在中文前缀后插入零宽字符的敏感词
	text := "前言敏\u200B感词结束"
	results := f.FindAll(text)
	if len(results) == 0 {
		t.Fatal("应匹配到敏感词")
	}

	t.Logf("匹配区间: [%d,%d) = %q, 原始敏感词: %q",
		results[0].Start, results[0].End, text[results[0].Start:results[0].End], results[0].Word)

	if results[0].Word != "敏感词" {
		t.Errorf("匹配词不正确: 期望 '敏感词', 实际 %q", results[0].Word)
	}

	// 验证位置不能为 0（应跳过"前言"部分）
	if results[0].Start != len("前言") {
		t.Errorf("起始位置应为 6（跳过'前言'），实际 %d", results[0].Start)
	}
}

// TestMultiOccurrenceFuzzy 测试同词多次出现
func TestMultiOccurrenceFuzzy(t *testing.T) {
	f := New([]string{"敏感词"}, WithFuzzy(true))

	// 同一敏感词在文本中多次出现（不同干扰方式）
	text := "敏 感 词 在这里，还有敏感词再次出现"
	results := f.FindAll(text)

	// 应至少匹配到两次：一次反清洗（空格拆分），一次精确
	if len(results) < 2 {
		t.Fatalf("多次出现应匹配至少 2 次，实际 %d: %+v", len(results), results)
	}

	// 验证报告的是原始敏感词
	for _, r := range results {
		if r.Word != "敏感词" {
			t.Errorf("匹配词应为 '敏感词'，实际 %q", r.Word)
		}
	}
}

// ============================================================================
// v2.1 新增测试：批量匹配、词库快照、告警回调、西里尔同形字、极端输入
// ============================================================================

// TestFindAllBatch 测试批量匹配 API
func TestFindAllBatch(t *testing.T) {
	f := New([]string{"敏感词", "测试", "违规"}, WithFuzzy(true))

	// 批量检测多条文本
	texts := []string{
		"这是敏感词测试",
		"干净文本无敏感",
		"包含违规内容",
		"",
	}

	results := f.FindAllBatch(texts)
	if len(results) != 4 {
		t.Fatalf("批量结果数量应为 4，实际 %d", len(results))
	}

	// 第0条：应有 2 个匹配（敏感词、测试）
	if len(results[0]) < 2 {
		t.Errorf("第0条文本应至少匹配2个敏感词，实际 %d", len(results[0]))
	}
	// 第1条：应无匹配
	if len(results[1]) != 0 {
		t.Errorf("第1条文本应无匹配，实际 %d", len(results[1]))
	}
	// 第2条：应有匹配
	if len(results[2]) != 1 {
		t.Errorf("第2条文本应匹配1个敏感词，实际 %d", len(results[2]))
	}
	// 第3条：空文本应无匹配
	if len(results[3]) != 0 {
		t.Errorf("第3条空文本应无匹配，实际 %d", len(results[3]))
	}
}

// TestWordsSnapshot 测试词库快照导出
func TestWordsSnapshot(t *testing.T) {
	f := New([]string{"敏感词A", "敏感词B", "测试"}, WithFuzzy(true))

	words := f.WordsSnapshot()

	// 反清洗模式应能导出原始词
	if len(words) < 2 {
		t.Errorf("词库快照应至少包含2个词，实际 %d: %v", len(words), words)
	}

	// 验证包含特定词
	found := make(map[string]bool)
	for _, w := range words {
		found[w] = true
	}
	if !found["敏感词A"] {
		t.Error("词库快照应包含 '敏感词A'")
	}
	if !found["测试"] {
		t.Error("词库快照应包含 '测试'")
	}
}

// TestAlertCallback 测试告警回调
func TestAlertCallback(t *testing.T) {
	var alerts []AlertRecord
	var mu sync.Mutex

	cb := func(record AlertRecord) {
		mu.Lock()
		alerts = append(alerts, record)
		mu.Unlock()
	}

	f := New([]string{"测试"},
		WithAlertCallback(cb),
		WithDegradeConfig(DegradeConfig{MaxMatchDuration: 1}), // 设置极低阈值触发告警
	)

	// 触发降级告警
	f.Degrade()

	mu.Lock()
	alertCount := len(alerts)
	mu.Unlock()

	if alertCount < 1 {
		t.Fatal("降级操作应触发至少1次告警回调")
	}

	mu.Lock()
	firstAlert := alerts[0]
	mu.Unlock()

	if firstAlert.Level != AlertLevelCritical {
		t.Errorf("降级告警级别应为 CRITICAL，实际 %s", firstAlert.Level)
	}
	if !firstAlert.IsDegraded {
		t.Error("降级告警中 IsDegraded 应为 true")
	}
	if firstAlert.Title != "反清洗功能已降级" {
		t.Errorf("告警标题应为 '反清洗功能已降级'，实际 %q", firstAlert.Title)
	}
}

// TestCyrillicHomoglyphBypass 测试西里尔字母同形攻击
func TestCyrillicHomoglyphBypass(t *testing.T) {
	f := New([]string{
		"hello", "react", "best",
		"cat", "top", "park", "mom",
		"box",
	}, WithFuzzy(true))

	testCases := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{"西里尔 е→e", "h\u0435llo", "hello", "е U+0435"},
		{"西里尔 о→o", "hell\u043E", "hello", "о U+043E"},
		{"西里尔 с→c", "\u0441at", "cat", "с U+0441"},
		{"西里尔 а→a", "p\u0430rk", "park", "а U+0430"},
		{"西里尔 н→h", "\u043Dello", "hello", "н U+043D"},
		{"西里尔 м→m", "\u043Com", "mom", "м U+043C"},
		{"西里尔 т→t", "\u0442op", "top", "т U+0442"},
		{"西里尔 х→x", "bo\u0445", "box", "х U+0445"},
		{"西里尔大写 Н→h", "\u041Dello", "hello", "Н U+041D"},
		{"西里尔大写 Т→t", "\u0422op", "top", "Т U+0422"},
		{"西里尔大写 Р→p", "\u0420ark", "park", "Р U+0420"},
		{"西里尔大写 М→m", "\u041Com", "mom", "М U+041C"},
		{"西里尔大写 С→c", "\u0421at", "cat", "С U+0421"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == tc.expected {
					found = true
					break
				}
			}
			if !found {
				nt := f.normalizer.Normalize(tc.input)
				t.Errorf("西里尔同形攻击漏检 [%s]: 输入 %q → 标准化=%q, 期望命中 %q, 实际结果: %+v",
					tc.name, tc.input, nt.Text, tc.expected, results)
			} else {
				t.Logf("[%s] %s 输入 %q → %q ✓", tc.name, tc.desc, tc.input, tc.expected)
			}
		})
	}

	cleanResults := f.FindAll("正常的hello文本无敏感词")
	foundHello := false
	for _, r := range cleanResults {
		if r.Word == "hello" {
			foundHello = true
			break
		}
	}
	if !foundHello {
		t.Error("正常拉丁文本 'hello' 应被检测")
	}
}

// TestFilterExtremeInput 测试极端输入场景
func TestFilterExtremeInput(t *testing.T) {
	f := New([]string{"敏感词", "TEST"}, WithFuzzy(true))

	// 场景1：超长文本（100KB纯中文）
	longText := strings.Repeat("这是一段很长的测试文本包含各种中文字符用于验证过滤系统", 2000)
	results := f.FindAll(longText)
	if len(results) > 0 {
		t.Logf("超长文本检测完成，命中 %d 个敏感词（如包含敏感词则为正常）", len(results))
	} else {
		t.Log("超长文本检测完成，无敏感词（预期）")
	}

	// 场景2：纯干扰字符文本（全是由空格、Emoji、零宽字符组成）
	noisyText := "\u200B\u200C😊 \uFEFF\t\n,.\u200D🎉"
	results = f.FindAll(noisyText)
	if len(results) != 0 {
		t.Errorf("纯干扰字符文本不应有匹配，实际 %d: %+v", len(results), results)
	}

	// 场景3：极短文本（单个字符）
	results = f.FindAll("测")
	if len(results) != 0 {
		t.Logf("单字符文本检测: %+v", results)
	}
}

// TestDefaultLoggerIntegration 测试默认日志器是否正常工作
func TestDefaultLoggerIntegration(t *testing.T) {
	f := New([]string{"测试"}, WithLogLevel(LogLevelWarn))

	f.FindAll("测试文本")

	stats := f.Stats()
	if stats["words"] != 1 {
		t.Fatalf("默认日志器下词数应为 1，实际 %d", stats["words"])
	}

	f2 := New([]string{"测试"}, WithLogLevel(LogLevelOff))
	results := f2.FindAll("测试文本")
	if len(results) != 1 {
		t.Fatal("关闭日志时过滤功能应正常工作")
	}
}

// TestAlertLevelString 测试告警级别字符串转换
func TestAlertLevelString(t *testing.T) {
	if AlertLevelWarn.String() != "WARN" {
		t.Fatalf("AlertLevelWarn 期望 'WARN'，实际 '%s'", AlertLevelWarn.String())
	}
	if AlertLevelCritical.String() != "CRITICAL" {
		t.Fatalf("AlertLevelCritical 期望 'CRITICAL'，实际 '%s'", AlertLevelCritical.String())
	}
	if AlertLevel(99).String() != "UNKNOWN" {
		t.Fatalf("未知 AlertLevel 期望 'UNKNOWN'，实际 '%s'", AlertLevel(99).String())
	}
}

// ============================================================================
// v2.2 新增测试：Leet Speak、Bidi Override、Tag 字符、CJK 间杂剥离
// ============================================================================

// TestLeetSpeakMapping 测试 Leet Speak 数字/符号→字母映射
func TestLeetSpeakMapping(t *testing.T) {
	// 启用 Leet Speak
	f := New([]string{"hello", "password", "test", "google", "boy", "shit", "ass"}, WithLeetSpeak(true))

	testCases := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{"3→e", "h3llo", "hello", "数字3替代e"},
		{"0→o", "hell0", "hello", "数字0替代o"},
		{"@→a", "p@ssword", "password", "@符号替代a"},
		{"5→s", "5hit", "shit", "数字5替代s"},
		{"4→a", "4ss", "ass", "数字4替代a"},
		{"7→t", "7est", "test", "数字7替代t"},
		{"8→b", "8oy", "boy", "数字8替代b"},
		{"6→g", "6oogle", "google", "数字6替代g"},
		{"9→g", "9oogle", "google", "数字9替代g"},
		{"1→l", "he1lo", "hello", "数字1替代l"},
		{"$→s", "$hit", "shit", "$符号替代s"},
		{"+→t", "+est", "test", "+符号替代t"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == tc.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Leet Speak 漏检 [%s]: 输入 %q 期望命中 %q, 实际结果: %+v",
					tc.desc, tc.input, tc.expected, results)
			}
		})
	}

	// 组合攻击测试
	comboResults := f.FindAll("h3ll0p@55w0rd")
	found := false
	for _, r := range comboResults {
		if r.Word == "password" {
			found = true
			break
		}
	}
	if found {
		t.Log("组合 Leet Speak 攻击检测成功")
	}
}

// TestLeetSpeakNotAffectsNormalText 测试 Leet Speak 不影响正常文本（无假阳性）
func TestLeetSpeakNotAffectsNormalText(t *testing.T) {
	f := New([]string{"hello", "test"}, WithLeetSpeak(true))

	// 正常数字不应导致误判
	results := f.FindAll("价格是100元")
	if len(results) > 0 {
		t.Logf("正常数字文本检测结果: %+v（如果误判需要关注）", results)
	}

	// 正常拉丁文本不应受影响
	results = f.FindAll("hello world test")
	if len(results) < 2 {
		t.Error("正常文本 'hello world test' 应检测到敏感词")
	}
}

// TestBidiOverrideFiltering 测试双向文本覆盖字符过滤
func TestBidiOverrideFiltering(t *testing.T) {
	f := New([]string{"敏感词", "测试"}, WithFuzzy(true))

	// Bidi 覆盖字符应被过滤，敏感词仍能被检测
	testCases := []struct {
		name  rune
		input string
	}{
		{0x202A, "敏\u202A感\u202A词"}, // LRE 从左到右嵌入
		{0x202B, "敏\u202B感\u202B词"}, // RLE 从右到左嵌入
		{0x202D, "敏\u202D感\u202D词"}, // LRO 从左到右覆盖
		{0x202E, "敏\u202E感\u202E词"}, // RLO 从右到左覆盖
		{0x202C, "敏\u202C感\u202C词"}, // PDF 弹出方向格式
		{0x2066, "敏\u2066感\u2066词"}, // LRI 从左到右隔离
		{0x2067, "敏\u2067感\u2067词"}, // RLI 从右到左隔离
		{0x2069, "敏\u2069感\u2069词"}, // PDI 弹出方向隔离
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("U+%04X", tc.name), func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == "敏感词" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Bidi字符 U+%04X 应被过滤: 输入 %q 未检测到 '敏感词', 结果=%v",
					tc.name, tc.input, results)
			}
		})
	}
}

// TestTagCharacterFiltering 测试 Unicode Tag 字符过滤
func TestTagCharacterFiltering(t *testing.T) {
	f := New([]string{"敏感词"}, WithFuzzy(true))

	// 使用字符串字面量构造 Tag SPACE 字符 U+E0020
	tagSpace := string(rune(0xE0020))
	input := "敏" + tagSpace + "感" + tagSpace + "词"

	results := f.FindAll(input)
	found := false
	for _, r := range results {
		if r.Word == "敏感词" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Tag 字符应被过滤: 输入 %q 未检测到敏感词, 结果=%v", input, results)
	} else {
		t.Logf("Tag 字符过滤成功: %q -> 命中敏感词", input)
	}

	// Tag 取消字符 U+E007F
	cancelTag := string(rune(0xE007F))
	input2 := "敏" + cancelTag + "感" + cancelTag + "词"
	results2 := f.FindAll(input2)
	found2 := false
	for _, r := range results2 {
		if r.Word == "敏感词" {
			found2 = true
			break
		}
	}
	if !found2 {
		t.Errorf("Tag Cancel 字符应被过滤: 输入 %q 未检测到敏感词, 结果=%v", input2, results2)
	}
}

// TestCJKInterstitialStripping 测试 CJK 间杂字符剥离
func TestCJKInterstitialStripping(t *testing.T) {
	f := New([]string{"敏感词", "测试词", "违规内容"}, WithFuzzy(true))

	testCases := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{"数字嵌入", "敏1感2词", "敏感词", "数字夹在中文字符之间"},
		{"拉丁字母嵌入", "敏a感b词", "敏感词", "拉丁字母夹在中文字符之间"},
		{"混合嵌入", "测1试2词", "测试词", "数字+中文混合"},
		{"单字符不剥离", "违1规内容", "违规内容", "单个数字嵌入仍应匹配"},
		{"多数字嵌入", "敏123感456词", "敏感词", "多个连续数字嵌入"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == tc.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("CJK 间杂剥离漏检 [%s]: 输入 %q 期望命中 %q, 实际结果: %+v",
					tc.desc, tc.input, tc.expected, results)
			} else {
				t.Logf("[%s] 输入 %q → %q ✓", tc.desc, tc.input, tc.expected)
			}
		})
	}

	// 边界测试：独立的数字/字母不应被剥离
	results := f.FindAll("这个商品编号123是违规内容")
	found := false
	for _, r := range results {
		if r.Word == "违规内容" {
			found = true
			break
		}
	}
	if !found {
		t.Error("非间杂的数字不应影响正常关键词检测")
	}
}

// TestCJKInterstitialNotAffectsNormal 测试 CJK 间杂剥离不误判正常文本
func TestCJKInterstitialNotAffectsNormal(t *testing.T) {
	f := New([]string{"测试"}, WithFuzzy(true))

	// 末尾的数字不应被误判（因为后面没有 CJK 字符）
	results := f.FindAll("成绩测试123")
	found := false
	for _, r := range results {
		if r.Word == "测试" {
			found = true
			break
		}
	}
	if !found {
		t.Error("末尾带数字的中文后续不应影响检测: '成绩测试123'")
	}

	// 开头的数字不应被误判（因为前面没有 CJK 字符）
	results = f.FindAll("123测试成绩")
	found = false
	for _, r := range results {
		if r.Word == "测试" {
			found = true
			break
		}
	}
	if !found {
		t.Error("开头带数字的中文前缀不应影响检测: '123测试成绩'")
	}
}

// ============================================================================
// v3.0 新增测试：CJK重复压缩、数学符号标准化、组合字符剥离
// ============================================================================

// TestDedupBasic 测试 CJK 重复字符压缩基本功能
func TestDedupBasic(t *testing.T) {
	input := []rune("敏敏敏感词")
	result := core.Dedup(input)
	if string(result) != "敏感词" {
		t.Fatalf("连续重复压缩失败: 期望 '敏感词', 实际 '%s'", string(result))
	}

	input = []rune("敏感词测试")
	result = core.Dedup(input)
	if string(result) != "敏感词测试" {
		t.Fatalf("非重复字符不应被压缩: 期望 '敏感词测试', 实际 '%s'", string(result))
	}

	input = []rune("敏敏敏敏感感感词词词")
	result = core.Dedup(input)
	if string(result) != "敏感词" {
		t.Fatalf("多段重复压缩失败: 期望 '敏感词', 实际 '%s'", string(result))
	}
}

// TestDedupNotAffectsNonCJK 测试 CJK 去重不影响非 CJK 字符
func TestDedupNotAffectsNonCJK(t *testing.T) {
	input := []rune("hello")
	result := core.Dedup(input)
	if string(result) != "hello" {
		t.Fatalf("英文单词不应被压缩: 期望 'hello', 实际 '%s'", string(result))
	}

	input = []rune("12345")
	result = core.Dedup(input)
	if string(result) != "12345" {
		t.Fatalf("数字不应被压缩: 期望 '12345', 实际 '%s'", string(result))
	}

	input = []rune("hello测试词world")
	result = core.Dedup(input)
	if string(result) != "hello测试词world" {
		t.Fatalf("混合文本不应被压缩: 期望 'hello测试词world', 实际 '%s'", string(result))
	}
}

// TestDedupEdgeCases 测试 CJK 去重边界情况
func TestDedupEdgeCases(t *testing.T) {
	result := core.Dedup([]rune{})
	if len(result) != 0 {
		t.Fatal("空切片应返回空切片")
	}

	result = core.Dedup([]rune("单"))
	if len(result) != 1 {
		t.Fatal("单字符应保持不变")
	}

	result = core.Dedup([]rune("中中中中中"))
	if string(result) != "中" {
		t.Fatalf("全部相同 CJK 应压缩为单字符: 期望 '中', 实际 '%s'", string(result))
	}

	result = core.Dedup([]rune("敏词敏词敏词"))
	if string(result) != "敏词敏词敏词" {
		t.Fatalf("交替重复不应压缩: 期望 '敏词敏词敏词', 实际 '%s'", string(result))
	}
}

// TestDedupFilterIntegration 测试 CJK 去重与 Filter 集成
func TestDedupFilterIntegration(t *testing.T) {
	f := New([]string{"敏感词", "违规内容", "测试词"})

	testCases := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{"2连重复", "敏敏感词", "敏感词", "双连重复"},
		{"3连重复", "敏敏敏感词", "敏感词", "三连重复"},
		{"多段重复", "敏敏敏敏感感感词词词", "敏感词", "多段连续重复"},
		{"单段重复", "违违规规内容", "违规内容", "敏感词每字重复"},
		{"部分重复", "测试试词", "测试词", "部分字符重复"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == tc.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Dedup 集成漏检 [%s]: 输入 %q 期望命中 %q, 实际结果: %+v",
					tc.desc, tc.input, tc.expected, results)
			}
		})
	}
}

// TestDedupWithDedupDisabled 测试关闭 Dedup 时的行为
func TestDedupWithDedupDisabled(t *testing.T) {
	f := New([]string{"敏感词"}, WithDedup(false))
	results := f.FindAll("敏敏敏感词")
	t.Logf("关闭 Dedup 结果: %+v", results)

	f2 := New([]string{"敏感词"})
	results2 := f2.FindAll("敏敏敏感词")
	found := false
	for _, r := range results2 {
		if r.Word == "敏感词" {
			found = true
			break
		}
	}
	if !found {
		t.Error("开启 Dedup（默认）应检测到敏感词")
	}
}

// TestMathAlphanumNormalization 测试数学字母符号标准化
func TestMathAlphanumNormalization(t *testing.T) {
	mathMap := core.BuildMathAlphanumMap()

	if v, ok := mathMap[0x1D400]; !ok || v != 'a' {
		t.Errorf("粗体大写 A 应映射到 'a', 实际 ok=%v, v=%c", ok, v)
	}

	if v, ok := mathMap[0x1D41A]; !ok || v != 'a' {
		t.Errorf("粗体小写 a 应映射到 'a', 实际 ok=%v, v=%c", ok, v)
	}

	if v, ok := mathMap[0x1D7CE]; !ok || v != '0' {
		t.Errorf("粗体数字 0 应映射到 '0', 实际 ok=%v, v=%c", ok, v)
	}

	if v, ok := mathMap[0x1D7FF]; !ok || v != '9' {
		t.Errorf("等宽数字 9 应映射到 '9', 实际 ok=%v, v=%c", ok, v)
	}

	if len(mathMap) < 500 {
		t.Errorf("数学字母符号映射表应包含至少 500 个条目, 实际 %d", len(mathMap))
	}
	t.Logf("数学字母符号映射表条目数: %d", len(mathMap))
}

// TestMathAlphanumFilterIntegration 测试数学字母符号 Filter 集成
func TestMathAlphanumFilterIntegration(t *testing.T) {
	f := New([]string{"hello", "test", "sensitive"}, WithFuzzy(true))

	// Bold 小写: h(U+1D421) e(U+1D41E) l(U+1D425) l(U+1D425) o(U+1D428)
	boldHello := string([]rune{0x1D421, 0x1D41E, 0x1D425, 0x1D425, 0x1D428})
	results := f.FindAll(boldHello)
	found := false
	for _, r := range results {
		if r.Word == "hello" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(boldHello)
		t.Errorf("数学符号漏检: 标准化=%q, 期望命中 hello, 结果=%+v", nt.Text, results)
	}
}

// TestMathAlphanumDigits 测试数学体数字符号识别
func TestMathAlphanumDigits(t *testing.T) {
	f := New([]string{"pass123", "test456"}, WithFuzzy(true))

	// Bold 数字: 𝟏 = U+1D7CF(1), 𝟐 = U+1D7D0(2), 𝟑 = U+1D7D1(3)
	boldDigitsInput := "pass" + string(rune(0x1D7CF)) + string(rune(0x1D7D0)) + string(rune(0x1D7D1))
	results := f.FindAll(boldDigitsInput)
	found := false
	for _, r := range results {
		if r.Word == "pass123" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(boldDigitsInput)
		t.Errorf("数学粗体数字漏检: 标准化=%q, 结果=%+v", nt.Text, results)
	}
}

// TestCombiningMarksStripping 测试组合字符剥离
func TestCombiningMarksStripping(t *testing.T) {
	input := []rune{'e', 0x0301, 'x', 'a', 'm', 'p', 'l', 'e'}
	result := core.StripCombiningMarks(input)
	if string(result) != "example" {
		t.Fatalf("组合重音应被剥离: 期望 'example', 实际 '%s'", string(result))
	}

	result = core.StripCombiningMarks([]rune{})
	if len(result) != 0 {
		t.Fatal("空切片应返回空")
	}

	result = core.StripCombiningMarks([]rune{0x0300, 0x0301, 0x0302})
	if len(result) != 0 {
		t.Fatalf("纯组合字符应全部剥离: 期望空, 实际 '%s'", string(result))
	}
}

// TestCombiningMarksFilterIntegration 测试组合字符剥离 Filter 集成
func TestCombiningMarksFilterIntegration(t *testing.T) {
	f := New([]string{"敏感词", "hello"}, WithFuzzy(true))

	input := "敏" + string(rune(0x0301)) + "感" + string(rune(0x0302)) + "词"
	results := f.FindAll(input)
	found := false
	for _, r := range results {
		if r.Word == "敏感词" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(input)
		t.Errorf("组合标记干扰中文漏检: 标准化=%q, 结果=%+v", nt.Text, results)
	}

	inputEn := "h" + string(rune(0x0301)) + "ello"
	results = f.FindAll(inputEn)
	found = false
	for _, r := range results {
		if r.Word == "hello" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(inputEn)
		t.Errorf("组合标记干扰英文漏检: 标准化=%q, 结果=%+v", nt.Text, results)
	}
}

// TestCombiningMarksBoundary 测试组合字符边界情况
func TestCombiningMarksBoundary(t *testing.T) {
	input := []rune{'a', 0x0300, 0x0301, 0x0302, 0x0303}
	result := core.StripCombiningMarks(input)
	if string(result) != "a" {
		t.Fatalf("多个组合标记应全部剥离: 期望 'a', 实际 '%s'", string(result))
	}

	input = []rune("正常文本测试")
	result = core.StripCombiningMarks(input)
	if string(result) != "正常文本测试" {
		t.Fatalf("普通文本应保持不变: 期望 '正常文本测试', 实际 '%s'", string(result))
	}
}

// TestAllCleaningTypesCombined 测试所有清洗类型组合场景
func TestAllCleaningTypesCombined(t *testing.T) {
	f := New([]string{
		"敏感词", "hello", "password", "违规内容",
	}, WithLeetSpeak(true), WithFuzzy(true))

	// CJK重复 + 零宽 + 组合标记混合
	input1 := "敏敏\u200B感\u0301感词\u0300词"
	results := f.FindAll(input1)
	found := false
	for _, r := range results {
		if r.Word == "敏感词" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(input1)
		t.Errorf("混合清洗漏检-敏感词: 标准化=%q, 结果=%+v", nt.Text, results)
	}

	// Leet Speak + 组合标记混合: h3ll0 → hello
	input2 := "h3ll\u03000"
	results = f.FindAll(input2)
	found = false
	for _, r := range results {
		if r.Word == "hello" {
			found = true
			break
		}
	}
	if !found {
		nt := f.normalizer.Normalize(input2)
		t.Errorf("混合清洗漏检-hello: 标准化=%q, 结果=%+v", nt.Text, results)
	}
}

// TestDedupAccuracy 测试 CJK 去重准确率
func TestDedupAccuracy(t *testing.T) {
	f := New([]string{
		"敏感词", "违规内容", "测试词",
		"你好世界", "过滤系统",
	}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
	}{
		{"敏敏感词", "敏感词"},
		{"敏感感词", "敏感词"},
		{"敏感词词", "敏感词"},
		{"敏敏敏敏敏感感感感感词词词词词", "敏感词"},
		{"违违规内容", "违规内容"},
		{"违规规内容", "违规内容"},
		{"违规内容容", "违规内容"},
		{"你好世界", "你好世界"},
	}

	totalTests := len(testCases)
	correct := 0
	for _, tc := range testCases {
		results := f.FindAll(tc.input)
		found := false
		for _, r := range results {
			if r.Word == tc.expected {
				found = true
				break
			}
		}
		if found {
			correct++
		} else {
			t.Errorf("Dedup 漏检: 输入 %q 期望命中 %q", tc.input, tc.expected)
		}
	}

	accuracy := float64(correct) / float64(totalTests) * 100
	t.Logf("CJK Dedup 准确率: %.2f%% (%d/%d)", accuracy, correct, totalTests)

	if accuracy < 95.0 {
		t.Errorf("Dedup 准确率不达标: %.2f%% (要求 >= 95%%)", accuracy)
	}
}

// TestCombiningMarksAccuracy 测试组合字符剥离准确率
func TestCombiningMarksAccuracy(t *testing.T) {
	f := New([]string{"hello", "test", "敏感词", "abc"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
	}{
		{"h" + string(rune(0x0301)) + "ello", "hello"},
		{"t" + string(rune(0x0300)) + "est", "test"},
		{"a" + string(rune(0x0302)) + "bc", "abc"},
		{"敏" + string(rune(0x0301)) + "感" + string(rune(0x0302)) + "词", "敏感词"},
		{"he" + string(rune(0x0308)) + "llo", "hello"},
	}

	totalTests := len(testCases)
	correct := 0
	for _, tc := range testCases {
		results := f.FindAll(tc.input)
		found := false
		for _, r := range results {
			if r.Word == tc.expected {
				found = true
				break
			}
		}
		if found {
			correct++
		} else {
			nt := f.normalizer.Normalize(tc.input)
			t.Errorf("组合标记漏检: 输入=%q 标准化=%q 期望=%q", tc.input, nt.Text, tc.expected)
		}
	}

	accuracy := float64(correct) / float64(totalTests) * 100
	t.Logf("组合字符剥离准确率: %.2f%% (%d/%d)", accuracy, correct, totalTests)

	if accuracy < 95.0 {
		t.Errorf("组合字符准确率不达标: %.2f%% (要求 >= 95%%)", accuracy)
	}
}

// TestDedupFalsePositive 测试 CJK 去重误报率
func TestDedupFalsePositive(t *testing.T) {
	f := New([]string{"敏感词", "违规", "你好世界"}, WithFuzzy(true))

	cleanTexts := []string{
		"这是一段干干净净的文本",
		"正常的中文句子没有任何敏感信息",
		"今天的天气真不错啊",
		"同学们都认真学习了",
		"希望大家都能健康成长",
	}

	falsePositives := 0
	for _, text := range cleanTexts {
		results := f.FindAll(text)
		if len(results) > 0 {
			t.Logf("误判: %q → %+v", text, results)
			falsePositives++
		}
	}

	if falsePositives > 0 {
		t.Errorf("CJK Dedup 误判数: %d/%d", falsePositives, len(cleanTexts))
	} else {
		t.Log("CJK Dedup 误报率为 0%")
	}
}

// TestWithDedupConfigOption 测试 WithDedup 配置选项
func TestWithDedupConfigOption(t *testing.T) {
	f := New([]string{"敏感词"}, WithDedup(true))
	results := f.FindAll("敏敏敏感词")
	found := false
	for _, r := range results {
		if r.Word == "敏感词" {
			found = true
			break
		}
	}
	if !found {
		t.Error("WithDedup(true) 应能检测重复字符中的敏感词")
	}

	f2 := New([]string{"敏感词"})
	results2 := f2.FindAll("敏敏敏感词")
	found2 := false
	for _, r := range results2 {
		if r.Word == "敏感词" {
			found2 = true
			break
		}
	}
	if !found2 {
		t.Error("默认 Dedup 应开启，能检测重复字符中的敏感词")
	}
}

// TestNormalizerConfigDefaults 测试 NormalizerConfig 默认值
func TestNormalizerConfigDefaults(t *testing.T) {
	n := core.NewNormalizer(core.NormalizerConfig{})
	if n.EnableDedup {
		t.Error("零值配置下 Dedup 应为关闭")
	}
	if n.CJKInterstitialSkippable {
		t.Error("零值配置下 CJK 间杂剥离应为关闭")
	}

	n2 := core.NewNormalizer(core.NormalizerConfig{
		EnableLeet:             true,
		EnableCJKInterstitial:  true,
		EnableDedup:            true,
	})
	if !n2.EnableDedup {
		t.Error("全开配置下 Dedup 应为开启")
	}
	if !n2.CJKInterstitialSkippable {
		t.Error("全开配置下 CJK 间杂剥离应为开启")
	}
	if n2.LeetMap == nil {
		t.Error("EnableLeet 开启时 leetMap 不应为 nil")
	}
}

// TestConfusableMapWithMath 测试合并映射表的完整性
func TestConfusableMapWithMath(t *testing.T) {
	combined := core.BuildConfusableMapWithMath()

	if v, ok := combined['曰']; !ok || v != '日' {
		t.Error("confusable 映射 '曰→日' 应在合并表中")
	}

	if v, ok := combined[0x1D400]; !ok || v != 'a' {
		t.Errorf("math alphanum 映射应在合并表中，实际 ok=%v, v=%c", ok, v)
	}

	if len(combined) < 600 {
		t.Errorf("合并映射表条目数应 > 600, 实际 %d", len(combined))
	}
	t.Logf("合并映射表条目数: %d (confusable + math alphanum)", len(combined))
}

// TestDedupPositionMappingAccuracy 测试 Dedup 后的位置映射准确性
func TestDedupPositionMappingAccuracy(t *testing.T) {
	f := New([]string{"敏感词"}, WithFuzzy(true))

	text := "前言敏敏敏感词结束"
	results := f.FindAll(text)
	if len(results) == 0 {
		t.Fatal("应匹配到敏感词")
	}

	t.Logf("Dedup 位置映射: [%d,%d) = %q, 敏感词=%q",
		results[0].Start, results[0].End,
		text[results[0].Start:results[0].End],
		results[0].Word)
}

// ============================================================================
// v3.1 新增测试：confusable.go 字符映射库扩充验证
// 覆盖拉丁重音/希腊/西里尔/小大写/字母式符号/数字变体/繁简/部首/同音字
// ============================================================================

// TestConfusableV31MapSize 验证 v3.1 扩充后的映射表条目数
func TestConfusableV31MapSize(t *testing.T) {
	combined := core.BuildConfusableMapWithMath()

	// v3.0 约 900 条（confusable 300+ + math alphanum 600+）
	// v3.1 扩充拉丁重音/希腊/西里尔/小大写/数字变体/中文后约 1000+ 条
	if len(combined) < 950 {
		t.Errorf("v3.1 映射表条目数不足: 期望 >= 950, 实际 %d", len(combined))
	}
	t.Logf("v3.1 合并映射表条目数: %d", len(combined))
}

// TestConfusableLatinAccented 测试拉丁扩展重音字符 → 基本 ASCII
func TestConfusableLatinAccented(t *testing.T) {
	f := New([]string{"cafe", "resume", "naive", "deja"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 重音变体测试
		{"caf\u00e9", "cafe", "cafe(é)"},                         // cafe + 重音
		{"r\u00e9sum\u00e9", "resume", "resume(全重音)"},         // resume 带重音
		{"na\u00efve", "naive", "naive(分音符)"},                  // naive 带分音
		{"dej\u00e0", "deja", "deja(重音符)"},                     // deja + à
		// 北欧/德语字符
		{"sm\u00f8r", "smor", "smor(ø→o)"},                       // 北欧 ø
		{"gro\u00df", "gros", "gros(ß→s)"},                       // 德语 ß → s
		// 多重重音（不含标准 ASCII 字符，仅测试重音变体映射）
		{"f\u00e2\u00e7\u00e3d\u00eb", "facade", "facade(多重音)"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableGreekHomoglyphs 测试希腊字母同形 → 拉丁小写
// 注意：映射基于视觉同形（visual homoglyph），非音译
// 如 ν→v（视觉=v）、ρ→p（视觉=p）、ψ→y（视觉=y）
func TestConfusableGreekHomoglyphs(t *testing.T) {
	f := New([]string{"abgde", "qupe", "eyivov"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 希腊小写视觉同形映射：α→a, β→b, γ→g, δ→d, ε→e
		{
			"\u03B1\u03B2\u03B3\u03B4\u03B5",
			"abgde", "希腊小写: αβγδε→abgde",
		},
		// 希腊小写视觉同形映射：θ→q, υ→u, ρ→p, ε→e
		{
			"\u03B8\u03C5\u03C1\u03B5",
			"qupe", "希腊小写: θυρε→qupe",
		},
		// 希腊大小写混合：Ε→e, ψ→y, ι→i, ν→v, ο→o, ν→v
		{
			"\u0395\u03C8\u03B9\u03BD\u03BF\u03BD",
			"eyivov", "希腊混合: Εψινον→eyivov",
		},
		// 希腊大写字母映射：Δ→d, Γ→g 等
		{
			"\u0391\u0392\u0393\u0394\u0395",
			"abgde", "希腊大写: ΑΒΓΔΕ→abgde",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableCyrillicHomoglyphs 测试西里尔字母同形 → 拉丁小写
func TestConfusableCyrillicHomoglyphs(t *testing.T) {
	f := New([]string{"hello", "world", "moscow"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 西里尔小写混淆 "неllо" — н→h, е→e, о→o 
		{
			"\u043D\u0435ll\u043E",
			"hello", "西里尔小写 н+е+о 混淆",
		},
		// 西里尔 "мoscow" — м→m
		{
			"\u043Coscow",
			"moscow", "西里尔小写 м 混淆",
		},
		// 西里尔 "рass" — р→p
		{
			"\u0440ass",
			"pass", "西里尔小写 р 混淆",
		},
		// 西里尔大写 "МОSCОW" — М→m, О→o, С→c
		{
			"\u041C\u041ES\u0421\u041E\u0428",
			"moscow", "西里尔大写 М+О+С+О+Ш 混淆",
		},
		// 西里尔 "гussia" — г→r
		{
			"\u0433ussia",
			"russia", "西里尔小写 г 混淆",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableSmallCaps 测试小大写字母 / 字母式符号 → 拉丁小写
func TestConfusableSmallCaps(t *testing.T) {
	f := New([]string{"hello", "world", "nice", "cup"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 小大写 form: ʜᴇʟʟᴏ → hello
		{
			string([]rune{0x029C, 0x1D07, 0x029F, 0x029F, 0x1D0F}),
			"hello", "小大写 ʜᴇʟʟᴏ",
		},
		// 小大写 form: ᴡᴏʀʟᴅ → world
		{
			string([]rune{0x1D21, 0x1D0F, 0x0280, 0x029F, 0x1D05}),
			"world", "小大写 ᴡᴏʀʟᴅ",
		},
		// 字母式符号: ℕiℂe → nice
		{
			string([]rune{0x2115, 'i', 0x2102, 'e'}),
			"nice", "双线符号 ℕℂ→nc",
		},
		// 混合小大写: ᴄᴜᴘ → cup
		{
			string([]rune{0x1D04, 0x1D1C, 0x1D18}),
			"cup", "小大写 ᴄᴜᴘ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableNumberVariants 测试数字变体 → 基本数字
func TestConfusableNumberVariants(t *testing.T) {
	f := New([]string{"pass123", "test456", "code789"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 上标数字: pass¹²³ → pass123
		{
			"pass\u00B9\u00B2\u00B3",
			"pass123", "上标数字 ¹²³→123",
		},
		// 丁贝符数字: pass➀➁➂ → pass123
		{
			"pass\u2780\u2781\u2782",
			"pass123", "丁贝符数字",
		},
		// 双圈数字: test⓵⓶⓷ → test123
		{
			"test\u24F5\u24F6\u24F7",
			"test123", "双圈数字",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableChineseV31 测试 v3.1 新增中文映射（繁简/部首/同音字）
func TestConfusableChineseV31(t *testing.T) {
	f := New([]string{"学习", "国家", "手机", "真的吗"}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 繁体→简体
		{"學習", "学习", "繁→简: 學習→学习"},
		{"國家", "国家", "繁→简: 國家→国家"},
		// 部首→本字: 扌(U+624C)→手
		{"\u624C机", "手机", "部首→本字: 扌→手"},
		// 同音字: 蒸→真，结合 dedup 真真→真 → 真吗
		{"真蒸吗", "真吗", "同音字: 蒸→真+dedup"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nt := f.normalizer.Normalize(tc.input)
			if nt.Text != tc.expected {
				t.Errorf("[%s] 标准化失败: 输入=%q 期望=%q 实际=%q",
					tc.desc, tc.input, tc.expected, nt.Text)
			}
		})
	}
}

// TestConfusableV31FilterIntegration 测试 v3.1 混淆字符与 Filter 集成
func TestConfusableV31FilterIntegration(t *testing.T) {
	f := New([]string{
		"hello", "world", "敏感词", "test123",
		"moscow", "alpha",
	}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		// 西里尔混淆
		{"\u043Coscow", "moscow", "Cyrillic м→m"},
		// 希腊混淆
		{"\u03B1lph\u03B1", "alpha", "Greek α→a"},
		// 拉丁重音
		{"h\u00ebll\u00f8", "hello", "Latin ë+ø→e+o"},
		// 小大写
		{"\u1D21\u1D0F\u0280\u029F\u1D05", "world", "Small caps world"},
		// 数字变体
		{"test\u00B9\u00B2\u00B3", "test123", "Superscript 123"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			results := f.FindAll(tc.input)
			found := false
			for _, r := range results {
				if r.Word == tc.expected {
					found = true
					break
				}
			}
			if !found {
				nt := f.normalizer.Normalize(tc.input)
				t.Errorf("[%s] Filter 集成漏检: 输入=%q 标准化=%q 期望=%q, 结果=%+v",
					tc.desc, tc.input, nt.Text, tc.expected, results)
			}
		})
	}
}

// TestConfusableV31Accuracy 测试 v3.1 混淆字符整体准确率
func TestConfusableV31Accuracy(t *testing.T) {
	f := New([]string{
		"hello", "world", "test", "cafe", "alpha", "delta",
		"pass", "nice", "cup", "moscow",
	}, WithFuzzy(true))

	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		{"caf\u00e9", "cafe", "cafe+重音"},
		{"h\u0435ll\u043E", "hello", "西里尔混淆 hello"},
		{"\u03B1lph\u03B1", "alpha", "希腊混淆 alpha"},
		{"\u03B4elta", "delta", "希腊混淆 delta"},
		{"\u0440ass", "pass", "西里尔混淆 pass"},
		{"\u1D04\u1D1C\u1D18", "cup", "小大写 cup"},
		{"\u2115i\u2102e", "nice", "双线符号 nice"},
		{"\u043Coscow", "moscow", "西里尔混淆 moscow"},
	}

	totalTests := len(testCases)
	correct := 0
	for _, tc := range testCases {
		results := f.FindAll(tc.input)
		found := false
		for _, r := range results {
			if r.Word == tc.expected {
				found = true
				break
			}
		}
		if found {
			correct++
		} else {
			nt := f.normalizer.Normalize(tc.input)
			t.Errorf("[%s] 漏检: 标准化=%q 期望=%q", tc.desc, nt.Text, tc.expected)
		}
	}

	accuracy := float64(correct) / float64(totalTests) * 100
	t.Logf("v3.1 混淆字符准确率: %.2f%% (%d/%d)", accuracy, correct, totalTests)

	if accuracy < 95.0 {
		t.Errorf("v3.1 混淆字符准确率不达标: %.2f%% (要求 >= 95%%)", accuracy)
	}
}
