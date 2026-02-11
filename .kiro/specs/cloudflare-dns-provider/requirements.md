# 需求文档：Cloudflare DNS Provider 及 CDN 故障转移

## 简介

为 DNS 健康监控系统新增 Cloudflare DNS 服务商支持，包括实现 DNSProvider 接口对接 Cloudflare API、前后端凭证管理，以及 Cloudflare 独享的 CDN 故障转移模式（通过 proxied 字段控制 Cloudflare CDN 代理开关）。

## 术语表

- **Cloudflare_Provider**：实现 DNSProvider 接口的 Cloudflare DNS 客户端，通过 Cloudflare API v4 管理 DNS 记录
- **Zone_ID**：Cloudflare 中域名的管理单元标识，所有 DNS 操作需先获取对应的 Zone ID
- **Proxied**：Cloudflare DNS 记录的 CDN 代理开关字段，`true` 表示流量经过 Cloudflare CDN（橙色云），`false` 表示仅 DNS 解析（灰色云）
- **CDN_Failover**：Cloudflare 独享的故障转移模式，通过切换 proxied 字段启用/关闭 CDN 代理来实现故障转移
- **Bearer_Token**：Cloudflare API 的认证令牌，需具备 Zone-DNS-Edit 和 Zone-Zone-Read 权限
- **ProviderFactory**：根据凭证类型创建对应 DNSProvider 实例的工厂函数
- **TaskForm**：前端探测任务创建/编辑表单组件
- **ProviderConfig**：前端服务商凭证字段配置注册表

## 需求

### 需求 1：Cloudflare DNS Provider 接口实现

**用户故事：** 作为系统管理员，我希望系统支持 Cloudflare DNS 服务商，以便我能通过统一接口管理 Cloudflare 上的 DNS 解析记录。

#### 验收标准

1. THE Cloudflare_Provider SHALL 实现 DNSProvider 接口的所有方法（SupportsPause, ListRecords, AddRecord, UpdateRecord, PauseRecord, ResumeRecord, DeleteRecord, UpdateRecordValue, GetRecordValue）
2. THE Cloudflare_Provider SHALL 使用 Bearer_Token 方式向 Cloudflare API v4 发送认证请求
3. WHEN 调用 ListRecords 时，THE Cloudflare_Provider SHALL 先通过域名获取 Zone_ID，再查询指定子域名和记录类型的 DNS 记录列表
4. WHEN 调用 AddRecord 时，THE Cloudflare_Provider SHALL 在对应 Zone 下创建新的 DNS 记录，并返回 Cloudflare 分配的记录 ID
5. WHEN 调用 UpdateRecord 时，THE Cloudflare_Provider SHALL 通过 PUT 请求更新指定记录的子域名、类型、值和 TTL
6. THE Cloudflare_Provider SHALL 返回 SupportsPause() = false，因为 Cloudflare 不支持暂停/启用单条 DNS 记录
7. WHEN 调用 PauseRecord 或 ResumeRecord 时，THE Cloudflare_Provider SHALL 返回"不支持暂停/启用操作"的错误信息
8. WHEN 调用 DeleteRecord 时，THE Cloudflare_Provider SHALL 通过 DELETE 请求删除指定的 DNS 记录
9. WHEN 调用 UpdateRecordValue 时，THE Cloudflare_Provider SHALL 仅更新记录的 content 字段，保留其他属性（type、name、proxied、ttl）不变
10. WHEN 调用 GetRecordValue 时，THE Cloudflare_Provider SHALL 返回指定记录的当前 content 值
11. IF Cloudflare API 返回 success=false 或 HTTP 状态码 >= 300，THEN THE Cloudflare_Provider SHALL 返回包含错误详情的中文错误信息
12. WHEN 查询 Zone_ID 未找到匹配的活跃 Zone 时，THE Cloudflare_Provider SHALL 返回"未找到域名对应的 Zone"错误

### 需求 2：Cloudflare Proxied（CDN 代理）控制能力

**用户故事：** 作为系统管理员，我希望能通过系统控制 Cloudflare DNS 记录的 CDN 代理状态（proxied 字段），以便实现 CDN 故障转移功能。

#### 验收标准

1. THE Cloudflare_Provider SHALL 提供 SetProxied 方法，接受记录 ID 和 proxied 布尔值，更新指定记录的 CDN 代理状态
2. THE Cloudflare_Provider SHALL 提供 GetProxied 方法，接受记录 ID，返回指定记录当前的 proxied 状态
3. WHEN SetProxied 将 proxied 设为 true 时，THE Cloudflare_Provider SHALL 通过 PATCH 或 PUT 请求将记录的 proxied 字段更新为 true
4. WHEN SetProxied 将 proxied 设为 false 时，THE Cloudflare_Provider SHALL 通过 PATCH 或 PUT 请求将记录的 proxied 字段更新为 false
5. IF 更新 proxied 状态失败，THEN THE Cloudflare_Provider SHALL 返回包含 Cloudflare API 错误详情的中文错误信息

### 需求 3：Cloudflare 凭证管理

**用户故事：** 作为系统管理员，我希望能在系统中添加和管理 Cloudflare API 凭证，以便系统能够调用 Cloudflare API 操作 DNS 记录。

#### 验收标准

1. THE ProviderConfig SHALL 包含 cloudflare 服务商配置，定义 api_token 凭证字段（标签为"API Token"，类型为 password，必填）
2. WHEN 用户在凭证管理页面选择 Cloudflare 服务商时，THE 凭证表单 SHALL 动态渲染 API Token 输入字段
3. THE ProviderFactory SHALL 支持 provider_type 为 "cloudflare" 的凭证，使用解密后的 api_token 创建 Cloudflare_Provider 实例
4. IF 创建 Cloudflare_Provider 时 api_token 为空，THEN THE ProviderFactory SHALL 返回"Cloudflare 凭证缺少 api_token"错误

### 需求 4：CDN 故障转移模式

**用户故事：** 作为系统管理员，我希望在使用 Cloudflare 时能选择"CDN 故障转移"模式，当探测目标异常时自动启用 Cloudflare CDN 代理（proxied=true），恢复后自动关闭 CDN 代理（proxied=false）。

#### 验收标准

1. THE 数据模型 SHALL 新增任务类型 "cdn_switch"，表示 CDN 故障转移模式
2. WHEN 创建 cdn_switch 类型任务时，THE 系统 SHALL 要求用户输入一个 CNAME 目标值，作为 CDN 代理的目标域名
3. WHEN cdn_switch 类型任务的探测目标连续失败达到失败阈值时，THE 故障转移执行器 SHALL 将对应 DNS 记录的 proxied 字段从 false 切换为 true
4. WHEN cdn_switch 类型任务的探测目标恢复健康且回切策略为自动时，THE 故障转移执行器 SHALL 将对应 DNS 记录的 proxied 字段从 true 切换回 false
5. WHILE cdn_switch 类型任务处于已切换状态（proxied=true）时，THE 系统 SHALL 在任务状态中标记 IsSwitched=true
6. WHEN cdn_switch 类型任务执行 CDN 切换操作时，THE 系统 SHALL 记录操作日志，包含操作类型（cdn_enable/cdn_disable）、记录 ID 和操作结果
7. THE cdn_switch 任务类型 SHALL 仅在凭证类型为 cloudflare 时可用
8. WHEN cdn_switch 类型任务执行切换时，THE 故障转移执行器 SHALL 保持 DNS 记录的 content 值不变，仅修改 proxied 字段

### 需求 5：前端 CDN 故障转移任务表单

**用户故事：** 作为系统管理员，我希望在创建探测任务时能选择 CDN 故障转移模式，并配置相关参数，以便通过界面管理 Cloudflare CDN 故障转移任务。

#### 验收标准

1. WHEN 用户在任务类型步骤选择凭证为 Cloudflare 类型时，THE TaskForm SHALL 显示第三个任务类型选项"CDN 故障转移"
2. WHEN 用户选择"CDN 故障转移"任务类型时，THE TaskForm SHALL 显示 CNAME 目标值输入字段，提示用户输入 CDN 代理的目标域名
3. WHEN 用户选择"CDN 故障转移"任务类型时，THE TaskForm SHALL 隐藏解析池选择器（CDN 故障转移不需要解析池）
4. WHEN 用户选择"CDN 故障转移"任务类型时，THE TaskForm SHALL 显示回切策略选择（自动回切/保持当前）
5. THE TaskForm SHALL 仅在所选凭证的 provider_type 为 "cloudflare" 时显示"CDN 故障转移"选项
6. WHEN 提交 cdn_switch 类型任务时，THE TaskForm SHALL 验证 CNAME 目标值为非空字符串

### 需求 6：后端 API 支持 CDN 故障转移任务

**用户故事：** 作为系统管理员，我希望后端 API 能正确处理 CDN 故障转移任务的创建、更新和执行，以便系统能完整支持 Cloudflare CDN 故障转移流程。

#### 验收标准

1. WHEN 创建任务时 task_type 为 "cdn_switch"，THE 任务 API SHALL 验证关联凭证的 provider_type 为 "cloudflare"
2. IF 创建 cdn_switch 任务时关联凭证不是 Cloudflare 类型，THEN THE 任务 API SHALL 返回"CDN 故障转移仅支持 Cloudflare 服务商"错误
3. WHEN 创建 cdn_switch 任务时，THE 任务 API SHALL 验证请求中包含非空的 CNAME 目标值
4. THE 数据模型 SHALL 在 ProbeTask 中新增 CDNTarget 字段，存储 CDN 故障转移的 CNAME 目标值
5. WHEN 调度器处理 cdn_switch 类型任务的故障转移时，THE 调度器 SHALL 调用 Cloudflare_Provider 的 SetProxied 方法启用 CDN 代理
6. WHEN 调度器处理 cdn_switch 类型任务的恢复回切时，THE 调度器 SHALL 调用 Cloudflare_Provider 的 SetProxied 方法关闭 CDN 代理
