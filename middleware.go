package ioc

// ResolveFunc 服务解析函数签名。
type ResolveFunc func() (any, error)

// Middleware 全局中间件 — 拦截所有 Make 调用。
//
// 中间件接收被解析的服务名称和下一个解析函数，返回包装后的解析函数。
// 按注册顺序形成洋葱模型：先注册的中间件在最外层。
//
// 仅在服务首次创建时执行（单例缓存命中后不再经过中间件链）。
//
// 适用于：全局日志、指标采集、链路追踪注入。
//
//	c.Use(func(abstract string, next ioc.ResolveFunc) ioc.ResolveFunc {
//	    return func() (any, error) {
//	        log.Printf("resolving %s", abstract)
//	        return next()
//	    }
//	})
type Middleware func(abstract string, next ResolveFunc) ResolveFunc

// Decorator 服务装饰器 — 针对特定服务的后处理。
//
// 在工厂函数返回后、缓存前执行，可包装/替换/增强实例。
// 多个装饰器按注册顺序依次执行（管道模型）。
//
// 适用于：连接池代理、读写分离包装、监控埋点、熔断器。
//
//	c.Decorate("db", func(instance any, c ioc.Container) (any, error) {
//	    pool := instance.(*ConnectionPool)
//	    return NewReadWriteProxy(pool), nil
//	})
type Decorator func(instance any, c Container) (any, error)
