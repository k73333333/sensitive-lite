# 项目目录结构说明

## 概述

本项目遵循 Go Modules 规范，采用 **包目录 / 非核心目录** 分离的架构。用户通过 `go get` 拉取的是核心功能模块，示例代码、测试数据等附属资源作为独立模块存放，不会混入核心依赖。

---

## 目录结构

```
sensitive-lite/
│
├── go.mod                          # 核心模块定义（go get 拉取的入口）
├── go.sum                          # 依赖校验和
├── .gitignore                      # 排除构建产物、IDE配置、临时文件
│
├── sensitive.go                    # 公开 API：Filter 过滤器（New/FindAll/Replace/Contains）
├── options.go                      # 配置选项：WithFuzzy/WithLeetSpeak/WithDedup 等
├── result.go                       # 结果类型：MatchResult/MatchType/FilterResult
├── logger.go                       # 日志接口：Logger/LogLevel/TraceCallback/AlertCallback
│
├── internal/                       # 内部实现（外部包禁止依赖）
│   └── core/                       # 核心引擎包
│       ├── dfa.go                  #   DFA 多模式匹配引擎（O(n) 时间复杂度）
│       ├── normalizer.go           #   文本标准化器（11 步反清洗管线）
│       ├── confusable.go           #   形近字/同音字映射表（240+ 条目）
│       ├── leet.go                 #   Leet Speak 映射表（数字/符号→字母）
│       ├── dedup.go                #   CJK 连续重复字符压缩
│       ├── math_alphanum.go        #   数学字母数字符号→ASCII 映射
│       └── combining.go            #   Unicode 组合字符剥离
│
├── filter_test.go                  # 单元测试 / 集成测试
├── benchmark_test.go               # 性能基准测试
│
├── examples/                       # 示例代码（独立模块，不会被 go get 核心模块时拉取）
│   ├── go.mod                      #   独立模块定义
│   ├── basic/                      #   基础精确匹配示例
│   ├── fuzzy/                      #   反清洗识别示例
│   └── high_perf/                  #   高性能配置示例
│
├── README.md                       # 项目说明文档
├── PROJECT_STRUCTURE.md            # 本文件：目录结构说明
└── LICENSE                         # MIT 开源许可
```

---

## 核心包目录（会被 `go get` 拉取）

| 目录/文件 | 用途 | 公开性 |
|---|---|---|
| `go.mod` | 模块路径 `github.com/kaidong77/sensitive-lite` | 公开 |
| `*.go`（根目录） | Filter 公开 API（sensitive/options/result/logger） | 公开 |
| `internal/core/` | 核心引擎实现（DFA/Normalizer/映射表） | 内部 |
| `go.sum` | 依赖校验 | 公开 |

用户通过以下命令拉取核心模块：

```bash
go get github.com/kaidong77/sensitive-lite
```

拉取后本地仅有上述核心内容，**不包含** `examples/` 目录（因为 `examples/` 拥有独立 `go.mod`，是一个不同的 Go 模块）。

---

## 非核心目录（不会被 `go get` 拉取）

| 目录/文件 | 用途 | 说明 |
|---|---|---|
| `examples/` | 使用示例代码 | 独立 Go 模块，`go get` 不会自动安装 |
| `filter_test.go` | 测试文件 | Go 编译器在 `go build` 时忽略，但仍随仓库下载 |
| `benchmark_test.go` | 性能基准测试 | 同上 |
| `README.md` | 项目文档 | 仓库元数据 |
| `PROJECT_STRUCTURE.md` | 目录说明 | 项目维护文档 |

---

## 模块解耦设计

### 核心模块（`go.mod`）

```go
module github.com/kaidong77/sensitive-lite
go 1.21
```

### 示例模块（`examples/go.mod`）

```go
module github.com/kaidong77/sensitive-lite/examples
go 1.21

// replace 指令指向本地核心模块
replace github.com/kaidong77/sensitive-lite => ../

require github.com/kaidong77/sensitive-lite v0.0.0
```

用户在本地开发时可以直接运行示例：

```bash
# 从项目根目录运行
go run ./examples/basic/main.go

# 或从 examples 目录运行（使用独立的 go.mod）
cd examples && go run ./basic/main.go
```

### 为什么 `go get` 不会拉取 examples？

因为 `examples/go.mod` 将 `examples/` 声明为一个**独立的 Go 模块**（`github.com/kaidong77/sensitive-lite/examples`），与核心模块（`github.com/kaidong77/sensitive-lite`）是不同的模块路径。

当用户执行 `go get github.com/kaidong77/sensitive-lite` 时，Go 工具链仅拉取核心模块的依赖树，不会自动拉取 `examples` 模块。

---

## `.gitignore` 规则

```
# Go 编译产物（不提交）
*.exe *.test *.out

# IDE 配置（不提交）
.idea/ .vscode/

# 临时文件（不提交）
tmp/ temp/

# 文档目录（不随核心代码发布）
doc/
```

**注意**：`go.sum` 必须提交到仓库（库项目标准做法），确保依赖完整性校验。

---

## 使用指南

### 作为库依赖引入

```go
import "github.com/kaidong77/sensitive-lite"

filter := sensitive.New([]string{"敏感词"}, sensitive.WithFuzzy(true))
```

### 本地开发（完整克隆仓库）

```bash
git clone https://github.com/kaidong77/sensitive-lite.git
cd sensitive-lite

# 运行测试
go test -short ./...

# 运行示例
go run ./examples/basic/main.go
```
