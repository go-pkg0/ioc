package ioc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// testDriver 测试用驱动实现。
type testDriver struct {
	name   string
	closed bool
}

func (d *testDriver) Name() string { return d.name }

func (d *testDriver) Close(_ context.Context) error {
	d.closed = true
	return nil
}

func TestDriverManagerBasic(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("primary")

	mgr.Register("primary", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "primary"}, nil
	})
	mgr.Register("secondary", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "secondary"}, nil
	})

	d, err := mgr.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "primary" {
		t.Fatalf("expected primary, got %s", d.Name())
	}

	d2, err := mgr.Driver(ctx, "secondary")
	if err != nil {
		t.Fatal(err)
	}
	if d2.Name() != "secondary" {
		t.Fatalf("expected secondary, got %s", d2.Name())
	}
}

func TestDriverManagerCaching(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")

	var count int
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		count++
		return &testDriver{name: "default"}, nil
	})

	mgr.Driver(ctx, "default")
	mgr.Driver(ctx, "default")

	if count != 1 {
		t.Fatalf("factory should be called once, got %d", count)
	}
}

func TestDriverManagerNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")

	_, err := mgr.Driver(ctx, "nonexistent")
	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}
}

func TestDriverManagerSetDefault(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("a")
	mgr.Register("a", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "a"}, nil
	})
	mgr.Register("b", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "b"}, nil
	})

	mgr.SetDefault("b")
	d, err := mgr.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "b" {
		t.Fatalf("expected b, got %s", d.Name())
	}
}

func TestDriverManagerExtend(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "default"}, nil
	})

	mgr.Extend("default", func(original *testDriver) (*testDriver, error) {
		return &testDriver{name: original.name + "+extended"}, nil
	})

	d, err := mgr.Driver(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "default+extended" {
		t.Fatalf("expected 'default+extended', got %s", d.Name())
	}
}

func TestDriverManagerExtendPanicsOnUnregistered(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("default")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Extend should panic on unregistered driver")
		}
	}()

	mgr.Extend("nonexistent", func(original *testDriver) (*testDriver, error) {
		return original, nil
	})
}

func TestDriverManagerDrivers(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("a")
	mgr.Register("a", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "a"}, nil
	})
	mgr.Register("b", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "b"}, nil
	})

	names := mgr.Drivers()
	if len(names) != 2 {
		t.Fatalf("expected 2 drivers, got %d", len(names))
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["a"] || !nameSet["b"] {
		t.Fatalf("missing drivers: %v", names)
	}
}

func TestDriverManagerConcurrency(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")

	var count atomic.Int32
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		count.Add(1)
		return &testDriver{name: "default"}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := mgr.Driver(ctx, "default")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if d.Name() != "default" {
				t.Errorf("unexpected name: %s", d.Name())
			}
		}()
	}
	wg.Wait()

	if c := count.Load(); c != 1 {
		t.Fatalf("factory should be called once, got %d", c)
	}
}

func TestDriverManagerFactoryError(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("default")

	callCount := 0
	mgr.Register("default", func(_ context.Context) (*testDriver, error) {
		callCount++
		return nil, errors.New("init failed")
	})

	_, err := mgr.Default(ctx)
	if err == nil {
		t.Fatal("expected error")
	}

	_, err = mgr.Default(ctx)
	if err == nil {
		t.Fatal("expected cached error")
	}
	if callCount != 1 {
		t.Fatalf("factory should be called exactly once (error cached), got %d", callCount)
	}
}

func TestDriverManagerClose(t *testing.T) {
	ctx := context.Background()
	mgr := NewDriverManager[*testDriver]("a")

	mgr.Register("a", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "a"}, nil
	})
	mgr.Register("b", func(_ context.Context) (*testDriver, error) {
		return &testDriver{name: "b"}, nil
	})

	dA, _ := mgr.Driver(ctx, "a")

	err := mgr.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !dA.closed {
		t.Fatal("driver a should be closed")
	}
}
