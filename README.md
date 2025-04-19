# IoC - Go 依赖注入容器

[![Go Reference](https://pkg.go.dev/badge/github.com/go-pkg0/ioc.svg)](https://pkg.go.dev/github.com/go-pkg0/ioc)

go-pkg0 生态的基础依赖注入框架，灵感来自 Laravel Service Container。零外部依赖，为 log、db、cache、es、mongo 等模块提供统一的注册与解析规范。

## 特性

- **Container** — 服务绑定与解析（Bind / Singleton / Instance / Make）
- **AOP** — 全局中间件（Middleware）+ 服务装饰器（Decorator），洋葱模型
- **ServiceProvider** — 两阶段生命周期（Register / Boot），支持延迟初始化
- **DriverManager[T]** — 泛型多驱动管理器，延迟创建 + 缓存，适配 db、cache、log 等多驱动场景
- **服务契约** — Closeable / HealthChecker / Configurable / ServiceInfo
- **Application** — 轻量编排器，自动 Boot / Shutdown / HealthCheck
- **并发安全** — per-key `sync.Once` 保证单例工厂恰好执行一次
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
    "fmt"
    "github.com/go-pkg0/ioc"
)

func main() {
    c := ioc.New()

    // 注册单例
    c.Singleton("greeting", func(c ioc.Container) (any, error) {
        return "Hello, IoC!", nil
    })

    // 解析（类型安全）
    msg := ioc.MustMakeTyped[string](c, "greeting")
    fmt.Println(msg) // Hello, IoC!
}
```

### ServiceProvider 模式

```go
type CacheServiceProvider struct{}

func (p *CacheServiceProvider) Register(c ioc.Container) error {
    c.Singleton("cache", func(c ioc.Container) (any, error) {
        return NewRedisCache("localhost:6379"), nil
    })
    return nil
}

func (p *CacheServiceProvider) Boot(c ioc.Container) error {
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

// Phase 1: 所有 Register() → Phase 2: 所有 Boot()
if err := app.Boot(); err != nil {
    log.Fatal(err)
}
defer app.Shutdown(context.Background()) // 反序优雅关闭

// 使用服务
db := ioc.MustMakeTyped[*DatabaseManager](app.Container(), "db")
```

### AOP — 装饰器 & 中间件

```go
c := ioc.New()

// 全局中间件：记录所有服务解析耗时
c.Use(func(abstract string, next ioc.ResolveFunc) ioc.ResolveFunc {
    return func() (any, error) {
        start := time.Now()
        val, err := next()
        log.Printf("[ioc] %s resolved in %v", abstract, time.Since(start))
        return val, err
    }
})

// 服务装饰器：为特定服务添加监控
c.Decorate("db", func(instance any, c ioc.Container) (any, error) {
    pool := instance.(*ConnectionPool)
    return NewMonitoredPool(pool), nil
})
```

### DriverManager — 多驱动管理

```go
// 创建多驱动管理器（默认使用 "redis"）
mgr := ioc.NewDriverManager[CacheDriver]("redis")

mgr.Register("redis", func() (CacheDriver, error) {
    return NewRedisDriver(redisCfg), nil
})
mgr.Register("memory", func() (CacheDriver, error) {
    return NewMemoryDriver(), nil
})

// 获取默认驱动
driver, err := mgr.Default()

// 按名称获取（延迟创建 + 缓存）
memDriver, err := mgr.Driver("memory")

// 用户扩展自定义驱动
mgr.Extend("memcached", func() (CacheDriver, error) {
    return NewMemcachedDriver(mcCfg), nil
})
```

## 核心接口

### Container

| 方法 | 说明 |
|------|------|
| `Bind(name, factory)` | 注册瞬时工厂，每次 Make 创建新实例 |
| `Singleton(name, factory)` | 注册单例工厂，首次 Make 后缓存 |
| `Instance(name, value)` | 注册已构建实例为单例 |
| `Make(name)` | 按名称解析服务 |
| `MustMake(name)` | 解析服务，失败 panic |
| `Has(name)` | 判断是否已注册 |
| `Decorate(name, decorator)` | 添加服务装饰器 |
| `Use(middleware...)` | 注册全局中间件 |
| `Remove(name)` | 删除绑定及缓存 |
| `Bindings()` | 返回所有已注册名称 |
| `Flush()` | 清空所有绑定和缓存 |

### 服务契约

| 接口 | 说明 | 适用模块 |
|------|------|----------|
| `Closeable` | 优雅关闭（Shutdown 反序调用） | db、log、cache、es、mongo |
| `HealthChecker` | 健康检查 | db、redis、es、mongo |
| `Configurable` | 运行时配置热更新 | log、cache |
| `ServiceInfo` | 服务元信息 | 所有模块 |

### 泛型助手

```go
// 类型安全解析
val, err := ioc.MakeTyped[*MyService](container, "my-service")

// 类型安全解析（失败 panic）
val := ioc.MustMakeTyped[*MyService](container, "my-service")
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
