package ioc

import (
	"context"
	"errors"
	"testing"
)

// --- 测试用 Provider ---

type orderTracker struct {
	log []string
}

type trackingProvider struct {
	name    string
	tracker *orderTracker
}

func (p *trackingProvider) Register(c Container) error {
	p.tracker.log = append(p.tracker.log, p.name+":register")
	return nil
}

func (p *trackingProvider) Boot(c Container) error {
	p.tracker.log = append(p.tracker.log, p.name+":boot")
	return nil
}

// --- 测试用 DeferrableProvider ---

type deferredProvider struct {
	name    string
	tracker *orderTracker
}

func (p *deferredProvider) Register(c Container) error {
	p.tracker.log = append(p.tracker.log, p.name+":register")
	c.Singleton(p.name, func(_ Container) (any, error) {
		return "deferred-value", nil
	})
	return nil
}

func (p *deferredProvider) Boot(c Container) error {
	p.tracker.log = append(p.tracker.log, p.name+":boot")
	return nil
}

func (p *deferredProvider) Deferred() bool { return true }

// --- 测试用 Closeable ---

type closeableService struct {
	name   string
	closed bool
}

func (s *closeableService) Close(_ context.Context) error {
	s.closed = true
	return nil
}

type closeableErrorService struct {
	name string
}

func (s *closeableErrorService) Close(_ context.Context) error {
	return errors.New("close failed: " + s.name)
}

// --- 测试用 HealthChecker ---

type healthyService struct{}

func (s *healthyService) Health(_ context.Context) error { return nil }

type unhealthyService struct{}

func (s *unhealthyService) Health(_ context.Context) error {
	return errors.New("unhealthy")
}

// --- Tests ---

func TestApplicationBootOrder(t *testing.T) {
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(
		&trackingProvider{name: "log", tracker: tracker},
		&trackingProvider{name: "db", tracker: tracker},
		&trackingProvider{name: "cache", tracker: tracker},
	)

	if err := app.Boot(); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"log:register", "db:register", "cache:register",
		"log:boot", "db:boot", "cache:boot",
	}
	if len(tracker.log) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(tracker.log), tracker.log)
	}
	for i, e := range expected {
		if tracker.log[i] != e {
			t.Fatalf("event[%d] = %q, want %q", i, tracker.log[i], e)
		}
	}
}

func TestApplicationBootIdempotent(t *testing.T) {
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(&trackingProvider{name: "svc", tracker: tracker})

	app.Boot()
	app.Boot() // 第二次应为 no-op

	if len(tracker.log) != 2 {
		t.Fatalf("expected 2 events (idempotent), got %d: %v", len(tracker.log), tracker.log)
	}
}

func TestApplicationDeferrableProvider(t *testing.T) {
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(
		&trackingProvider{name: "log", tracker: tracker},
		&deferredProvider{name: "es", tracker: tracker},
	)

	if err := app.Boot(); err != nil {
		t.Fatal(err)
	}

	// DeferrableProvider 的 Boot 不应被调用
	for _, e := range tracker.log {
		if e == "es:boot" {
			t.Fatal("deferred provider's Boot should not be called")
		}
	}

	// 但 Register 应该被调用
	found := false
	for _, e := range tracker.log {
		if e == "es:register" {
			found = true
		}
	}
	if !found {
		t.Fatal("deferred provider's Register should still be called")
	}
}

func TestApplicationShutdownReverseOrder(t *testing.T) {
	app := NewApp()
	c := app.Container()

	svc1 := &closeableService{name: "first"}
	svc2 := &closeableService{name: "second"}
	svc3 := &closeableService{name: "third"}

	c.Instance("first", svc1)
	c.Instance("second", svc2)
	c.Instance("third", svc3)

	err := app.Shutdown(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !svc1.closed || !svc2.closed || !svc3.closed {
		t.Fatal("all closeable services should be closed")
	}
}

func TestApplicationShutdownAggregatesErrors(t *testing.T) {
	app := NewApp()
	c := app.Container()

	c.Instance("svc1", &closeableErrorService{name: "svc1"})
	c.Instance("svc2", &closeableErrorService{name: "svc2"})

	err := app.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	// errors.Join 的结果应包含两个错误信息
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Fatal("error message should not be empty")
	}
}

func TestApplicationHealthCheck(t *testing.T) {
	app := NewApp()
	c := app.Container()

	c.Instance("healthy", &healthyService{})
	c.Instance("unhealthy", &unhealthyService{})
	c.Instance("plain", "not-a-health-checker")

	result := app.HealthCheck(context.Background())

	// 只有实现 HealthChecker 的服务才会被检查
	if len(result) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(result))
	}
	if result["healthy"] != nil {
		t.Fatal("healthy service should return nil error")
	}
	if result["unhealthy"] == nil {
		t.Fatal("unhealthy service should return error")
	}
}

func TestApplicationWithContainer(t *testing.T) {
	c := New()
	c.Instance("existing", "value")

	app := NewApp(WithContainer(c))
	if app.Container() != c {
		t.Fatal("should use provided container")
	}

	v, err := app.Container().Make("existing")
	if err != nil || v != "value" {
		t.Fatal("should access existing bindings")
	}
}

func TestApplicationRegisterError(t *testing.T) {
	app := NewApp()
	app.Register(&errorProvider{phase: "register"})

	err := app.Boot()
	if err == nil {
		t.Fatal("expected register error")
	}
}

func TestApplicationBootError(t *testing.T) {
	app := NewApp()
	app.Register(&errorProvider{phase: "boot"})

	err := app.Boot()
	if err == nil {
		t.Fatal("expected boot error")
	}
}

type errorProvider struct {
	phase string
}

func (p *errorProvider) Register(c Container) error {
	if p.phase == "register" {
		return errors.New("register failed")
	}
	return nil
}

func (p *errorProvider) Boot(c Container) error {
	if p.phase == "boot" {
		return errors.New("boot failed")
	}
	return nil
}
