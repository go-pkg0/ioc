package ioc

import "context"

// ServiceProvider 定义服务提供者的两阶段生命周期。
//
// 所有模块（log、db、cache、es、mongo 等）的 Provider 必须实现此接口。
//
// 生命周期：
//  1. Register — 所有 Provider 的 Register 先于任何 Boot 执行。
//     该阶段仅绑定工厂函数到容器，禁止解析其他 Provider 注册的依赖。
//  2. Boot — 所有 Provider 完成 Register 后，依次执行 Boot。
//     该阶段允许解析依赖并执行初始化逻辑。
//     ctx 用于控制 Boot 超时和链路追踪。
//
// 示例:
//
//	type LogServiceProvider struct{ Config log.Config }
//
//	func (p *LogServiceProvider) Register(c ioc.Container) error {
//	    cfg := p.Config
//	    c.Singleton("log", func(ctx context.Context, c ioc.Container) (any, error) {
//	        return log.NewManager(cfg)
//	    })
//	    return nil
//	}
//
//	func (p *LogServiceProvider) Boot(ctx context.Context, c ioc.Container) error {
//	    mgr := ioc.MustMakeTyped[*log.Manager](ctx, c, "log")
//	    log.SetDefault(mgr)
//	    return nil
//	}
type ServiceProvider interface {
	Register(c Container) error
	Boot(ctx context.Context, c Container) error
}

// DeferrableProvider 是可选接口，用于标记延迟初始化的服务提供者。
//
// 当 Deferred() 返回 true 时，Application 在 Boot 阶段不会主动触发
// 该 Provider 注册的服务解析，而是等到首次 Make 调用时才执行工厂函数。
//
// 适合启动不常用但初始化开销大的服务（如 Elasticsearch、MongoDB）。
type DeferrableProvider interface {
	ServiceProvider
	Deferred() bool
}
