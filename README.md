# IoC - Go 依赖注入容器

[![Go Reference](https://pkg.go.dev/badge/github.com/go-pkg0/ioc.svg)](https://pkg.go.dev/github.com/go-pkg0/ioc)

go-pkg0 生态的基础依赖注入框架，灵感来自 Laravel Service Container。零外部依赖，为 log、db、cache、es、mongo 等模块提供统一的注册与解析规范。

## 特性

- **泛型优先** — `Singleton[T]` / `Bind[T]` / `Make[T]` / `Decorate[T]` 编译期约束类型，消除 `any` 手动断言
- **Container** — 服务绑定与解析，全链路 context.Context 传递
- **别名系统** — `Alias(alias, abstract)` 支持链式别名解析（最大深度 10），同一服务可多名访问
- **循环依赖检测** — 基于 context 的 per-goroutine 解析栈，Make 时自动检测并报告完整循环路径
- **AOP** — 全局中间件（Middleware）+ 服务装饰器（`Decorate[T]`），洋葱模型，支持链路追踪注入
- **ServiceProvider** — 两阶段生命周期（Register / Boot），支持延迟初始化 + `ProvidesAware` 内省
- **DriverManager[T]** — 泛型多驱动管理器，延迟创建 + 缓存 + 优雅关闭，适配 db、cache、log 等多驱动场景
- **服务契约** — Closeable / HealthChecker / `Configurable[T]` / ServiceInfo
- **Application** — 轻量编排器，完整状态机（created→booting→booted→shutdown/failed），自动 Boot / Shutdown / HealthCheck
- **生命周期管理** — Container 内置 Close（幂等）/ HealthCheck（并行），关闭后 Make 返回 `ErrContainerClosed`
- **并发安全** — per-key `sync.Once` 保证单例工厂恰好执行一次，永久缓存错误避免竞态
- **零依赖** — 纯标准库，任何框架均可使用

## 安装

```bash
go get github.com/go-pkg0/ioc@latest
```

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "fmt"
    "github.com/go-pkg0/ioc"
)

func main() {
    c := ioc.New()
    ctx := context.Background()

    // 注册单例 — 工厂返回类型由编译器推断为 string
    ioc.Singleton(c, "greeting", func(ctx context.Context, c ioc.Container) (string, error) {
        return "Hello, IoC!", nil
    })

    // 解析（类型安全，编译期检查）
    msg := ioc.MustMake[string](ctx, c, "greeting")
    fmt.Println(msg) // Hello, IoC!
}
```

### ServiceProvider 模式

```go
type CacheServiceProvider struct{}

func (p *CacheServiceProvider) Register(c ioc.Container) error {
    ioc.Singleton(c, "cache", func(ctx context.Context, c ioc.Container) (*RedisCache, error) {
        return NewRedisCache(ctx, "localhost:6379"), nil
    })
    return nil
}

func (p *CacheServiceProvider) Boot(ctx context.Context, c ioc.Container) error {
    // 所有 Provider 的 Register 完成后才执行 Boot
    // 这里可以安全地解析其他依赖
    return nil
}
```

### Application 编排

```go
app := ioc.NewApp()
app.Register(
    &LogServiceProvider{},
    &DatabaseServiceProvider{},
    &CacheServiceProvider{},
)

ctx := context.Background()

// Phase 1: 所有 Register() → Phase 2: 所有 Boot(ctx)
if err := app.Boot(ctx); err != nil {
    log.Fatal(err)
}
defer app.Shutdown(ctx) // 反序优雅关闭

// 使用服务（类型安全）
db := ioc.MustMake[*DatabaseManager](ctx, app.Container(), "db")
```

### AOP — 装饰器 & 中间件

```go
c := ioc.New()

// 全局中间件：记录所有服务解析耗时 + 链路追踪
c.Use(func(abstract string, next ioc.ResolveFunc) ioc.ResolveFunc {
    return func(ctx context.Context) (any, error) {
        start := time.Now()
        val, err := next(ctx)
        log.Printf("[ioc] %s resolved in %v", abstract, time.Since(start))
        return val, err
    }
})

// 类型安全装饰器：为特定服务添加监控（无需手动类型断言）
ioc.Decorate(c, "db", func(ctx context.Context, pool *ConnectionPool, c ioc.Container) (*ConnectionPool, error) {
    return NewMonitoredPool(pool), nil
})
```

### DriverManager — 多驱动管理

```go
ctx := context.Background()

// 创建多驱动管理器（默认使用 "redis"）
mgr := ioc.NewDriverManager[CacheDriver]("redis")

mgr.Register("redis", func(ctx context.Context) (CacheDriver, error) {
    return NewRedisDriver(ctx, redisCfg), nil
})
mgr.Register("memory", func(ctx context.Context) (CacheDriver, error) {
    return NewMemoryDriver(), nil
})

// 获取默认驱动
driver, err := mgr.Default(ctx)

// 按名称获取（延迟创建 + 缓存）
memDriver, err := mgr.Driver(ctx, "memory")

// 装饰已有驱动（Extend 包装原始工厂）
mgr.Extend("redis", func(original CacheDriver) (CacheDriver, error) {
    return NewMonitoredDriver(original), nil
})

// 优雅关闭所有已创建的驱动
defer mgr.Close(ctx)
```

## 泛型 API（推荐）

所有注册、解析、装饰操作均提供泛型函数，编译期约束类型，消除 `any`：

```go
// 注册 — 工厂返回类型由编译器推断
ioc.Singleton(c, "db", func(ctx context.Context, c ioc.Container) (*sql.DB, error) { ... })
ioc.Bind(c, "req", func(ctx context.Context, c ioc.Container) (*http.Request, error) { ... })
ioc.Instance(c, "config", &AppConfig{Port: 8080})

// 解析 — 返回具体类型，无需手动断言
db, err := ioc.Make[*sql.DB](ctx, c, "db")
db := ioc.MustMake[*sql.DB](ctx, c, "db")

// 装饰 — 入参和返回均为具体类型
ioc.Decorate(c, "db", func(ctx context.Context, db *sql.DB, c ioc.Container) (*sql.DB, error) {
    return NewMonitoredDB(db), nil
})
```

| 泛型函数 | 对应 Container 方法 | 说明 |
|----------|---------------------|------|
| `ioc.Singleton[T](c, name, factory)` | `c.Singleton(name, factory)` | 工厂返回 T，非 any |
| `ioc.Bind[T](c, name, factory)` | `c.Bind(name, factory)` | 工厂返回 T，非 any |
| `ioc.Instance[T](c, name, value)` | `c.Instance(name, value)` | value 类型为 T |
| `ioc.Decorate[T](c, name, fn)` | `c.Decorate(name, decorator)` | fn 接收和返回 T |
| `ioc.Make[T](ctx, c, name)` | `c.Make(ctx, name)` | 返回 T，非 any |
| `ioc.MustMake[T](ctx, c, name)` | `c.MustMake(ctx, name)` | 返回 T，非 any |

Container 接口方法为底层 API，仅在中间件、框架扩展等需要操作 `any` 的场景中直接使用。

## 核心接口

### Container

| 方法 | 说明 |
|------|------|
| `Bind(name, factory)` | 注册瞬时工厂，每次 Make 创建新实例 |
| `Singleton(name, factory)` | 注册单例工厂，首次 Make 后缓存 |
| `Instance(name, value)` | 注册已构建实例为单例 |
| `Alias(alias, abstract)` | 注册别名，支持链式解析（最大深度 10） |
| `Make(ctx, name)` | 按名称解析服务，自动检测循环依赖，ctx 传递给工厂和装饰器 |
| `MustMake(ctx, name)` | 解析服务，失败 panic |
| `Has(name)` | 判断是否已注册（自动解析别名） |
| `Decorate(name, decorator)` | 添加服务装饰器 |
| `Use(middleware...)` | 注册全局中间件 |
| `Close(ctx)` | 反序优雅关闭所有 Closeable 单例（幂等） |
| `HealthCheck(ctx)` | 并行对所有 HealthChecker 单例执行健康检查 |
| `Remove(name)` | 删除绑定及缓存 |
| `Bindings()` | 返回所有已注册名称 |
| `Flush()` | 清空所有绑定和缓存 |

### 服务契约

| 接口 | 说明 | 适用模块 |
|------|------|----------|
| `Closeable` | 优雅关闭（Close 反序调用） | db、log、cache、es、mongo |
| `HealthChecker` | 健康检查 | db、redis、es、mongo |
| `Configurable[T]` | 运行时配置热更新（T 为具体配置类型） | log、cache |
| `ServiceInfo` | 服务元信息 | 所有模块 |

## 别名

```go
c := ioc.New()
ioc.Instance(c, "database.mysql", mysqlConn)
c.Alias("db", "database.mysql")      // db → database.mysql
c.Alias("default-db", "db")          // default-db → db → database.mysql

conn, err := ioc.Make[*sql.DB](ctx, c, "default-db") // 自动链式解析
```

## 错误处理

### 哨兵错误

| 错误 | 说明 |
|------|------|
| `ErrNotBound` | 服务未注册 |
| `ErrNoFactory` | 绑定无工厂函数 |
| `ErrDriverNotFound` | DriverManager 中驱动未注册 |
| `ErrCircularDependency` | 检测到循环依赖（包含完整路径） |
| `ErrContainerClosed` | 容器已关闭，拒绝新的 Make |
| `ErrTypeMismatch` | `Make[T]` / `Decorate[T]` 类型断言失败 |

### 单例错误缓存

单例工厂返回的错误会被**永久缓存**，后续调用返回相同错误，避免并发竞态。如需重试：

```go
c.Remove("svc")                                                        // 清除绑定和缓存
ioc.Singleton(c, "svc", func(ctx context.Context, c ioc.Container) (T, error) { ... })  // 重新注册
val, err := ioc.Make[T](ctx, c, "svc")                                // 重新解析
```

## 生态模块接入

各模块按以下模板实现 `ServiceProvider` 即可接入：

| 模块 | 容器键 | 实现契约 |
|------|--------|----------|
| log | `log` | Closeable |
| db | `db` | Closeable, HealthChecker |
| cache | `cache` | Closeable, HealthChecker |
| es | `es` | Closeable, HealthChecker |
| mongo | `mongo` | Closeable, HealthChecker |

## 许可证

MIT License
