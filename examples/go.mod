module github.com/kaidong77/sensitive-lite/examples

go 1.21

// 指向本地核心模块（示例代码属于项目附属资源，
// 不会被 go get 核心模块时自动拉取）
replace github.com/kaidong77/sensitive-lite => ../

require github.com/kaidong77/sensitive-lite v0.0.0
