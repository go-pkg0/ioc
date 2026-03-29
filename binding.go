package ioc

// binding 内部绑定结构体。
type binding struct {
	factory   Factory // 工厂函数（Instance 注册时为 nil）
	singleton bool    // 是否为单例
}
