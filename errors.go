package ioc

import "errors"

var (
	// ErrNotBound Make 调用未注册的名称时返回。
	ErrNotBound = errors.New("ioc: binding not found")

	// ErrNoFactory 绑定存在但无工厂函数时返回。
	ErrNoFactory = errors.New("ioc: binding has no factory")

	// ErrDriverNotFound DriverManager 中驱动未注册时返回。
	ErrDriverNotFound = errors.New("ioc: driver not found")
)
