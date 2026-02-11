# 开发文档

## 架构概述

系统采用前后端分离架构，Go 后端提供 REST API 并托管前端静态文件。

```
用户浏览器 → Gin HTTP 服务器 → API 路由 → 业务处理器
                ↓                              ↓
           静态文件服务              调度器 / 数据库 / DNS API
           (web/dist/)
```

核心模块依赖关系：

```
api 层 → scheduler → prober（执行探测）
                   → provider（DNS 操作）
                   → cache（已删除记录）
       → crypto（凭证加密）
       → database（数据持久化）
```

## 模块说明

### 1. 数据模型 (`internal/model`)

定义 6 个 GORM 模型：

| 模型 | 说明 |
|------|------|
| `User` | 用户账户，bcrypt 哈希密码 |
| `Credential` | 云服务商凭证，AES-GCM 加密存储 |
| `ProbeTask` | 探测任务配置 |
| `DeletedRecord` | 已删除 DNS 记录缓存 |
| `ProbeResult` | 探测结果记录 |
| `OperationLog` | DNS 操作日志 |

### 2. 健康探测 (`internal/prober`)

`Prober` 接口统一所有协议：

```go
type Prober interface {
    Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult
}
```

5 种实现：
- `ICMPProber` — go-ping 库发送 Echo 请求
- `TCPProber` — `net.DialTimeout` 建立连接
- `UDPProber` — UDP 数据包发送
- `HTTPProber` — HTTP GET，2xx/3xx 为健康
- `HTTPSProber` — 同 HTTP，使用 HTTPS

工厂函数 `NewProber(protocol)` 根据协议类型创建对应探测器。

### 3. 云服务商对接 (`internal/provider`)

`DNSProvider` 统一接口：

```go
type DNSProvider interface {
    SupportsPause() bool
    ListRecords(ctx, domain, subDomain, recordType) ([]DNSRecord, error)
    AddRecord(ctx, domain, subDomain, recordType, value, ttl) (string, error)
    UpdateRecord(ctx, recordID, subDomain, recordType, value, ttl) error
    PauseRecord(ctx, recordID) error
    ResumeRecord(ctx, recordID) error
    DeleteRecord(ctx, recordID) error
}
```

当前实现：
- `aliyun.AliyunDNSClient` — 阿里云 DNS API，HMAC-SHA1 签名认证

扩展新服务商只需实现 `DNSProvider` 接口。

### 4. 监控调度器 (`internal/scheduler`)

每个探测任务独立 goroutine，按配置周期执行：

1. 从 DNS 服务商获取当前域名记录
2. 合并已删除记录缓存中的 IP
3. 对每个 IP 执行健康探测
4. 更新连续成功/失败计数
5. 达到失败阈值 → 暂停或删除记录
6. 达到恢复阈值 → 恢复或重新添加记录
7. 最后一条记录保护：不会暂停/删除域名下唯一的活跃记录

关键导出函数（用于单元测试和外部调用）：
- `MergeIPList` — 合并在线记录和缓存记录的 IP 列表
- `EvaluateFailureAction` — 根据阈值和 SupportsPause 判定失败操作
- `EvaluateRecoverAction` — 判定恢复操作
- `IsLastActiveRecord` — 最后一条记录保护判定

### 5. 已删除记录缓存 (`internal/cache`)

基于 GORM 操作 `DeletedRecord` 表：
- `Add` — 记录被删除时存入缓存
- `Remove` — 记录恢复后从缓存移除
- `ListByTask` — 获取某任务下所有已删除记录
- `CleanByTask` — 任务删除时清理关联缓存

### 6. 凭证加密 (`internal/crypto`)

- `Encrypt(plaintext, key)` — AES-256-GCM 加密，返回 base64 编码
- `Decrypt(ciphertext, key)` — 解密
- `MaskSecret(secret)` — 脱敏显示（前4位 + **** + 后4位）

密钥 32 字节，首次启动自动生成并保存到 `data/encryption.key`。

### 7. Web API 层 (`internal/api`)

基于 Gin 框架，JWT 认证中间件保护受限接口。

## API 接口列表

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/login` | 登录，返回 JWT token |

### 受保护接口（需 `Authorization: Bearer <token>`）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/logout` | 登出 |
| GET | `/api/credentials` | 获取凭证列表（脱敏） |
| POST | `/api/credentials` | 添加凭证 |
| DELETE | `/api/credentials/:id` | 删除凭证 |
| GET | `/api/tasks` | 获取任务列表 |
| POST | `/api/tasks` | 创建探测任务 |
| GET | `/api/tasks/:id` | 获取任务详情 |
| PUT | `/api/tasks/:id` | 更新探测任务 |
| DELETE | `/api/tasks/:id` | 删除探测任务 |
| GET | `/api/tasks/:id/history` | 探测历史（支持 `?ip=&page=&page_size=`） |
| GET | `/api/tasks/:id/logs` | 操作日志（支持 `?page=&page_size=`） |

### 请求/响应示例

登录：
```json
// POST /api/login
// 请求
{ "username": "admin", "password": "admin123" }
// 响应
{ "token": "eyJhbGciOiJIUzI1NiIs..." }
```

创建探测任务：
```json
// POST /api/tasks
{
  "credential_id": 1,
  "domain": "example.com",
  "sub_domain": "www",
  "probe_protocol": "HTTP",
  "probe_port": 80,
  "probe_interval_sec": 60,
  "timeout_ms": 3000,
  "fail_threshold": 3,
  "recover_threshold": 3
}
```

添加凭证：
```json
// POST /api/credentials
{
  "provider_type": "aliyun",
  "name": "我的阿里云",
  "access_key_id": "LTAI5t...",
  "access_key_secret": "SRnz43..."
}
```

## 前端开发

前端位于 `web/` 目录，Vue 3 + Vite + Element Plus。

```bash
cd web
npm install
npm run dev    # 启动开发服务器（自动代理 /api 到 localhost:8080）
npm run build  # 构建到 web/dist/
```

页面组件：

| 文件 | 说明 |
|------|------|
| `Login.vue` | 登录页面 |
| `Dashboard.vue` | 系统总览（任务统计） |
| `TaskList.vue` | 探测任务列表 |
| `TaskForm.vue` | 创建/编辑任务表单 |
| `TaskDetail.vue` | 任务详情（探测历史 + 操作日志） |
| `Credentials.vue` | 凭证管理 |

`App.vue` 包含侧边栏导航布局，登录页单独全屏显示。

## 数据库

使用 SQLite（纯 Go 驱动 `github.com/glebarez/sqlite`，无需 CGO/GCC）。

数据库文件默认路径：`data/dns-monitor.db`

GORM AutoMigrate 自动建表，首次启动自动创建默认管理员账户。

## 扩展新的 DNS 服务商

1. 在 `internal/provider/` 下创建新目录（如 `cloudflare/`）
2. 实现 `provider.DNSProvider` 接口
3. 在 `main.go` 的 `createProviderFactory` 中添加 `case` 分支
4. 前端 `Credentials.vue` 的服务商下拉框中添加新选项

## 构建与部署

```bash
# 编译
cd web && npm run build && cd ..
go build -o dns-health-monitor.exe .

# 部署所需文件
dns-health-monitor.exe
config.yaml
web/dist/          # 前端静态文件
data/              # 运行时自动创建
```

生产环境注意事项：
- 修改 `config.yaml` 中的 `jwt.secret` 为随机字符串
- 修改默认管理员密码
- 设置 `server.mode` 为 `release`
- 备份 `data/encryption.key`（丢失后已加密的凭证无法解密）
