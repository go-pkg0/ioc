package ioc

import "errors"

var (
	// ErrNotBound Make 调用未注册的名称时返回。
	ErrNotBound = errors.New("ioc: binding not found")

	// ErrNoFactory 绑定存在但无工厂函数时返回。
	ErrNoFactory = errors.New("ioc: binding has no factory")

	// ErrDriverNotFound DriverManager 中驱动未注册时返回。
	ErrDriverNotFound = errors.New("ioc: driver not found")

	// ErrCircularDependency Make 检测到循环依赖时返回。
	// 错误消息包含完整依赖链，如 "A -> B -> A"。
	ErrCircularDependency = errors.New("ioc: circular dependency detected")

	// ErrContainerClosed 容器已关闭后调用 Make 时返回。
	ErrContainerClosed = errors.New("ioc: container is closed")

	// ErrTypeMismatch MakeTyped 类型断言失败时返回。
	ErrTypeMismatch = errors.New("ioc: type mismatch")
)
