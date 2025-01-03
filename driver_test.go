package ioc

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// testDriver 测试用驱动实现。
type testDriver struct {
	name string
}

func (d *testDriver) Name() string { return d.name }

func TestDriverManagerBasic(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("primary")

	mgr.Register("primary", func() (*testDriver, error) {
		return &testDriver{name: "primary"}, nil
	})
	mgr.Register("secondary", func() (*testDriver, error) {
		return &testDriver{name: "secondary"}, nil
	})

	// Default
	d, err := mgr.Default()
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "primary" {
		t.Fatalf("expected primary, got %s", d.Name())
	}

	// By name
	d2, err := mgr.Driver("secondary")
	if err != nil {
		t.Fatal(err)
	}
	if d2.Name() != "secondary" {
		t.Fatalf("expected secondary, got %s", d2.Name())
	}
}

func TestDriverManagerCaching(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("default")

	var count int
	mgr.Register("default", func() (*testDriver, error) {
		count++
		return &testDriver{name: "default"}, nil
	})

	mgr.Driver("default")
	mgr.Driver("default")

	if count != 1 {
		t.Fatalf("factory should be called once, got %d", count)
	}
}

func TestDriverManagerNotFound(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("default")

	_, err := mgr.Driver("nonexistent")
	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}
}

func TestDriverManagerSetDefault(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("a")
	mgr.Register("a", func() (*testDriver, error) {
		return &testDriver{name: "a"}, nil
	})
	mgr.Register("b", func() (*testDriver, error) {
		return &testDriver{name: "b"}, nil
	})

	mgr.SetDefault("b")
	d, err := mgr.Default()
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "b" {
		t.Fatalf("expected b, got %s", d.Name())
	}
}

func TestDriverManagerExtend(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("default")
	mgr.Register("default", func() (*testDriver, error) {
		return &testDriver{name: "default"}, nil
	})

	// Extend 注册自定义驱动
	mgr.Extend("custom", func() (*testDriver, error) {
		return &testDriver{name: "custom"}, nil
	})

	d, err := mgr.Driver("custom")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "custom" {
		t.Fatalf("expected custom, got %s", d.Name())
	}
}

func TestDriverManagerDrivers(t *testing.T) {
	mgr := NewDriverManager[*testDriver]("a")
	mgr.Register("a", func() (*testDriver, error) {
		return &testDriver{name: "a"}, nil
	})
	mgr.Register("b", func() (*testDriver, error) {
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
	mgr := NewDriverManager[*testDriver]("default")

	var count atomic.Int32
	mgr.Register("default", func() (*testDriver, error) {
		count.Add(1)
		return &testDriver{name: "default"}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := mgr.Driver("default")
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
	mgr := NewDriverManager[*testDriver]("default")
	mgr.Register("default", func() (*testDriver, error) {
		return nil, errors.New("init failed")
	})

	_, err := mgr.Default()
	if err == nil {
		t.Fatal("expected error")
	}
}
