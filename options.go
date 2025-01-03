package ioc

// AppOption 配置 Application 的函数式选项。
type AppOption func(*Application)

// WithContainer 使用自定义的 Container 实现。
// 若不指定，Application 会自动创建默认容器。
func WithContainer(c Container) AppOption {
	return func(a *Application) {
		a.container = c
	}
}
