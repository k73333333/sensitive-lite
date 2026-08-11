package sensitive

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// ============================================================================
// Logger 日志接口 — 反清洗全流程日志追踪
//
// 支持自定义日志实现，默认使用标准库 log.Logger 输出到 stdout。
// 可通过 WithLogger 注入自定义日志器（如集成到 ELK、Zap、Logrus 等）。
// ============================================================================

// Logger 日志接口
// 实现者需保证线程安全
type Logger interface {
	// Info 信息级别日志（正常业务流程记录）
	Info(format string, args ...interface{})
	// Warn 警告级别日志（可恢复的异常情况）
	Warn(format string, args ...interface{})
	// Error 错误级别日志（需关注的异常，但不影响系统运行）
	Error(format string, args ...interface{})
	// Debug 调试级别日志（仅在开发/调试阶段启用）
	Debug(format string, args ...interface{})
}

// ============================================================================
// 默认日志实现
// ============================================================================

// defaultLogger 基于标准库 log.Logger 的默认日志实现
type defaultLogger struct {
	logger *log.Logger
	level  LogLevel
	mu     sync.Mutex
}

// LogLevel 日志级别
type LogLevel int

const (
	// LogLevelDebug 调试级别
	LogLevelDebug LogLevel = iota
	// LogLevelInfo 信息级别（默认）
	LogLevelInfo
	// LogLevelWarn 警告级别
	LogLevelWarn
	// LogLevelError 错误级别
	LogLevelError
	// LogLevelOff 关闭日志
	LogLevelOff
)

// newDefaultLogger 创建默认日志器
func newDefaultLogger() *defaultLogger {
	return &defaultLogger{
		logger: log.New(os.Stdout, "[sensitive] ", log.LstdFlags),
		level:  LogLevelInfo,
	}
}

func (l *defaultLogger) log(level LogLevel, prefix string, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	msg := fmt.Sprintf(prefix+format, args...)
	l.logger.Output(3, msg)
}

func (l *defaultLogger) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, "[INFO] ", format, args...)
}

func (l *defaultLogger) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, "[WARN] ", format, args...)
}

func (l *defaultLogger) Error(format string, args ...interface{}) {
	l.log(LogLevelError, "[ERROR] ", format, args...)
}

func (l *defaultLogger) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, "[DEBUG] ", format, args...)
}

// SetLogLevel 设置默认日志器的日志级别
func (l *defaultLogger) SetLogLevel(level LogLevel) {
	l.level = level
}

// ============================================================================
// 空日志器（高性能模式：零日志开销）
// ============================================================================

// noopLogger 空操作日志器，用于高性能场景完全关闭日志输出
type noopLogger struct{}

func (n *noopLogger) Info(string, ...interface{})  {}
func (n *noopLogger) Warn(string, ...interface{})  {}
func (n *noopLogger) Error(string, ...interface{}) {}
func (n *noopLogger) Debug(string, ...interface{}) {}

// ============================================================================
// TraceLogger 溯源追踪日志器 — 审计级日志记录
//
// 在 noopLogger 基础上，增加对每次过滤操作的结构化记录能力。
// 用于安全审计、合规检查、问题回溯等场景。
// ============================================================================

// TraceRecord 单次过滤操作的溯源记录
type TraceRecord struct {
	// Timestamp 操作时间戳
	Timestamp time.Time
	// TextHash 输入文本的哈希值（用于去重统计，取前 8 字符）
	TextHash string
	// TextLen 输入文本长度
	TextLen int
	// MatchCount 命中敏感词数量
	MatchCount int
	// MatchWords 命中的敏感词列表（脱敏后仅保留前 3 个）
	MatchWords []string
	// Duration 本次过滤耗时
	Duration time.Duration
	// ErrMsg 异常信息（正常为空）
	ErrMsg string
}

// TraceCallback 溯源回调函数类型
// 每次过滤操作完成后调用，传入本次操作的溯源记录
type TraceCallback func(record TraceRecord)

// ============================================================================
// AlertCallback 告警回调 — 结构化告警通知
//
// 当系统发生需要运维关注的事件时（降级、性能劣化等），
// 通过此回调通知外部系统（如钉钉/飞书机器人、Prometheus AlertManager 等）。
// ============================================================================

// AlertLevel 告警级别
type AlertLevel int

const (
	// AlertLevelWarn 警告级别（可恢复的异常，如性能劣化）
	AlertLevelWarn AlertLevel = iota
	// AlertLevelCritical 严重级别（需要立即处理，如自动降级）
	AlertLevelCritical
)

// String 返回告警级别可读描述
func (a AlertLevel) String() string {
	switch a {
	case AlertLevelWarn:
		return "WARN"
	case AlertLevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// AlertRecord 告警记录
type AlertRecord struct {
	// Timestamp 告警时间
	Timestamp time.Time
	// Level 告警级别
	Level AlertLevel
	// Title 告警标题（简短描述）
	Title string
	// Message 告警详情
	Message string
	// IsDegraded 当前是否处于降级状态
	IsDegraded bool
}

// AlertCallback 告警回调函数类型
// 当触发告警事件时调用，由调用方决定如何处理（发钉钉/飞书通知、写告警文件等）
type AlertCallback func(record AlertRecord)

// ============================================================================
// 降级策略配置
// ============================================================================

// DegradeConfig 降级策略配置
// 当系统资源不足以支持完整反清洗功能时，自动触发降级以保证核心服务可用
type DegradeConfig struct {
	// MaxMemoryMB 最大内存阈值（MB），超过此值触发降级
	// 0 表示不限制
	MaxMemoryMB int
	// FallbackToExact 降级后是否回退到精确匹配（关闭反清洗）
	FallbackToExact bool
	// MaxMatchDuration 单次匹配最大耗时（毫秒），超过触发告警
	// 0 表示不限制
	MaxMatchDuration int
}

// ============================================================================
// WithLogger / WithTrace / WithDegrade 配置选项
// ============================================================================

// WithLogger 注入自定义日志器
// 参数：实现 Logger 接口的日志器实例
func WithLogger(l Logger) Option {
	return func(o *options) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithTraceCallback 设置溯源追踪回调
// 每次 FindAll/Replace 操作完成后会调用此回调，记录结构化审计日志
// 回调在调用 goroutine 中同步执行，避免耗时操作阻塞过滤流程
func WithTraceCallback(cb TraceCallback) Option {
	return func(o *options) {
		if cb != nil {
			o.traceCallback = cb
		}
	}
}

// WithAlertCallback 设置告警回调
// 当触发降级、性能劣化等告警事件时调用，用于集成外部告警系统
// 回调在触发 goroutine 中同步执行，请确保回调本身轻量（异步通知由调用方实现）
func WithAlertCallback(cb AlertCallback) Option {
	return func(o *options) {
		if cb != nil {
			o.alertCallback = cb
		}
	}
}

// WithDegradeConfig 配置降级策略
// 当系统资源紧张时自动降级以保证核心服务可用
func WithDegradeConfig(cfg DegradeConfig) Option {
	return func(o *options) {
		o.degradeConfig = cfg
	}
}

// WithLogLevel 设置默认日志器的日志级别
// 仅在未注入自定义 Logger 时生效
func WithLogLevel(level LogLevel) Option {
	return func(o *options) {
		o.logLevel = level
	}
}

// hashText 对文本生成简单哈希（取前 8 位用于审计去重）
func hashText(text string) string {
	if len(text) <= 8 {
		return text
	}
	// 简单取前 4 字符 + 后 4 字符的摘要
	h := 0
	for _, r := range text {
		h = h*31 + int(r)
	}
	return fmt.Sprintf("%08x", h)
}
