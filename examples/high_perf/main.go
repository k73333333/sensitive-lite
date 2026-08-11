package main

import (
	"fmt"
	"runtime"
	"time"

	sensitive "github.com/kaidong77/sensitive-lite"
)

// ============================================================================
// 示例 3：高性能与低内存场景
//
// 适用场景：
//   - 低内存机器（< 256MB）上的敏感词过滤
//   - 10 万级以上大规模敏感词库
//   - 高并发请求场景
//
// 性能优化策略：
//   - WithMaxWordLen: 限制敏感词长度，减少 DFA 深度
//   - WithLazyBuild: 延迟 DFA 构建，减少冷启动内存
//   - WithFuzzy(false): 纯精确匹配场景关闭反清洗
//
// 使用方法：
//
//	go run examples/high_perf/main.go
// ============================================================================

func main() {
	fmt.Println("========== 高性能敏感词过滤示例 ==========")
	fmt.Println()

	// ========================================================================
	// 场景 A：大规模词库 + 低内存优化
	// ========================================================================
	fmt.Println("【场景 A】10 万词库 + 低内存优化")
	demoLargeScaleLowMem()

	fmt.Println()

	// ========================================================================
	// 场景 B：高并发过滤
	// ========================================================================
	fmt.Println("【场景 B】高并发过滤性能")
	demoHighConcurrency()

	fmt.Println()

	// ========================================================================
	// 场景 C：延迟构建 + 冷启动优化
	// ========================================================================
	fmt.Println("【场景 C】懒加载 + 冷启动优化")
	demoLazyBuild()

	fmt.Println()
	fmt.Println("========== 全部测试完成 ==========")
}

// demoLargeScaleLowMem 演示大规模词库在低内存机器上的表现
func demoLargeScaleLowMem() {
	// 生成 10 万个中文敏感词
	wordCount := 100000
	fmt.Printf("  生成 %d 个敏感词...\n", wordCount)

	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		words[i] = fmt.Sprintf("敏感词%d测试", i)
	}

	// 记录构建前内存
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// 使用内存优化配置创建过滤器
	start := time.Now()
	filter := sensitive.New(words,
		sensitive.WithMaxWordLen(20),  // 限制最长敏感词
		sensitive.WithFuzzy(false),     // 纯精确匹配场景关闭反清洗
	)

	buildTime := time.Since(start)
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	fmt.Printf("  构建耗时: %v\n", buildTime)
	fmt.Printf("  内存增量: %.2f MB\n", memMB)
	fmt.Printf("  DFA 统计: %+v\n", filter.Stats())

	// 验证过滤功能
	text := "这是一个敏感词12345测试的内容"
	start = time.Now()
	results := filter.FindAll(text)
	matchTime := time.Since(start)

	fmt.Printf("  查找耗时: %v\n", matchTime)
	fmt.Printf("  命中敏感词: %d 个\n", len(results))
}

// demoHighConcurrency 演示高并发场景下的过滤性能
func demoHighConcurrency() {
	// 构建词库（1 万词）
	words := make([]string, 10000)
	for i := 0; i < 10000; i++ {
		words[i] = fmt.Sprintf("词库%d", i)
	}

	filter := sensitive.New(words, sensitive.WithFuzzy(false))

	// 并发执行过滤
	concurrency := 100
	iterations := 1000
	fmt.Printf("  并发数: %d, 每协程执行: %d 次\n", concurrency, iterations)

	done := make(chan bool, concurrency)
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				text := fmt.Sprintf("这是词库%d的测试文本", j%10000)
				filter.FindAll(text)
			}
			done <- true
		}(i)
	}

	// 等待所有协程完成
	for i := 0; i < concurrency; i++ {
		<-done
	}

	elapsed := time.Since(start)
	totalOps := concurrency * iterations
	fmt.Printf("  总操作数: %d\n", totalOps)
	fmt.Printf("  总耗时: %v\n", elapsed)
	fmt.Printf("  平均吞吐: %.0f ops/s\n", float64(totalOps)/elapsed.Seconds())
}

// demoLazyBuild 演示懒加载构建策略
func demoLazyBuild() {
	// 准备词库（不立即构建 DFA）
	words := make([]string, 50000)
	for i := 0; i < 50000; i++ {
		words[i] = fmt.Sprintf("延迟词%d", i)
	}

	// 记录初始化时内存
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// 使用懒加载模式创建过滤器（不触发 DFA 构建）
	filter := sensitive.New(words, sensitive.WithLazyBuild(true))

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	initMem := float64(m2.Alloc-m1.Alloc) / 1024
	fmt.Printf("  初始化阶段内存: %.2f KB（未构建 DFA）\n", initMem)

	// 首次调用触发 DFA 构建
	start := time.Now()
	results := filter.FindAll("延迟词12345的测试")
	firstCallTime := time.Since(start)

	fmt.Printf("  首次调用耗时: %v（含 DFA 构建）\n", firstCallTime)
	fmt.Printf("  命中敏感词: %d 个\n", len(results))

	// 第二次调用（DFA 已构建）
	start = time.Now()
	filter.FindAll("延迟词99999的测试")
	secondCallTime := time.Since(start)
	fmt.Printf("  二次调用耗时: %v（DFA 已构建）\n", secondCallTime)
}
