# SpringHere Gin Server

面向管理后台的认证与用户管理服务。基于 Gin、PostgreSQL、Redis，可独立部署，也可作为前后端分离后台的 API。

登录使用 **Access Token + HttpOnly Refresh Cookie**。登录态以 Redis 为准：刷新会轮换 Token，踢人、改密、停用或删除用户后，旧 Token 立即失效。登录前需要一次性图形验证码。

HTTP 接口说明（含登录页对接示例）见 [docs/接口文档.md](docs/接口文档.md)。

## 能做什么

- 登录 / 刷新 / 退出 / 当前用户
- 图形验证码（一次性、短 TTL，登录必须校验）
- 管理员对用户的增删改查、启用停用、踢下线
- 角色：`admin`、`user`；用户接口只允许 admin
- 健康检查：`/healthz` 存活，`/readyz` 检查 PostgreSQL 和 Redis
- 优雅停机（SIGINT / SIGTERM）

## 架构

```text
cmd/server          进程入口
pkg/app             组装配置、数据库、Redis、路由
internal/router     路由
internal/service    HTTP 处理（认证、用户）
internal/data       PostgreSQL / Redis
internal/entity     请求与响应 DTO
internal/model      数据表结构；query 由 gormgen 生成
pkg/middleware      鉴权、CORS、限流 body、日志、恢复
pkg/config          YAML + 环境变量
```

数据分工：

| 存储 | 用途 |
|------|------|
| PostgreSQL | 用户账号、角色、密码哈希 |
| Redis | 登录会话、Refresh 索引、验证码、验证码 IP 限流 |

不要把原始 Access Token 或 Refresh Token 写入数据库。Redis 里只存会话快照和 Refresh 的 SHA-256 digest。

### 认证流程

```text
浏览器                         服务端                         Redis
  |  GET /auth/captcha           |                              |
  |----------------------------->|  生成图片，写入 captcha:v1:*   |
  |  captcha_id + image          |                              |
  |  POST /auth/login            |                              |
  |  email, password,            |  核销验证码 → 校验密码         |
  |  captcha_id, captcha         |  写入 session + refresh 索引  |
  |<-----------------------------|  JSON: access_token          |
  |  Set-Cookie: refresh         |                              |
  |  API + Authorization Bearer  |  验 JWT，再对 session/jti     |
  |  POST /auth/refresh + Cookie |  轮换 refresh 与 access jti   |
```

- **Access Token**：JWT，放在内存里，请求头 `Authorization: Bearer <token>`，默认 15 分钟。
- **Refresh Token**：高熵随机值，只在 Cookie `springhere_refresh` 中，Path 为 `/api/v1/auth`，HttpOnly，SameSite=Lax。JavaScript 读不到。
- 受保护请求会核对 Redis 中的 `auth:v2:session:{sid}`，JWT 的 `jti` 必须等于当前会话的 Access 标识。Session 被删或已轮换时，JWT 即使未过期也是 401。
- 刷新不会延长首次登录时的绝对会话期限。

前端应对 `/auth/refresh` 做单飞，且不要拦截 `/auth/captcha`、`/auth/login`、`/auth/refresh` 再去刷新 Token。登录返回 `401 INVALID_CREDENTIALS` 是账号或密码错误，应换验证码，而不是当会话过期。

## 需要什么

- Go 1.26.3 或更高
- PostgreSQL
- Redis（请开持久化，淘汰策略用 `noeviction`）

登录态只在 Redis。Redis 若用 `allkeys-lru` 之类策略，会话和验证码可能被清掉，表现为用户突然掉线。

## 快速开始

```bash
docker compose -f deploy/springhere/docker-compose.yaml up -d postgres redis
go run ./cmd/migrate -action up
go run ./cmd/server -config application.yaml
```

默认监听 `http://localhost:8080`。本地允许的前端 Origin 为 `http://localhost:5173`。建议用 Vite 把 `/api` 代理到后端，前端只走相对路径。

```text
GET /healthz   进程是否还在
GET /readyz    PostgreSQL 和 Redis 是否可达
```

### 引导管理员

每次进程启动都会确保存在 `admin@localhost.com`：没有就创建，**已有则重置密码**，并清掉该账号全部登录。明文密码用 UUID 生成，打到日志里：

```text
bootstrap admin ready    {"email": "admin@localhost.com", "password": "<uuid>"}
```

这是方便本地第一次跑起来的行为，**不适合生产**。生产请关掉这套引导逻辑，改用迁移或一次性脚本创建管理员，密码不要打日志。

## 配置

默认文件是仓库根目录的 `application.yaml`。环境变量会覆盖文件：

| 变量 | 作用 |
|------|------|
| `APP_ENV` | 运行环境。生产请设为 `production` |
| `SERVER_ADDR` | 监听地址，默认 `:8080` |
| `DATABASE_DSN` | PostgreSQL 连接串 |
| `REDIS_ADDR` | Redis 地址 |
| `REDIS_USERNAME` / `REDIS_PASSWORD` | Redis 账号 |
| `AUTH_JWT_SECRET` | JWT 密钥，至少 32 字节 |

参考 `.env.example`。不要把真实密钥提交进 Git。

验证码默认：

```yaml
captcha:
  enabled: true
  length: 4          # 4～6
  ttl: 2m
  width: 160
  height: 48
  issue_limit_per_minute: 20
```

`APP_ENV=production` 时：

- 必须 `refresh_cookie_secure: true`（HTTPS）
- 必须 `captcha.enabled: true`
- 否则进程拒绝启动

## 接口一览

业务前缀：`/api/v1`。

| 方法 | 路径 | 认证 |
|------|------|------|
| GET | `/healthz` | 无 |
| GET | `/readyz` | 无 |
| GET | `/api/v1/auth/captcha` | 无 |
| POST | `/api/v1/auth/login` | 无（需验证码） |
| POST | `/api/v1/auth/refresh` | Refresh Cookie + Origin |
| POST | `/api/v1/auth/logout` | Refresh Cookie + Origin |
| GET | `/api/v1/auth/me` | Bearer |
| GET/POST | `/api/v1/users` | Admin Bearer |
| GET/PATCH/DELETE | `/api/v1/users/:id` | Admin Bearer |
| PATCH | `/api/v1/users/:id/status` | Admin Bearer |
| POST | `/api/v1/users/:id/kick` | Admin Bearer |

登录请求体：

```json
{
  "email": "admin@localhost.com",
  "password": "<password>",
  "captcha_id": "<uuid>",
  "captcha": "ab12"
}
```

先 `GET /api/v1/auth/captcha` 拿到 `captcha_id` 和图片 Data URL。验证码 4 位、大小写不敏感、一次性；失败后必须换新图。字段、错误码和前端示例见 [docs/接口文档.md](docs/接口文档.md)。

## 目录

```text
cmd/server              HTTP 服务
cmd/migrate             SQL 迁移（文件嵌入二进制）
cmd/gormgen             根据 model 生成 query
deploy/springhere/      Compose、Nginx
docs/接口文档.md         给前端的 HTTP 契约
internal/               业务代码
pkg/                    可复用的配置、中间件、组件
application.yaml        本地默认配置
```

## 开发

```bash
make run                 # 等价 go run ./cmd/server -config application.yaml
make test
make fmt
make migrate-up
make migrate-down
make gormgen
make gormgen-check       # 适合放 CI：生成后 query 目录不能有未提交差异
```

改 `internal/model` 之后要重新生成 `internal/model/query` 并提交。迁移文件在 `cmd/migrate/migrations/`，在仓库根目录执行 `go run ./cmd/migrate -action up|down|version`。DSN 读 `application.yaml`，仍可用 `DATABASE_DSN` 覆盖。

## 构建镜像

在仓库根目录：

```bash
docker build -f deploy/Dockerfile -t springhere-gin-server .
```

入口为 `/server -config /application.yaml`。镜像里那份 YAML 是开发默认值。容器里请用环境变量覆盖 `DATABASE_DSN`、`REDIS_ADDR`、`AUTH_JWT_SECRET`，并把地址改成编排网络里的服务名，不要用 `localhost`。

## 生产部署注意

1. `APP_ENV=production`，换掉默认 `jwt_secret`，打开 `refresh_cookie_secure`。
2. 用环境变量注入 DSN、Redis、JWT，不要把生产密钥写进镜像或仓库。
3. 前后端同站点或反代到同一站点，Cookie 才是 `SameSite=Lax`。跨子域需要自己改 Cookie 策略。
4. CORS 按完整 Origin 精确匹配（协议 + 域名 + 端口）。把真实前端 Origin 配进 `auth.allowed_origins`。
5. Redis 开 AOF/RDB，`maxmemory-policy noeviction`。
6. 若前面有反代，必须给 Gin 配置可信代理，否则验证码按 IP 限流会被 `X-Forwarded-For` 伪造。当前代码未设置 Trusted Proxies。
7. 不要依赖启动时的引导管理员。每次重启都会改 `admin@localhost.com` 的密码并踢掉登录。
8. 图形验证码能挡脚本化乱撞，挡不住 OCR。需要更强防护时，再加登录失败限流或第三方人机验证。

以下操作会立刻让目标用户全部 Token 失效：退出、管理员踢人、改资料/角色/密码、启停、删除。

## 当前限制

这些是有意取舍或尚未补上的点，使用和二次开发时请知晓：

- **引导管理员每次启动都重置**，生产直接用会丢管理员密码。
- **会话只在 Redis**，Redis 空了等于全员下线。
- **验证码是 4 位字母数字图**，登录接口本身没有额外按账号/IP 限流。
- **Refresh Cookie 未设 Domain**，依赖同站点；跨站 SPA 需要改 Cookie / 反向代理。
- **登录不校验 Origin**（刷新和退出会校验），依赖 CORS + SameSite。
- **用户是物理删除**，没有软删或审计表。
- **自动化测试覆盖偏薄**，目前主要覆盖配置校验和验证码存储。
- **尚未指定开源许可证**。对外发布前请补 `LICENSE`。
- 仓库 `.gitignore` 较简，本地 IDE 目录和 `.DS_Store` 需要自行避免提交。

## 相关文档

- [接口文档](docs/接口文档.md)：统一响应、认证模型、登录对接、错误码、CORS 与 Cookie
