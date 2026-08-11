package sensitive

// Option 过滤器配置函数类型
// 采用函数式选项模式，允许用户在初始化时灵活组合配置项
type Option func(*options)

// options 过滤器内部配置结构体
// 所有字段均为私有，通过 Option 函数进行设置
type options struct {
	// enableFuzzy 是否启用反清洗识别（空格拆分、形近字、特殊字符等）
	enableFuzzy bool
	// enableLeetSpeak 是否启用 Leet Speak 数字/符号→字母映射
	// 默认关闭，开启后数字 0-9 和 @$+ 符号会被映射为对应拉丁字母
	enableLeetSpeak bool
	// enableDedup 是否启用 CJK 连续重复字符压缩
	// 默认开启，如 "敏敏敏感词" → "敏感词"
	enableDedup bool
	// replacement 敏感词替换字符，默认为 '*'
	replacement rune
	// maxWordLen 单个敏感词最大长度限制（rune 单位），0 表示不限制
	// 设置后能限制 DFA 树的深度，减少内存占用
	maxWordLen int
	// lazyBuild 是否启用懒加载构建模式
	// 启用后仅在首次过滤时才构建 DFA，适合冷启动优化场景
	lazyBuild bool
	// logger 日志器实例（nil 时使用默认 noopLogger 以避免日志开销）
	logger Logger
	// traceCallback 溯源追踪回调（nil 时不启用）
	traceCallback TraceCallback
	// alertCallback 告警回调（nil 时不启用）
	alertCallback AlertCallback
	// degradeConfig 降级策略配置
	degradeConfig DegradeConfig
	// logLevel 默认日志器日志级别（仅未注入自定义 Logger 时生效）
	logLevel LogLevel
}

// defaultOptions 返回默认配置
func defaultOptions() *options {
	return &options{
		enableFuzzy: true,
		enableDedup: true, // CJK 重复字符压缩默认开启
		replacement: '*',
		maxWordLen:  0,
		lazyBuild:   false,
		logger:      nil, // nil 表示使用 noopLogger（零开销）
		logLevel:    LogLevelWarn,
	}
}

// WithFuzzy 配置是否启用反清洗识别
// 启用后可识别空格拆分、形近字替换、特殊字符夹杂等干扰手段
// 默认启用
func WithFuzzy(enable bool) Option {
	return func(o *options) {
		o.enableFuzzy = enable
	}
}

// WithLeetSpeak 配置是否启用 Leet Speak 数字/符号→字母映射
// 启用后数字（0-9）和特殊符号（@、$、+）会被映射为对应拉丁字母
// 例如 "h3llo" → "hello", "p@ss" → "pass"
// 适用于存在英文 Leet Speak 混淆风险的场景，默认关闭
func WithLeetSpeak(enable bool) Option {
	return func(o *options) {
		o.enableLeetSpeak = enable
	}
}

// WithDedup 配置是否启用 CJK 连续重复字符压缩
// 启用后 "敏敏敏感词" 中的连续重复 CJK 字符会被压缩为单个字符
// 例如 "敏敏敏感词" → "敏感词"，"违违规规内容" → "违规内容"
// 不影响英文单词（如 "hello" 保持不变），默认开启
func WithDedup(enable bool) Option {
	return func(o *options) {
		o.enableDedup = enable
	}
}

// WithReplacement 配置敏感词替换字符
// 默认为 '*'，可设置为其他字符如 '#'、'X' 等
func WithReplacement(r rune) Option {
	return func(o *options) {
		o.replacement = r
	}
}

// WithMaxWordLen 配置敏感词最大长度限制
// 设置后超长敏感词将被忽略，可有效控制 DFA 内存占用
// 参数 len 为 rune 单位，0 表示不限制
func WithMaxWordLen(length int) Option {
	return func(o *options) {
		if length > 0 {
			o.maxWordLen = length
		}
	}
}

// WithLazyBuild 配置是否启用懒加载构建
// 启用后 DFA 树将在首次过滤调用时才构建，适合初始化阶段内存敏感的场景
func WithLazyBuild(enable bool) Option {
	return func(o *options) {
		o.lazyBuild = enable
	}
}
