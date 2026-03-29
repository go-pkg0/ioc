package ioc

import (
	"context"
	"fmt"
)

// MakeTyped 解析服务实例并断言为目标类型 T。
// 解析失败返回底层错误，类型不匹配返回 ErrTypeMismatch。
//
// 示例:
//
//	mgr, err := ioc.MakeTyped[*log.Manager](ctx, c, "log")
func MakeTyped[T any](ctx context.Context, c Container, abstract string) (T, error) {
	val, err := c.Make(ctx, abstract)
	if err != nil {
		var zero T
		return zero, err
	}
	typed, ok := val.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: %q resolved to %T, want %T", ErrTypeMismatch, abstract, val, zero)
	}
	return typed, nil
}

// MustMakeTyped 解析并断言类型，失败时 panic。
// 适用于启动期和测试场景。
//
// 示例:
//
//	mgr := ioc.MustMakeTyped[*log.Manager](ctx, c, "log")
func MustMakeTyped[T any](ctx context.Context, c Container, abstract string) T {
	v, err := MakeTyped[T](ctx, c, abstract)
	if err != nil {
		panic(err)
	}
	return v
}
