# AGENTS.md

给在本仓库改代码的人和大模型用的开发约束。先读本文和 [docs/接口文档.md](docs/接口文档.md)，再改代码。实现细节以仓库代码为准。

模块路径：`github.com/buding00/springhere-gin-server`。Go 1.26.3。HTTP 框架 Gin。持久化 PostgreSQL，登录态 / 验证码 Redis。

## 这是什么

可独立部署的管理后台认证与用户服务：登录、验证码、刷新、退出、当前用户；管理员对用户的增删改查、启停、踢下线。

不要把它改成带全局单例、手写 SQL、或 `controller / usecase / repository` 多层套娃的另一种结构。按现有分层加功能。

## 目录

```text
cmd/server              HTTP 进程入口，只解析 -config 并启动
cmd/migrate             SQL 迁移（文件嵌入二进制）
cmd/gormgen             根据 internal/model 生成 internal/model/query
internal/server.go      Gin 引擎、全局中间件、/healthz /readyz、挂 /api/v1
internal/router         只注册路由和中间件，不写业务
internal/service        HTTP 处理：绑参、调 data、写响应
internal/entity         请求 / 响应 DTO，以及会话快照等跨层结构
internal/model          表结构；query/ 由 gormgen 生成，不要手改
internal/data           PostgreSQL / Redis 访问
pkg/app                 组装依赖并注入，没有全局变量
pkg/component           创建 logger、Postgres、Redis 连接
pkg/config              YAML + 环境变量 + Validate
pkg/middleware          鉴权、CORS、日志、恢复、body 限制、Request-ID
pkg/response            统一 JSON
pkg/constant            角色等稳定枚举
pkg/utils               密码、JWT、验证码画图等无状态工具
docs/接口文档.md         给前端的 HTTP 契约，改接口必须同步
application.yaml        本地默认配置
```

`internal/` 只放：`data/`、`entity/`、`model/`、`router/`、`service/`、`server.go`。不要新增 `controller`、`repo`、`usecase`、`handler`、`global` 之类目录。可复用、与具体业务无关的放 `pkg/`。

## 请求怎么走

```text
cmd/server
  → pkg/app.New（读配置、建连接、NewXxx、注入）
    → internal.NewServer（中间件、健康检查）
      → router.Register
        → middleware（可选鉴权）
        → service.Method（gin.Context）
          → data（query.Query 或 Redis store）
          → response.OK* / FailWithCode
```

- `router`：组路径、鉴权角色、把方法挂到 service。不查库、不碰 Redis、不拼 JSON。
- `service`：`ShouldBind*`、校验、调一到多个 data/store、映射 `entity`、写 HTTP 状态和 `pkg/response`。不写原始 SQL，不直接 `query.Use`。
- `data`：只做存储。Postgres 用 gormgen 的 `*query.Query`；Redis 用注入的 `redis.Cmdable`。返回 `data.ErrNotFound`、`data.ErrConflict` 或包装后的 error。不写 `gin.Context`。
- `entity`：JSON / form 的进出站结构。`model.User` 不准直接当 HTTP 响应（会带出 `password_hash`）。
- `model`：表映射。改字段后必须跑 gormgen，并把 `query/` 提交进 Git。
- service 里为写入组装 `model.User` 可以；**禁止**把 `model` 直接 `c.JSON` 出去。转 `entity.User` / `entity.AuthUser`。
- 请求里用 `c.Request.Context()`，不要在 handler 里 `context.Background()`（启动引导除外）。
- 失败用 `c.AbortWithStatusJSON`，成功用 `c.JSON`。JSON 绑定时若是 `*http.MaxBytesError`，返回 `413 PAYLOAD_TOO_LARGE`（各 POST/PATCH 已有同样分支，照抄）。

依赖方向（不要反向、不要循环）：

```text
cmd → pkg/app → internal/router → internal/service → internal/data
                              ↘ pkg/middleware
service → entity、data、model、pkg/*
data    → model、model/query、entity（会话结构）、pkg/constant、pkg/utils
router  → service、middleware、constant          不 import data / model
model   → constant                               不 import gin / data / service
pkg/component、pkg/config、pkg/response、pkg/utils  不 import internal
```

`pkg/middleware` 已依赖 `internal/data`、`internal/entity`（鉴权读会话）。不要再增加 `pkg → internal` 的包。新业务放 `internal/`，不要为了「公共」把仓储塞进 `pkg`。

不要引入 Wire、fx、dig；组装只写在 `pkg/app/app.go`。

## 依赖注入，不用全局

禁止：

```go
var DB *gorm.DB
var Rdb *redis.Client
var Log *zap.Logger
```

以及 `init()` 里连库、`package` 级 client、隐式单例。

正确做法：构造函数收依赖，在 `pkg/app/app.go` 里创建并传入。

```go
db, databaseCleanup := component.OpenPostgres(cfg.Database)
redisClient, redisCleanup := component.NewRedis(cfg.Redis)
userRepo := data.NewUserRepository(db)
authSessionStore := data.NewRedisAuthSessionStore(redisClient)
```

- Postgres：`data.NewUserRepository(db)`，仓储内 `query.Use(db)`。
- Redis：store 持有 `redis.Cmdable`（测试可换 miniredis），不要在业务包里 `redis.NewClient`。
- Logger、JWT、验证码画图器同样注入。
- 需要新依赖时：扩展 `NewXxx(...)` 参数，并只改 `pkg/app/app.go` 的组装处。

`component` 只负责 Open/Ping/Close。业务读写不放在 `pkg/component`。

## PostgreSQL：gormgen，不要手写 SQL

表结构以 `cmd/migrate/migrations/` 为准，**不要** `AutoMigrate`。查询以 `internal/model/query` 为准，**不要** `db.Raw` / `db.Exec` / 字符串拼 SQL。

新增或修改表：

1. 写 `cmd/migrate/migrations/NNNNNN_name.up.sql` 和配套 `.down.sql`（六位序号，紧接现有最大号）。在仓库根目录执行迁移。
2. 新迁移只改自己的表，**不要**再 `DROP TABLE users`。`000001` 里的 DROP 只给空库初始化用。
3. 改或新增 `internal/model` 结构体（`TableName()`、列 tag）。gormgen **不连数据库**，只看结构体。
4. 若是新模型：在 `cmd/gormgen/main.go` 的 `ApplyBasic(...)` 里登记。
5. `make gormgen`（或 `go run ./cmd/gormgen`）。**不要手改** `internal/model/query/*.gen.go`。
6. 在 `internal/data` 用生成代码，例如：

```go
r.query.User.WithContext(ctx).Where(r.query.User.Email.Eq(email)).Take()
```

7. 把 `internal/model/query` 的生成结果提交进仓库。`make gormgen-check` 会再生成一次，有未提交差异则失败。

LIKE 等用户输入要转义，参考 `escapeLikePattern`。唯一冲突映射为 `ErrConflict`（依赖 GORM `TranslateError: true`）。

## Redis

登录会话、验证码、IP 限流只放 Redis，不落 PostgreSQL。

现有 key：

```text
auth:v2:session:{sessionID}
auth:v2:refresh:{digest}          # 只存 SHA-256，不存原始 Token
auth:v2:user-sessions:{userID}    # ZSET，score = 过期毫秒
captcha:v1:{captchaID}
captcha:v1:ip:{ip}
```

新 key 用清晰前缀，不要复用 `auth:v2:` / `captcha:v1:`。需要原子操作时用 Lua（见 `auth_session_scripts.go`、验证码核销），不要先 GET 再 SET 制造竞态。

会话 TTL 等于 Refresh 有效期。踢人、改密、改角色、启停、删除用户必须删掉该用户全部会话（调 `AuthService.InvalidateUserSessions`）。改资料时先失效会话再写库（与现有 Update/Delete/SetActive 一致）。

管理员不能删除、停用或改**自己的角色**（`rejectSelfTarget`）。踢自己、改自己密码/邮箱可以。新的危险操作先看是否要同样拦自己。

用户是**物理删除**，不要擅自改成软删，除非产品明确要求并走迁移。

## HTTP 与鉴权

- 业务前缀：`/api/v1`。健康检查在根上：`GET /healthz`、`GET /readyz`。
- 统一响应：`pkg/response`。成功 `code=0`；业务失败 HTTP 状态 + `code=1000` + `reason`。前端靠 HTTP 状态和 `reason` 分支，不要发明另一套外层 code。
- 已有 `reason` 能用就用：`INVALID_REQUEST`、`INVALID_CAPTCHA`、`INVALID_CREDENTIALS`、`UNAUTHORIZED`、`FORBIDDEN`、`NOT_FOUND`、`CONFLICT`、`PAYLOAD_TOO_LARGE`、`TOO_MANY_REQUESTS`、`SERVICE_UNAVAILABLE`、`INTERNAL_ERROR`。新错误先看 [docs/接口文档.md](docs/接口文档.md) 第 7 节，没有再用新大写蛇形名字。
- 成功用 `OKWithData` / `OKWithPageData` / `OKWithMessage`；失败用 `FailWithCode`，**不要把内部 error 字符串返回给客户端**。
- 鉴权：`middleware.AuthMiddleware.Required(roles...)`，必须显式列出允许的角色。路由里写 `constant.RoleAdmin` / `constant.RoleUser`，不要写 `"admin"` 字符串。角色只有这两种，用 `Role.Valid()`。
- Access Token：`Authorization: Bearer`；Refresh：HttpOnly Cookie，Path `/api/v1/auth`，SameSite=Lax，JavaScript 读不到。Refresh / Logout 要加 `RequireAllowedOrigin`。不要把 Refresh 放进 JSON。
- 登录 / `/me` 返回 `entity.AuthUser`（无 `active`、`online`）。用户管理返回 `entity.User`。
- 登录先核销验证码再比密码。密码 bcrypt，长度 8～72 字节（bcrypt 上限）。用户不存在时走 `DummyCompare`。邮箱一律 `ToLower(TrimSpace)`。新资源主键用 `uuid.NewString()`。
- `User.online`：是否还有未过期 Redis 会话，不是心跳。列表/详情查 Redis；写接口清会话后可固定 `false`。Redis 故障返回 503，不要假装全员离线。
- 当前用 **PATCH** 做部分更新，不要无故改成 PUT。若新增 HTTP 方法，必须同步改 `pkg/middleware/cors.go` 的 `Allow-Methods`（现为 GET, POST, PATCH, DELETE, OPTIONS）。
- CORS Origin **精确匹配**（协议+域名+端口）。新前端地址加到 `auth.allowed_origins`，并让 `Validate` 继续校验绝对 Origin。
- 列表 `page` 最小 1，`page_size` 默认 20、最大 100。JSON 分页里是 `pageSize`。

HTTP 状态按现有接口对齐：

| 情况 | 状态 | reason |
|------|------|--------|
| 参数 / 绑定失败 | 400 | `INVALID_REQUEST` |
| 验证码错误或已用 | 400 | `INVALID_CAPTCHA` |
| 账号或密码错 | 401 | `INVALID_CREDENTIALS` |
| 未登录 / Token 无效 | 401 | `UNAUTHORIZED` |
| 角色不够 / Origin 不允许 | 403 | `FORBIDDEN` |
| 资源不存在 | 404 | `NOT_FOUND` |
| 邮箱冲突 | 409 | `CONFLICT` |
| 请求体过大 | 413 | `PAYLOAD_TOO_LARGE` |
| 验证码拉取过频 | 429 | `TOO_MANY_REQUESTS` |
| Redis 或会话存储失败 | 503 | `SERVICE_UNAVAILABLE` |
| 其它内部错误 | 500 | `INTERNAL_ERROR` |
| 创建成功 | 201 | （成功体，无 reason） |

## 配置

- 默认文件 `application.yaml`。新增项必须四处一起改：结构体字段、`defaults()`、`Validate()`、`application.yaml`。只有需要部署覆盖的才加环境变量，并写进 `override()`（现在只有 `APP_ENV`、`SERVER_ADDR`、`DATABASE_DSN`、`REDIS_*`、`AUTH_JWT_SECRET`）。
- 生产（`app.env == production`）已强制：Secure Refresh Cookie、验证码开启、JWT secret 至少 32 字节。同类安全开关继续放进 `Validate`。`APP_ENV=production` 时 `gin.SetMode(ReleaseMode)`。
- 不要把真实密钥提交进 Git。不要在日志里打 Access Token、Refresh、验证码答案；引导管理员明文密码仅开发启动时打一次。引导逻辑每次启动都会重置 `admin@localhost.com`，新功能不要再强化这条路径，也不要在生产依赖它。

## 文档（改接口必做）

对外 HTTP 一变，必须改 [docs/接口文档.md](docs/接口文档.md)，使前端能按文档对接。至少包括：

1. 第 1 节接口一览表
2. 第 4 节 TypeScript 类型
3. 对应章节的方法、路径、请求/响应 JSON、字段表
4. 错误码表（若有新 `reason`）
5. 鉴权、Cookie、分页等行为变化

登录页、列表字段这类前端易踩的点，补示例和错误处理说明（参考验证码、`online` 的写法）。

不要只改代码不改文档，也不要只在 README 里提一句。

## 注释

新增功能必须带注释，风格与现有文件一致，不要提交无注释的导出符号。

- 用简短**中文**，说明职责或非显而易见的约束，不要写修改过程、不要写「开始处理请求」这类流水账。
- 每个新的导出类型、构造函数、导出方法都要有一行注释（Go 文档注释，紧挨声明）。
- `entity` 的请求/响应结构、`model` 的表结构、`data` 的仓储/store、`service` 的 handler、`router` 的 `Register` 都要注释。
- 错误变量、Redis key 前缀、Lua 脚本、鉴权/一次性核销/TTL 等安全相关逻辑，注释写清为什么，而不是复述代码。
- 未导出的辅助函数若有非显而易见约束，也写一行（如 `escapeLikePattern`、`rejectSelfTarget`）。
- 不给显而易见的赋值加注释；不用 `TODO`、`FIXME` 代替实现；不要用注释关掉本该修的问题。
- 生成代码（`internal/model/query`）不要手写注释。
- 新文件、新符号用中文注释（与 `internal/` 一致）。不要把整文件改成英文注释。

示例（与现有代码相同）：

```go
// UserRepository 负责 users 表的读写。
type UserRepository struct{ query *query.Query }

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository { ... }

// List 分页查询用户。
func (s *UserService) List(c *gin.Context) { ... }
```

## 代码风格

- 错误在 service 打日志（`zap.Error`），响应用固定英文 message + `reason`。
- JSON 字段 `snake_case`（`access_token`、`captcha_id`、`created_at`）；分页里的 `pageSize` 保持现有拼写。
- 时间用 RFC 3339（`time.Time` 默认 JSON）。
- 不要引入与现有栈重复的 Web/ORM/日志库，也不要擅自加 Swagger/gRPC/OpenAPI；对外契约就是 `docs/接口文档.md`。
- 不要用 `panic` 处理请求内错误；启动期依赖失败（连不上库、配置非法）可以 panic。
- 改动范围只覆盖需求本身，不要顺手重构无关文件。
- 格式化：`gofmt` / `make fmt`。提交前 `make test`。涉及生成代码时再跑 `make gormgen-check`。

## 测试

- 能单测的 data 层用 miniredis（Redis）或纯函数测试，不要为了测仓储去连真实 Postgres（除非已有测试夹具）。
- 改会话、验证码、在线状态时补 `internal/data` 测试。
- 改配置校验补 `pkg/config` 测试。

## 新增一项业务功能的步骤

以「管理员可管理的新资源」为例：

1. 迁移 SQL → `internal/model` → `cmd/gormgen` 登记 → `make gormgen`。
2. `internal/entity`：请求/响应 DTO。
3. `internal/data`：仓储，只用 `query`。
4. `internal/service`：handler；需要 Redis 则注入已有 store 或新 store（构造函数注入）。
5. `internal/router`：挂到 `/api/v1/...`，声明角色。
6. `pkg/app/app.go`：New 出来并注入，不要全局。
7. 给新增的类型、构造函数、方法补中文注释（见「注释」）。
8. 更新 `docs/接口文档.md`。
9. 需要的配置：结构体、`defaults()`、`Validate()`、`application.yaml`，部署覆盖再加 `override()`。
10. 新 HTTP 方法同步 CORS。
11. `make fmt`、`make test`；动过 model 再 `make gormgen-check`。

只加只读查询：同样走 gormgen，不要在 service 里 `db.Raw`。

只加 Redis 状态：新 store 放 `internal/data`，在 `app.New` 注入。

## 不要做的事

- 拆掉 router / service / data / entity / model 的边界
- 用全局 DB/Redis/Logger
- 手写 SQL 绕过 gormgen
- 把 `internal/model/query` 当手写包改完不重新生成
- 用 GORM AutoMigrate 代替 `cmd/migrate`
- 把密码哈希、Refresh 明文、验证码答案写进 JSON 或日志
- 改了对外接口却不更新 `docs/接口文档.md`
- 新增导出类型/方法却不写中文注释，或用英文流水账注释
- 在 `internal/` 下再堆一套分层目录
- 给 refresh/logout 去掉 Origin 检查，或把 Refresh 放进 JSON
- 把会话改存 PostgreSQL（登录态以 Redis 为准）
- 新增 HTTP 方法却不改 CORS
- 新增配置项只改 yaml、不改 `defaults`/`Validate`/`override`
- 在 handler 里 `context.Background()` 或把 `model` 直接当 JSON 响应

## 常用命令

```bash
make fmt
make test
make gormgen
make gormgen-check
make migrate-up          # 需要本地 PostgreSQL
make run                 # 需要 PostgreSQL + Redis
```

本地依赖：

```bash
docker compose -f deploy/springhere/docker-compose.yaml up -d postgres redis
```

引导管理员每次启动会重置 `admin@localhost.com` 的密码并打到日志，生产不要依赖这个行为。
