package ioc

import "context"

// ResolveFunc 服务解析函数签名。
// ctx 由 Make 传入，沿中间件链传递，用于超时控制和链路追踪。
type ResolveFunc func(ctx context.Context) (any, error)

// Middleware 全局中间件 — 拦截所有 Make 调用。
//
// 中间件接收被解析的服务名称和下一个解析函数，返回包装后的解析函数。
// 按注册顺序形成洋葱模型：先注册的中间件在最外层。
//
// 仅在服务首次创建时执行（单例缓存命中后不再经过中间件链）。
//
// ctx 沿链传递，中间件可在调用 next 前增强 context（如注入 tracing span）。
//
// 适用于：全局日志、指标采集、链路追踪注入。
//
//	c.Use(func(abstract string, next ioc.ResolveFunc) ioc.ResolveFunc {
//	    return func(ctx context.Context) (any, error) {
//	        span, ctx := tracer.Start(ctx, "ioc.resolve."+abstract)
//	        defer span.End()
//	        return next(ctx)
//	    }
//	})
type Middleware func(abstract string, next ResolveFunc) ResolveFunc

// Decorator 服务装饰器 — 针对特定服务的后处理。
//
// 在工厂函数返回后、缓存前执行，可包装/替换/增强实例。
// 多个装饰器按注册顺序依次执行（管道模型）。
// ctx 由 Make 传入，用于装饰器中的初始化操作。
//
// 适用于：连接池代理、读写分离包装、监控埋点、熔断器。
//
//	c.Decorate("db", func(ctx context.Context, instance any, c ioc.Container) (any, error) {
//	    pool := instance.(*ConnectionPool)
//	    return NewReadWriteProxy(pool), nil
//	})
type Decorator func(ctx context.Context, instance any, c Container) (any, error)
