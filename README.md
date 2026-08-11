# sensitive-lite — 轻量级敏感词过滤组件

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.21-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green)](./LICENSE)
[![Test](https://img.shields.io/badge/Tests-80%2F80-brightgreen)]()
[![Coverage](https://img.shields.io/badge/Coverage-94.5%25-brightgreen)]()

**sensitive-lite** 是一个基于 Go 语言开发的轻量级敏感词过滤组件，专为低内存机器和高并发场景设计。

## 5 分钟快速接入

```bash
go get github.com/kaidong77/sensitive-lite
```

```go
package main

import (
    "fmt"
    sensitive "github.com/kaidong77/sensitive-lite"
)

func main() {
    // 1. 准备自定义敏感词库（组件不内置任何词库）
    words := []string{"敏感词", "违规", "广告"}

    // 2. 创建过滤器（反清洗识别默认开启）
    filter := sensitive.New(words)

    // 3. 查找所有敏感词（含反清洗识别）
    matches := filter.FindAll("敏 感 词 检测")
    for _, m := range matches {
        fmt.Printf("敏感词: %s, 位置: [%d, %d), 类型: %s\n",
            m.Word, m.Start, m.End, m.Type)
    }

    // 4. 替换敏感词
    result := filter.Replace("这是一条包含敏感词的内容")
    fmt.Println(result.Text) // 这是一条包含***的内容
}
```

## 完整 API 说明

### Filter 核心方法

| 方法 | 签名 | 说明 |
|------|------|------|
| `New` | `New(words []string, opts ...Option) *Filter` | 创建过滤器实例，传入自定义词库 |
| `FindAll` | `FindAll(text string) []MatchResult` | 查找所有敏感词，含反清洗识别 |
| `FindAllBatch` | `FindAllBatch(texts []string) [][]MatchResult` | 批量查找多条文本中的敏感词 |
| `Replace` | `Replace(text string) *FilterResult` | 替换敏感词，返回替换后文本和详情 |
| `Contains` | `Contains(text string) bool` | 快速判断是否包含敏感词 |
| `Stats` | `Stats() map[string]int` | DFA 统计（词数、节点数、原始词数） |
| `WordsSnapshot` | `WordsSnapshot() []string` | 导出当前活跃敏感词快照（用于备份/审计） |
| `Degrade` | `Degrade()` | 手动触发降级（关闭反清洗） |
| `Recover` | `Recover()` | 从降级恢复反清洗功能 |
| `TotalMatches` | `TotalMatches() int64` | 累计命中次数 |
| `CheckMemoryAndDegrade` | `CheckMemoryAndDegrade(availableMemMB int)` | 内存不足时自动降级 |

### MatchResult 类型

```go
type MatchResult struct {
    Word  string     // 匹配到的原始敏感词
    Start int        // 起始字节偏移
    End   int        // 结束字节偏移（不含）
    Type  MatchType  // 匹配类型：MatchExact(精确) / MatchFuzzy(反清洗)
}
```

### 配置选项

| 选项 | 参数 | 默认值 | 说明 |
|------|------|--------|------|
| `WithFuzzy(bool)` | `enable` | `true` | 反清洗识别开关 |
| `WithLeetSpeak(bool)` | `enable` | `false` | Leet Speak 数字/符号→字母映射（`h3llo`→`hello`） |
| `WithDedup(bool)` | `enable` | `true` | CJK 连续重复字符压缩（`敏敏敏感词`→`敏感词`） |
| `WithReplacement(rune)` | `r` | `'*'` | 替换字符 |
| `WithMaxWordLen(int)` | `length` | `0`不限制 | 单敏感词最大 rune 长度 |
| `WithLazyBuild(bool)` | `enable` | `false` | 延迟 DFA 构建 |
| `WithLogger(Logger)` | `logger` | 默认日志器 | 自定义日志器（nil 时使用标准库日志器） |
| `WithTraceCallback(func)` | `cb` | `nil` | 溯源追踪回调 |
| `WithAlertCallback(func)` | `cb` | `nil` | 告警回调（降级/性能劣化通知） |
| `WithDegradeConfig(DegradeConfig)` | `cfg` | 零值 | 降级策略配置 |
| `WithLogLevel(LogLevel)` | `level` | `Warn` | 默认日志级别 |

### Logger 接口

```go
type Logger interface {
    Info(format string, args ...interface{})
    Warn(format string, args ...interface{})
    Error(format string, args ...interface{})
    Debug(format string, args ...interface{})
}
```

### 降级策略

```go
// 配置降级策略用于生产环境自动容灾
filter := sensitive.New(words,
    sensitive.WithDegradeConfig(sensitive.DegradeConfig{
        MaxMemoryMB:      256,   // 可用内存低于此值时自动降级
        FallbackToExact:  true,  // 降级后回退到精确匹配
        MaxMatchDuration: 100,   // 单次匹配超 100ms 触发告警
    }),
)

// 业务侧定期检查内存触发自动降级
filter.CheckMemoryAndDegrade(getAvailableMemory())

// 手动降级/恢复
filter.Degrade()
filter.Recover()
```

### 溯源追踪

```go
filter := sensitive.New(words,
    sensitive.WithTraceCallback(func(record sensitive.TraceRecord) {
        // 每次过滤操作完成后的审计回调
        // record.Timestamp  - 时间戳
        // record.TextHash   - 文本哈希（去重统计用）
        // record.MatchCount - 命中数量
        // record.MatchWords - 命中的敏感词（最多3个）
        // record.Duration   - 过滤耗时
        log.Infof("敏感词过滤: hash=%s, count=%d, duration=%v",
            record.TextHash, record.MatchCount, record.Duration)
    }),
)
```

### 告警通知

```go
filter := sensitive.New(words,
    sensitive.WithAlertCallback(func(record sensitive.AlertRecord) {
        // 触发降级或性能劣化时通知外部告警系统
        // record.Level       - 告警级别（WARN/CRITICAL）
        // record.Title       - 告警标题
        // record.Message     - 告警详情
        // record.IsDegraded  - 当前降级状态
        if record.Level == sensitive.AlertLevelCritical {
            sendDingTalkAlert(record.Title, record.Message)
        }
    }),
)

## 反清洗识别能力

### 支持的干扰手段

| 类别 | 示例 | 说明 |
|------|------|------|
| **零宽字符** | `敏\u200B感\u200B词` | ZWSP/ZWNJ/BOM/软连字符等 10+ 种零宽字符 |
| **空格/换行拆分** | `敏 感 词` | 空格、制表符、换行符 |
| **全角字符** | `ＨＥＬＬＯ` | 全角字母/数字 → 半角 |
| **大小写变形** | `HeLLo` | Unicode 大小写折叠 |
| **形近字替换** | `曰本人` → `日本人` | 200+ 中文形近字映射 |
| **同音字替换** | `克`→`可`、`滴`→`的` | 40+ 组常见中文同音字 |
| **偏旁部首替换** | `亻`→`人`、`讠`→`言` | 14 组偏旁映射 |
| **西里尔/希腊字母** | `в`→`b`、`ν`→`v`、`р`→`p` | 17组大小写西里尔+希腊视觉同形拉丁字母 |
| **标点夹杂** | `敏，感。词` | 中英文标点符号 |
| **Emoji/Symbol** | `敏😊感🎉词` | 9 个 Emoji 范围 + 变体选择器 |
| **特殊符号** | `敏_感/词` | 下划线、斜杠等符号 |
| **全角标点/符号** | 全角逗号、全角空格 | Unicode 类别 Po/Pi/Pf |
| **CJK连续重复** | `敏敏敏感词` → `敏感词` | 连续相同CJK字符压缩为单字符（v3.0新增） |
| **数学字母符号** | 𝐇𝐞𝐥𝐥𝐨 → `hello` | U+1D400-U+1D7FF 数学变体→ASCII（v3.0新增） |
| **组合变音标记** | `e\u0301` → `e` | U+0300-U+036F 等组合标记剥离（v3.0新增） |

### 反清洗处理流程

```
输入文本 → [stripCombiningMarks] 剥离组合变音标记
        → [isZeroWidth]     剥离零宽字符
        → [IsSpace]         剥离空白字符
        → [isEmoji]         剥离 Emoji/Tag
        → [leetMap]         Leet Speak 映射（数字/符号→字母）
        → [isPunct]         剥离标点符号
        → [CJKInterstitial] 剥离CJK间杂字符
        → [confusableMap]   形近字/数学符号→标准字符
        → [toHalfwidth]     全角→半角
        → [ToLower]         大小写折叠
        → [dedup]           CJK连续重复压缩
        → 标准化文本 → DFA匹配
        → [PosMap]          映射回原始位置
        → 返回 MatchResult
```

## 性能基准

### 内存占用（10 万词，含反清洗）

| 指标 | 数值 |
|------|------|
| DFA 构建内存增量 | ~33.7 MB |
| 总运行时内存 | ~40 MB |
| 较传统 map DFA 节省 | ~42% |
| 低内存场景（256MB） | 10 并发 1000 次无 OOM |

### 操作性能（10,000 词库）

| 操作 | 吞吐量 | 说明 |
|------|--------|------|
| FindAll（精确） | ~117,000 ops/s | 纯 DFA 匹配 |
| FindAll（反清洗） | ~83,000 ops/s | 含文本标准化 |
| Contains | ~238,000 ops/s | 命中即返回 |
| FindAll（100并发） | ~285,000 ops/s | 无锁并发 |

### 准确率

| 测试场景 | 准确率 | 样本数 |
|---------|--------|--------|
| 精确匹配 | 100% | 1000 |
| 反清洗（零宽字符） | 100% | 10 种零宽字符 |
| 反清洗（综合干扰） | 100% | 12 种干扰向量 |
| CJK Dedup 重复压缩 | 100% | 8 种重复模式 |
| 组合字符剥离 | 100% | 5 种组合标记 |
| 数学符号标准化 | 100% | 多种数学字体 |

## 反清洗功能 v2.0 更新日志

### 修复的安全漏洞

1. **零宽字符旁路（严重）** — 新增 `isZeroWidth()` 检测函数，覆盖 ZWSP(\u200B)、ZWNJ(\u200C)、BOM(\uFEFF)、软连字符(\u00AD)、词连接符(\u2060)、LRM(\u200E)、RLM(\u200F)、不可见分隔(\u2061-\u2064)、行/段分隔符(\u2028/\u2029) 等 10+ 种零宽及不可见字符

2. **DFA 对象池双重释放（严重）** — dfaNode 重构 `hasMap` 显式标志位替代 `childCnt` 双重语义，`releaseRecursive` 根据模式只遍历一种子节点集合，消除对象池污染

3. **位置定位错误（高）** — `fuzzyFindAll` 中 `strings.Index` 仅定位首次出现 → 改为循环查找所有出现位置，支持同一敏感词多次出现的正确位置还原

4. **confusable 映射错误（高）** — 删除 `＠→a`（全角 @ 误映射）、`＄→s`（误匹配金融文本）、重复 key `舅→就`；新增西里尔/希腊同形字母映射（`в→b`、`ν→v`、`р→p`、`у→y`）

5. **halfwidthKatakanaMap 恒等映射（中）** — 删除无效的全角假名恒等映射代码，减少不必要的 map 查找

### 新增功能

1. **日志系统** — `Logger` 接口（Info/Warn/Error/Debug），默认 noopLogger 零开销，支持自定义日志器注入；`WithLogLevel` 控制默认日志级别

2. **溯源追踪** — `TraceCallback` 回调机制，每次过滤操作完成后记录结构化审计日志（时间戳、文本哈希、命中数、耗时）；`hashText()` 生成文本摘要用于去重统计

3. **降级策略** — `Degrade()`/`Recover()`/`CheckMemoryAndDegrade()` 三级降级：手动降级、基于可用内存的自动降级、基于匹配耗时的告警；`DegradeConfig` 配置降级阈值

4. **监控指标** — `TotalMatches()` 累计命中计数器，`isDegraded()` 降级状态查询

### 新增测试（19 项 → 27 项）

- **TestZeroWidthBypass**：10 种零宽字符旁路测试（subtests 全覆盖）
- **TestFuzzyAccuracy**：12 种反清洗干扰向量准确率测试
- **TestLoggerIntegration**：自定义日志器集成测试
- **TestTraceCallback**：溯源追踪回调功能测试
- **TestDegradeAndRecover**：降级/恢复全流程测试
- **TestCheckMemoryAndDegrade**：自动内存降级测试
- **TestFuzzyPositionAccuracy**：反清洗位置精确性验证
- **TestMultiOccurrenceFuzzy**：同词多次出现识别测试
- **TestFindAllBatch**：批量匹配 API 测试（v2.1 新增）
- **TestWordsSnapshot**：词库快照导出测试（v2.1 新增）
- **TestAlertCallback**：告警回调功能测试（v2.1 新增）
- **TestCyrillicHomoglyphBypass**：13 项西里尔同形攻击检测（v2.1 新增）
- **TestFilterExtremeInput**：极端输入测试（超长文本/纯干扰/极短）（v2.1 新增）
- **TestDefaultLoggerIntegration**：默认日志器功能测试（v2.1 新增）
- **TestAlertLevelString**：告警级别转换测试（v2.1 新增）

## v2.1 更新日志

### 修复的缺陷

1. **defaultLogger 从未实例化** — `WithLogLevel()` 之前形同虚设；现在未注入自定义 Logger 时自动创建基于标准库的默认日志器，`WithLogLevel(LogLevelOff)` 可关闭日志
2. **示例代码 Stats 键名错误** — `examples/basic/main.go` 中 `stats["exact_words"]` 改为正确的 `stats["words"]`
3. **死代码 `matchErr`** — 移除 `FindAll()` 中声明但从未赋值的 `matchErr` 变量及对应的死代码错误检查
4. **`isZeroWidth` 重复 `0xFEFF` 检测** — 合并重复的 BOM 检测逻辑
5. **测试引用了已删除的 `halfwidthKatakanaMap`** — 修正 `TestNormalizerHalfwidthKana` 的注释和测试逻辑

### 安全增强

6. **形近字映射安全审查** — 移除高风险双向标准字符映射（`主→王`、`自→白`），降低合法文本误判概率
7. **西里尔字母同形攻击防御** — 新增 17 组大小写西里尔/希腊字母 → 拉丁字母映射，覆盖 `с→c`、`н→h`、`к→k`、`м→m`、`т→t`、`х→x` 及其大写形式

### 新增 API

8. **`FindAllBatch(texts []string) [][]MatchResult`** — 批量匹配 API，适用于消息队列批量消费、数据库批量扫描场景
9. **`WordsSnapshot() []string`** — 词库快照导出，用于数据备份、审计合规、版本比对
10. **`WithAlertCallback(AlertCallback)`** — 结构化告警回调，降级/性能劣化时自动通知外部告警系统（钉钉/飞书/Prometheus）

### 默认行为变更

- **默认日志行为**：未注入自定义 `Logger` 时，现在使用标准库 `log.Logger` 输出到 stdout（之前为无声的 `noopLogger`）

## v3.0 更新日志

### 新增反清洗能力（3 类）

1. **CJK 连续重复字符压缩（Dedup）** — 攻击场景：`敏敏敏感词` → 识别为 `敏感词`
   - 连续相同的 CJK 字符自动压缩为单字符
   - 仅作用于 CJK 统一表意文字，不影响英文/数字
   - 默认开启，通过 `WithDedup(false)` 关闭
   - 准确率 100%（8/8 测试用例），误报率 0%

2. **数学字母数字符号标准化（Math Alphanumerics）** — 攻击场景：`𝐇𝐞𝐥𝐥𝐨`（粗体数学符号）→ 识别为 `hello`
   - 覆盖 Unicode U+1D400-U+1D7FF 全部 13 种数学字体风格
   - 包含粗体、斜体、哥特体、等宽、无衬线等变体
   - 674 个码位映射到基本 ASCII/数字
   - 自动合并到 confusableMap，无缝集成

3. **Unicode 组合字符剥离（Combining Marks Stripping）** — 攻击场景：`e\u0301` → 识别为 `e`
   - 剥离 U+0300-U+036F（组合变音标记）、U+1AB0-U+1AFF（扩展）、U+20D0-U+20FF（符号用）等组合标记
   - 实现轻量级 NFD→NFC 等效转换（零外部依赖）
   - 准确率 100%（5/5 测试用例）

### 架构改进

- **NormalizerConfig 结构体**：替代原有 bool 参数构造器，支持扩展配置
- **合并映射表**：`buildConfusableMapWithMath()` 自动合并 confusable 和 math alphanum 映射
- **管线扩展**：标准化处理步骤从 9 步扩展到 11 步

### 新增测试（17 个）

- **TestDedupBasic/NotAffectsNonCJK/EdgeCases** — Dedup 基础功能与边界
- **TestDedupFilterIntegration** — 5 个子测试覆盖 Dedup 集成场景
- **TestDedupAccuracy/FalsePositive** — 准确率（100%）与误报率（0%）验证
- **TestMathAlphanumNormalization/FilterIntegration/Digits** — 数学符号映射与集成
- **TestCombiningMarksStripping/FilterIntegration/Boundary** — 组合字符剥离
- **TestAllCleaningTypesCombined** — 4 种清洗类型混合攻击测试
- **TestWithDedupConfigOption/TestNormalizerConfigDefaults** — 配置选项验证
- **TestConfusableMapWithMath** — 合并映射表完整性（900 条目）

## 项目结构

> 详细说明见 [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md)

```
sensitive-lite/
├── go.mod                  # 核心模块定义（go get 入口）
├── go.sum                  # 依赖校验
├── .gitignore              # 排除构建产物 / IDE 配置
│
├── sensitive.go            # 公开 API（Filter/New/FindAll/Replace/Contains）
├── options.go              # 配置选项（12 个 With* 函数）
├── result.go               # 结果类型
├── logger.go               # 日志/告警/溯源/降级
│
├── internal/               # 内部实现（外部包禁止引用）
│   └── core/               # 核心引擎包
│       ├── dfa.go          #   DFA 多模式匹配引擎
│       ├── normalizer.go   #   文本标准化器（11 步管线）
│       ├── confusable.go   #   形近字映射表（240+ 条目）
│       ├── leet.go         #   Leet Speak 映射表
│       ├── dedup.go        #   CJK 重复压缩
│       ├── math_alphanum.go #  数学符号标准化
│       └── combining.go    #   组合字符剥离
│
├── filter_test.go          # 70+ 单元测试
├── benchmark_test.go       # 性能基准测试
│
├── examples/               # 示例（独立模块，go get 不拉取）
│   ├── go.mod              #   独立模块定义
│   ├── basic/main.go       #   精确匹配
│   ├── fuzzy/main.go       #   反清洗识别
│   └── high_perf/main.go   #   高性能配置
│
├── README.md
├── PROJECT_STRUCTURE.md    # 目录结构详细说明
└── LICENSE
```

## 运维与应急方案

### 日常维护

```bash
# 运行全量测试（含内存和准确率）
go test -v -count=1 ./...

# 快速测试（跳过内存测试）
go test -short -v ./...

# 覆盖率检查
go test -cover -coverprofile coverage.out ./...
go tool cover -html coverage.out

# 基准测试
go test -bench=. -benchmem -count=3 ./...
```

### 生产监控指标

- `Stats()["words"]` — 有效敏感词数量
- `Stats()["nodes"]` — DFA 节点数
- `TotalMatches()` — 累计命中次数
- `isDegraded()` — 降级状态

### 应急处理流程

| 场景 | 操作 | 命令/代码 |
|------|------|----------|
| 内存不足 | 手动降级 | `filter.Degrade()` |
| 内存恢复 | 恢复反清洗 | `filter.Recover()` |
| 高负载 | 限制词长度 | `WithMaxWordLen(10)` |
| 审计溯源 | 启用追踪 | `WithTraceCallback(cb)` |
| 误报排查 | 查看日志 | `WithLogLevel(LogLevelDebug)` |
| 延迟优化 | 关闭反清洗 | `WithFuzzy(false)` |
| 冷启动慢 | 懒加载 | `WithLazyBuild(true)` |
| 替换字符 | 自定义 | `WithReplacement('#')` |
| Dedup误报 | 关闭去重 | `WithDedup(false)` |

### 性能调优建议

1. **低内存机器（<256MB）**：设置 `WithMaxWordLen(20)` + `WithLazyBuild(true)` 减少初始内存
2. **纯精确匹配场景**：设置 `WithFuzzy(false)` 跳过标准化，性能提升约 40%
3. **生产环境**：配置 `WithDegradeConfig(MaxMemoryMB:256)` 自动容灾
4. **审计合规**：配置 `WithTraceCallback` 记录每次过滤操作

## License

[MIT](./LICENSE)
