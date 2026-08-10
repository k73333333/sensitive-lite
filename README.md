# sensitive-lite

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.21-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green)](./LICENSE)

**sensitive-lite** 是一个基于 Go 语言开发的轻量级敏感词过滤组件，专为低内存机器和高并发场景设计。

## 核心特性

- **零预设词库**：组件完全独立，不内置任何敏感词，仅通过用户传入自定义词库初始化
- **低内存消耗**：内存优化 DFA 实现，10 万级敏感词场景下内存占用仅为传统 DFA 的 60%
- **反清洗识别**：精准绕过空格拆分、形近字替换、特殊字符/Emoji 干扰、大小写变形、全半角转换等常见文本干扰
- **高并发安全**：过滤操作无锁设计，多 goroutine 可安全并发调用
- **单文件依赖**：无第三方依赖，仅使用 Go 标准库

## 5 分钟快速接入

### 安装

```bash
go get github.com/sensitive-lite/sensitive-lite
```

### 基础使用

```go
package main

import (
    "fmt"
    sensitive "github.com/sensitive-lite/sensitive-lite"
)

func main() {
    // 1. 准备自定义敏感词库
    words := []string{"敏感词", "违规", "广告"}

    // 2. 创建过滤器（反清洗识别默认开启）
    filter := sensitive.New(words)

    // 3. 查找文本中的所有敏感词
    matches := filter.FindAll("这是一条包含敏感词和违规内容的评论")
    for _, m := range matches {
        fmt.Printf("敏感词: %s, 位置: [%d, %d)\n", m.Word, m.Start, m.End)
    }

    // 4. 替换文本中的敏感词
    result := filter.Replace("这是一条包含敏感词和违规内容的评论")
    fmt.Println(result.Text) // 这是一条包含***和**内容的评论
}
```

## API 接口全量说明

### Filter 类型

| 方法 | 签名 | 说明 |
|------|------|------|
| `New` | `New(words []string, opts ...Option) *Filter` | 创建过滤器实例 |
| `FindAll` | `FindAll(text string) []MatchResult` | 查找所有敏感词，返回匹配详情列表 |
| `Replace` | `Replace(text string) *FilterResult` | 替换敏感词并返回替换后文本和匹配详情 |
| `Contains` | `Contains(text string) bool` | 快速判断文本是否包含任意敏感词 |
| `Stats` | `Stats() map[string]int` | 返回 DFA 统计信息（词数、节点数） |

### MatchResult 类型

```go
type MatchResult struct {
    Word  string    // 匹配到的敏感词
    Start int       // 起始字节偏移
    End   int       // 结束字节偏移（不含）
    Type  MatchType // 匹配类型：MatchExact / MatchFuzzy
}
```

### FilterResult 类型

```go
type FilterResult struct {
    Text    string        // 替换后的文本
    Matches []MatchResult // 匹配详情
    Count   int           // 命中数量
}
```

### 配置选项

| 选项函数 | 参数 | 默认值 | 说明 |
|---------|------|--------|------|
| `WithFuzzy` | `enable bool` | `true` | 是否启用反清洗识别 |
| `WithReplacement` | `r rune` | `'*'` | 敏感词替换字符 |
| `WithMaxWordLen` | `length int` | `0`（不限制） | 单敏感词最大长度限制 |
| `WithLazyBuild` | `enable bool` | `false` | 延迟 DFA 构建（首次调用时才构建） |

## 性能基准测试报告

以下测试在 Intel Core i7-12700H / 16GB RAM / Go 1.21 环境下进行。

### 核心操作性能

| 测试项 | 词库规模 | 操作次数 | 耗时 | 吞吐量 |
|--------|---------|---------|------|--------|
| FindAll（精确） | 10,000 | 100,000 | ~850ms | ~117,000 ops/s |
| FindAll（反清洗） | 5,000 | 100,000 | ~1.2s | ~83,000 ops/s |
| Contains | 10,000 | 100,000 | ~420ms | ~238,000 ops/s |
| Replace | 1,000 | 100,000 | ~680ms | ~147,000 ops/s |

### 并发性能（100 并发）

| 测试项 | 词库规模 | 总操作数 | 耗时 | 吞吐量 |
|--------|---------|---------|------|--------|
| FindAll（并发） | 10,000 | 100,000 | ~350ms | ~285,000 ops/s |
| Contains（并发） | 10,000 | 100,000 | ~180ms | ~555,000 ops/s |

### 内存占用对比

| 词库规模（万字） | 传统 DFA（map方案） | sensitive-lite | 内存节省 |
|-----------------|-------------------|----------------|---------|
| 1 | ~5.2 MB | ~3.1 MB | 40% |
| 5 | ~28 MB | ~16 MB | 43% |
| 10 | ~58 MB | ~32 MB | 45% |

### 低内存机器验证（256MB 限制）

- **10 万敏感词** 场景下内存分配 < 45MB
- 10 并发持续操作 1000 次无 OOM

## 示例代码

项目提供了 3 个可直接运行的示例，覆盖常见业务场景：

```bash
# 基础精确匹配
go run examples/basic/main.go

# 反清洗敏感词识别
go run examples/fuzzy/main.go

# 高性能与低内存优化
go run examples/high_perf/main.go
```

## 项目维护指南

### 目录结构

```
sensitive-lite/
├── go.mod                # Go Module 定义
├── sensitive.go          # 主 API（New, FindAll, Replace, Contains）
├── options.go            # 配置选项（函数式选项模式）
├── dfa.go                # DFA 引擎（节点定义、Trie 构建、匹配）
├── normalizer.go         # 文本标准化器（全半角、大小写、Emoji 剥离）
├── confusable.go         # 形近字/同音字映射表
├── result.go             # 结果类型定义
├── filter_test.go        # 单元测试
├── benchmark_test.go     # 基准测试与内存测试
├── examples/
│   ├── basic/main.go     # 示例1：基础匹配
│   ├── fuzzy/main.go     # 示例2：反清洗识别
│   └── high_perf/main.go # 示例3：高性能优化
└── README.md
```

### 运行测试

```bash
# 运行全部单元测试
go test -v ./...

# 运行基准测试
go test -bench=. -benchmem ./...

# 运行测试并查看覆盖率
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 快速测试（跳过内存和规模测试）
go test -short -v ./...
```

### 贡献指南

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feature/xxx`
3. 提交代码前运行测试确保通过
4. 提交 Pull Request

### 版本规范

遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范：

- `MAJOR`：不兼容的 API 修改
- `MINOR`：向下兼容的功能新增
- `PATCH`：向下兼容的问题修正

### 发布检查清单

- [ ] 单元测试全部通过（覆盖率 ≥ 95%）
- [ ] 基准测试无性能回退
- [ ] 反清洗准确率测试通过（≥ 99.5%）
- [ ] 低内存场景验证通过（< 256MB 稳定运行）
- [ ] 文档更新（README、API 说明）

## License

[MIT](./LICENSE)
