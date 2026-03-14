package ioc

import "context"

// Closeable 优雅关闭契约。
//
// 实现此接口的单例会在 Container.Close / Application.Shutdown 时被反序调用 Close。
// Close 应在 ctx 超时前完成资源释放。
//
// 适用于：数据库连接池、日志缓冲区、MQ 客户端、ES 客户端等。
type Closeable interface {
	Close(ctx context.Context) error
}

// HealthChecker 健康检查契约。
//
// 实现此接口的服务可通过 Container.HealthCheck 被统一探活（并行执行）。
// Health 应在合理时间内返回，可通过 ctx 控制超时。
//
// 适用于：MySQL、Redis、ES、MongoDB 等有连接状态的服务。
type HealthChecker interface {
	Health(ctx context.Context) error
}

// Configurable 动态配置契约。
//
// 类型参数 T 为具体配置结构体类型，编译期约束配置类型，避免运行时断言。
// 实现此接口的服务支持运行时配置热更新，无需重启。
//
// 适用于：日志级别调整、缓存策略变更、连接池参数调优。
//
// 示例:
//
//	type LogConfig struct {
//	    Level string
//	}
//
//	// Manager 实现 Configurable[LogConfig]
//	func (m *Manager) Configure(cfg LogConfig) error {
//	    m.setLevel(cfg.Level)
//	    return nil
//	}
type Configurable[T any] interface {
	Configure(cfg T) error
}

// ServiceInfo 服务元信息契约。
//
// 实现此接口的服务可提供自身描述信息，用于注册中心、监控面板等场景。
type ServiceInfo interface {
	Name() string
	Version() string
}
