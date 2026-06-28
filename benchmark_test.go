package ioc

import (
	"context"
	"testing"
)

func BenchmarkMakeSingletonCached(b *testing.B) {
	c := New()
	ctx := context.Background()
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "value", nil
	})
	Make[string](ctx, c, "svc") // warm up

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Make[string](ctx, c, "svc")
	}
}

func BenchmarkMakeTransient(b *testing.B) {
	c := New()
	ctx := context.Background()
	Bind(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "value", nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Make[string](ctx, c, "svc")
	}
}

func BenchmarkMakeSingletonCachedParallel(b *testing.B) {
	c := New()
	ctx := context.Background()
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "value", nil
	})
	Make[string](ctx, c, "svc")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Make[string](ctx, c, "svc")
		}
	})
}

func BenchmarkDriverManagerCached(b *testing.B) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "default"}, nil
	})
	mgr.Driver(ctx, "default") // warm up

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.Driver(ctx, "default")
	}
}

func BenchmarkDriverManagerCachedParallel(b *testing.B) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "default"}, nil
	})
	mgr.Driver(ctx, "default")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mgr.Driver(ctx, "default")
		}
	})
}
