package ioc

import "context"

// Closeable 优雅关闭契约。
//
// 实现此接口的单例会在 Application.Shutdown 时被反序调用 Close。
// Close 应在 ctx 超时前完成资源释放。
//
// 适用于：数据库连接池、日志缓冲区、MQ 客户端、ES 客户端等。
type Closeable interface {
	Close(ctx context.Context) error
}

// HealthChecker 健康检查契约。
//
// 实现此接口的服务可通过 Application.HealthCheck 被统一探活。
// Health 应在合理时间内返回，可通过 ctx 控制超时。
//
// 适用于：MySQL、Redis、ES、MongoDB 等有连接状态的服务。
type HealthChecker interface {
	Health(ctx context.Context) error
}

// Configurable 动态配置契约。
//
// 实现此接口的服务支持运行时配置热更新，无需重启。
//
// 适用于：日志级别调整、缓存策略变更、连接池参数调优。
type Configurable interface {
	Configure(cfg map[string]any) error
}

// ServiceInfo 服务元信息契约。
//
// 实现此接口的服务可提供自身描述信息，用于注册中心、监控面板等场景。
type ServiceInfo interface {
	Name() string
	Version() string
}
