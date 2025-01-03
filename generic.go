package ioc

import "fmt"

// MakeTyped 解析服务实例并断言为目标类型 T。
// 解析失败或类型不匹配时返回 T 的零值和错误。
//
// 示例:
//
//	mgr, err := ioc.MakeTyped[*log.Manager](c, "log")
func MakeTyped[T any](c Container, abstract string) (T, error) {
	val, err := c.Make(abstract)
	if err != nil {
		var zero T
		return zero, err
	}
	typed, ok := val.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("ioc: %q resolved to %T, want %T", abstract, val, zero)
	}
	return typed, nil
}

// MustMakeTyped 解析并断言类型，失败时 panic。
// 适用于启动期和测试场景。
//
// 示例:
//
//	mgr := ioc.MustMakeTyped[*log.Manager](c, "log")
func MustMakeTyped[T any](c Container, abstract string) T {
	v, err := MakeTyped[T](c, abstract)
	if err != nil {
		panic(err)
	}
	return v
}
