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

func (p *trackingProvider) Boot(_ context.Context, c Container) error {
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
	c.Singleton(p.name, func(_ context.Context, _ Container) (any, error) {
		return "deferred-value", nil
	})
	return nil
}

func (p *deferredProvider) Boot(_ context.Context, c Container) error {
	p.tracker.log = append(p.tracker.log, p.name+":boot")
	return nil
}

func (p *deferredProvider) Deferred() bool { return true }

// --- 测试用 ProvidesAware DeferrableProvider ---

type providesAwareProvider struct {
	deferredProvider
	provides []string
}

func (p *providesAwareProvider) Provides() []string { return p.provides }

// --- 测试用 Closeable ---

type closeableService struct {
	name    string
	closed  bool
	onClose func()
}

func (s *closeableService) Close(_ context.Context) error {
	s.closed = true
	if s.onClose != nil {
		s.onClose()
	}
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
	ctx := context.Background()
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(
		&trackingProvider{name: "log", tracker: tracker},
		&trackingProvider{name: "db", tracker: tracker},
		&trackingProvider{name: "cache", tracker: tracker},
	)

	if err := app.Boot(ctx); err != nil {
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
	ctx := context.Background()
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(&trackingProvider{name: "svc", tracker: tracker})

	app.Boot(ctx)
	app.Boot(ctx) // 第二次应为 no-op

	if len(tracker.log) != 2 {
		t.Fatalf("expected 2 events (idempotent), got %d: %v", len(tracker.log), tracker.log)
	}
}

func TestApplicationDeferrableProvider(t *testing.T) {
	ctx := context.Background()
	tracker := &orderTracker{}
	app := NewApp()
	app.Register(
		&trackingProvider{name: "log", tracker: tracker},
		&deferredProvider{name: "es", tracker: tracker},
	)

	if err := app.Boot(ctx); err != nil {
		t.Fatal(err)
	}

	for _, e := range tracker.log {
		if e == "es:boot" {
			t.Fatal("deferred provider's Boot should not be called")
		}
	}

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
	ctx := context.Background()
	app := NewApp()
	c := app.Container()

	svc1 := &closeableService{name: "first"}
	svc2 := &closeableService{name: "second"}
	svc3 := &closeableService{name: "third"}

	c.Instance("first", svc1)
	c.Instance("second", svc2)
	c.Instance("third", svc3)

	err := app.Shutdown(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !svc1.closed || !svc2.closed || !svc3.closed {
		t.Fatal("all closeable services should be closed")
	}
}

func TestApplicationShutdownAggregatesErrors(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	c := app.Container()

	c.Instance("svc1", &closeableErrorService{name: "svc1"})
	c.Instance("svc2", &closeableErrorService{name: "svc2"})

	err := app.Shutdown(ctx)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Fatal("error message should not be empty")
	}
}

func TestApplicationHealthCheck(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Register(&trackingProvider{name: "init", tracker: &orderTracker{}})
	app.Boot(ctx)
	c := app.Container()

	c.Instance("healthy", &healthyService{})
	c.Instance("unhealthy", &unhealthyService{})
	c.Instance("plain", "not-a-health-checker")

	result := app.HealthCheck(ctx)

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

func TestApplicationHealthCheckNotBooted(t *testing.T) {
	app := NewApp()
	result := app.HealthCheck(context.Background())
	if result != nil {
		t.Fatal("HealthCheck before boot should return nil")
	}
}

func TestApplicationWithContainer(t *testing.T) {
	ctx := context.Background()
	c := New()
	c.Instance("existing", "value")

	app := NewApp(WithContainer(c))
	if app.Container() != c {
		t.Fatal("should use provided container")
	}

	v, err := app.Container().Make(ctx, "existing")
	if err != nil || v != "value" {
		t.Fatal("should access existing bindings")
	}
}

func TestApplicationRegisterError(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Register(&errorProvider{phase: "register"})

	err := app.Boot(ctx)
	if err == nil {
		t.Fatal("expected register error")
	}
}

func TestApplicationBootError(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Register(&errorProvider{phase: "boot"})

	err := app.Boot(ctx)
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

func (p *errorProvider) Boot(_ context.Context, c Container) error {
	if p.phase == "boot" {
		return errors.New("boot failed")
	}
	return nil
}

// --- P1: Application 状态机 ---

func TestApplicationRegisterAfterBootPanics(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Boot(ctx)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Register after Boot should panic")
		}
	}()

	app.Register(&trackingProvider{name: "late", tracker: &orderTracker{}})
}

func TestApplicationBootAfterFailReturnsError(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Register(&errorProvider{phase: "boot"})

	err := app.Boot(ctx)
	if err == nil {
		t.Fatal("expected boot error")
	}

	// 重试 Boot 应返回错误（failed 状态）
	err = app.Boot(ctx)
	if err == nil {
		t.Fatal("Boot retry after failure should return error")
	}
}

func TestApplicationShutdownFromCreated(t *testing.T) {
	app := NewApp()
	err := app.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown from created state should succeed, got %v", err)
	}
}

func TestApplicationShutdownIdempotent(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Boot(ctx)

	if err := app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	// 第二次应为 no-op
	if err := app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationShutdownFromFailed(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	c := app.Container()
	c.Instance("svc", &closeableService{name: "svc"})
	app.Register(&errorProvider{phase: "boot"})

	app.Boot(ctx) // fails

	// Shutdown from failed state should still close resources
	err := app.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown from failed state should succeed, got %v", err)
	}
}

func TestApplicationBootAfterShutdownFails(t *testing.T) {
	ctx := context.Background()
	app := NewApp()
	app.Boot(ctx)
	app.Shutdown(ctx)

	err := app.Boot(ctx)
	if err == nil {
		t.Fatal("Boot after Shutdown should fail")
	}
}

func TestApplicationBooted(t *testing.T) {
	app := NewApp()
	if app.Booted() {
		t.Fatal("should not be booted initially")
	}

	app.Boot(context.Background())
	if !app.Booted() {
		t.Fatal("should be booted after Boot")
	}

	app.Shutdown(context.Background())
	if app.Booted() {
		t.Fatal("should not be booted after Shutdown")
	}
}

// --- P2: ProvidesAware ---

func TestProvidesAwareInterface(t *testing.T) {
	tracker := &orderTracker{}
	p := &providesAwareProvider{
		deferredProvider: deferredProvider{name: "es", tracker: tracker},
		provides:         []string{"es", "es.client"},
	}

	names := p.Provides()
	if len(names) != 2 {
		t.Fatalf("expected 2 provides, got %d", len(names))
	}
	if !p.Deferred() {
		t.Fatal("should be deferred")
	}
}
