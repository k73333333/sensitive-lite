package sensitive

// Option 过滤器配置函数类型
// 采用函数式选项模式，允许用户在初始化时灵活组合配置项
type Option func(*options)

// options 过滤器内部配置结构体
// 所有字段均为私有，通过 Option 函数进行设置
type options struct {
	// enableFuzzy 是否启用反清洗识别（空格拆分、形近字、特殊字符等）
	enableFuzzy bool
	// replacement 敏感词替换字符，默认为 '*'
	replacement rune
	// maxWordLen 单个敏感词最大长度限制（rune 单位），0 表示不限制
	// 设置后能限制 DFA 树的深度，减少内存占用
	maxWordLen int
	// lazyBuild 是否启用懒加载构建模式
	// 启用后仅在首次过滤时才构建 DFA，适合冷启动优化场景
	lazyBuild bool
}

// defaultOptions 返回默认配置
func defaultOptions() *options {
	return &options{
		enableFuzzy: true,
		replacement: '*',
		maxWordLen:  0,
		lazyBuild:   false,
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
