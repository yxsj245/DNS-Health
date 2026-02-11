# 需求文档

## 简介

DNS 健康检测自动暂停/恢复系统：通过周期性健康探测监控域名解析记录中各 IP 的可用性，当 IP 不可达时自动暂停或删除对应 DNS 记录，当 IP 恢复时自动恢复或重新添加记录。系统提供 Web 控制台供用户配置探测任务、查看状态和历史。

## 术语表

- **Health_Prober**：健康探测模块，负责对目标 IP 执行 ICMP、TCP、UDP、HTTP、HTTPS 五种协议的健康检测
- **DNS_Provider**：云服务商对接模块，负责与阿里云等 DNS 服务商 API 交互，执行记录查询、添加、更新、暂停、启用、删除操作
- **Aliyun_DNS_Client**：阿里云 DNS 对接客户端，DNS_Provider 的具体实现
- **Monitor_Scheduler**：监控调度器，按用户配置的周期调度探测任务并根据结果执行 DNS 记录操作
- **Deleted_Record_Cache**：已删除记录缓存，本地持久化存储因探测失败而被删除的 DNS 记录信息，确保这些 IP 继续被探测
- **Web_Console**：Web 控制台，提供用户认证、探测任务配置、状态查看等功能的独立 Web 应用
- **Probe_Task**：探测任务，用户配置的一条域名健康探测规则，包含域名、探测协议、探测周期、超时阈值、失败阈值等参数
- **Probe_Result**：探测结果，单次对某个 IP 执行健康探测后的结果记录，包含状态、延迟、时间戳等
- **DNS_Record**：DNS 解析记录，包含记录 ID、域名、记录类型（A/AAAA）、IP 值、状态等信息

## 需求

### 需求 1：健康探测

**用户故事：** 作为系统管理员，我希望对域名解析的各个 IP 地址执行多协议健康探测，以便及时发现不可用的 IP。

#### 验收标准

1. WHEN 用户指定 ICMP 探测协议时，THE Health_Prober SHALL 向目标 IP 发送 ICMP Echo 请求并根据是否收到回复判定健康状态
2. WHEN 用户指定 TCP 探测协议时，THE Health_Prober SHALL 尝试与目标 IP 的指定端口建立 TCP 连接，连接成功则判定为健康
3. WHEN 用户指定 UDP 探测协议时，THE Health_Prober SHALL 向目标 IP 的指定端口发送 UDP 数据包并根据响应判定健康状态
4. WHEN 用户指定 HTTP 探测协议时，THE Health_Prober SHALL 向目标 IP 发送 HTTP 请求并根据响应状态码判定健康状态
5. WHEN 用户指定 HTTPS 探测协议时，THE Health_Prober SHALL 向目标 IP 发送 HTTPS 请求并根据响应状态码判定健康状态
6. WHEN 探测请求在用户配置的超时时间内未收到响应时，THE Health_Prober SHALL 将该次探测标记为失败
7. THE Health_Prober SHALL 返回每次探测的结果，包含探测状态（成功/失败）、响应延迟和时间戳

### 需求 2：云服务商 DNS 操作

**用户故事：** 作为系统管理员，我希望系统能够通过云服务商 API 管理 DNS 解析记录，以便自动化暂停和恢复操作。

#### 验收标准

1. THE DNS_Provider SHALL 定义统一的接口，支持查询主机记录记录、添加记录、更新记录、暂停记录、启用记录、删除记录操作，并声明该服务商是否支持暂停/启用操作
2. WHEN 调用查询主机记录记录操作时，THE Aliyun_DNS_Client SHALL 使用 DescribeSubDomainRecords API 获取指定域名的所有 A 和 AAAA 类型记录
3. WHEN 调用添加记录操作时，THE Aliyun_DNS_Client SHALL 使用 AddDomainRecord API 创建新的 DNS 记录并返回记录 ID
4. WHEN 调用更新记录操作时，THE Aliyun_DNS_Client SHALL 使用 UpdateDomainRecord API 更新指定记录的值
5. WHEN 调用暂停记录操作时，THE Aliyun_DNS_Client SHALL 使用 SetDomainRecordStatus API 将记录状态设置为 Disable
6. WHEN 调用启用记录操作时，THE Aliyun_DNS_Client SHALL 使用 SetDomainRecordStatus API 将记录状态设置为 Enable
7. WHEN 调用删除记录操作时，THE Aliyun_DNS_Client SHALL 使用 DeleteDomainRecord API 删除指定记录
8. THE Aliyun_DNS_Client SHALL 使用 HMAC-SHA1 签名算法对所有 API 请求进行认证，签名参数包含 AccessKeyId、时间戳、随机数等公共参数

### 需求 3：监控调度与自动暂停/恢复

**用户故事：** 作为系统管理员，我希望系统按配置的周期自动执行探测并根据结果暂停或恢复 DNS 记录，以便保证域名解析始终指向可用的 IP。

#### 验收标准

1. WHEN 探测周期到达时，THE Monitor_Scheduler SHALL 从 DNS_Provider 获取该探测任务对应域名的当前所有解析记录（A 和 AAAA 类型）
2. WHEN 获取到解析记录后，THE Monitor_Scheduler SHALL 合并 Deleted_Record_Cache 中该域名的已删除记录，形成完整的待探测 IP 列表
3. WHEN 某个 IP 连续探测失败次数达到用户配置的失败阈值且该 DNS_Provider 支持暂停操作时，THE Monitor_Scheduler SHALL 使用暂停操作禁用该 IP 对应的 DNS 记录
4. WHEN 某个 IP 连续探测失败次数达到用户配置的失败阈值且该 DNS_Provider 不支持暂停操作时，THE Monitor_Scheduler SHALL 使用删除操作移除该 IP 对应的 DNS 记录，并将该记录信息存入 Deleted_Record_Cache
5. WHEN 一个已暂停或已删除的 IP 连续探测成功次数达到用户配置的恢复阈值时，THE Monitor_Scheduler SHALL 自动恢复该记录（已暂停的执行启用操作，已删除的执行添加操作）
6. WHEN 一条 DNS 记录因探测失败被删除后，THE Deleted_Record_Cache SHALL 持久化保存该记录的域名、主机记录、记录类型、IP 值和 TTL 信息
7. WHEN Monitor_Scheduler 执行探测周期时，THE Deleted_Record_Cache SHALL 提供该域名下所有已删除记录的 IP 列表用于继续探测
8. WHILE 一个域名下仅剩最后一条健康的解析记录时，THE Monitor_Scheduler SHALL 保留该记录不执行暂停或删除操作，即使该 IP 探测失败

### 需求 4：Web 控制台认证

**用户故事：** 作为系统管理员，我希望通过安全的登录认证访问控制台，以便防止未授权的访问。

#### 验收标准

1. WHEN 用户访问 Web_Console 的任何受保护页面且未登录时，THE Web_Console SHALL 重定向用户到登录页面
2. WHEN 用户提交正确的用户名和密码时，THE Web_Console SHALL 生成会话令牌并允许用户访问受保护页面
3. WHEN 用户提交错误的用户名或密码时，THE Web_Console SHALL 显示"用户名或密码错误"的提示信息并保持在登录页面
4. WHEN 用户的会话令牌过期时，THE Web_Console SHALL 要求用户重新登录
5. WHEN 用户点击退出登录时，THE Web_Console SHALL 销毁当前会话并重定向到登录页面

### 需求 5：探测任务配置管理

**用户故事：** 作为系统管理员，我希望通过 Web 控制台配置和管理探测任务，以便灵活控制对哪些域名进行健康探测。

#### 验收标准

1. WHEN 用户创建探测任务时，THE Web_Console SHALL 要求用户提供域名、云服务商类型、探测协议、探测端口（TCP/UDP/HTTP/HTTPS 时）、探测周期（秒）、超时时间（毫秒）、连续失败阈值和连续恢复阈值
2. WHEN 用户提交探测任务配置时，THE Web_Console SHALL 验证所有必填字段非空且数值参数为正整数
3. WHEN 用户修改已有探测任务时，THE Monitor_Scheduler SHALL 在下一个探测周期使用更新后的配置
4. WHEN 用户删除探测任务时，THE Monitor_Scheduler SHALL 停止该任务的周期探测，并清理该任务关联的 Deleted_Record_Cache 数据
5. THE Web_Console SHALL 展示所有探测任务的列表，包含域名、探测协议、探测周期、当前状态（运行中/已停止）

### 需求 6：探测状态与历史查看

**用户故事：** 作为系统管理员，我希望查看每个域名下各 IP 的实时探测状态和历史记录，以便了解域名解析的健康状况。

#### 验收标准

1. WHEN 用户查看某个探测任务的详情时，THE Web_Console SHALL 展示该域名下所有 IP（包括已删除缓存中的 IP）的当前状态（健康/不健康/已暂停/已删除）
2. WHEN 每次探测完成后，THE Monitor_Scheduler SHALL 记录探测结果，包含目标 IP、探测时间、探测状态、响应延迟
3. WHEN 用户查看探测历史时，THE Web_Console SHALL 按时间倒序展示探测记录列表，支持按 IP 地址筛选
4. WHEN DNS 记录状态发生变更（暂停、删除、恢复、重新添加）时，THE Monitor_Scheduler SHALL 记录操作日志，包含操作类型、目标记录、操作时间和操作结果

### 需求 7：数据持久化

**用户故事：** 作为系统管理员，我希望系统的配置和运行数据持久化存储，以便系统重启后能恢复运行状态。

#### 验收标准

1. THE Web_Console SHALL 将探测任务配置、用户账户信息持久化存储到本地数据库
2. THE Monitor_Scheduler SHALL 将探测结果和操作日志持久化存储到本地数据库
3. THE Deleted_Record_Cache SHALL 将已删除记录信息持久化存储到本地数据库
4. WHEN 系统重启时，THE Monitor_Scheduler SHALL 从数据库加载所有探测任务配置和 Deleted_Record_Cache 数据，恢复探测调度

### 需求 8：云服务商凭证管理

**用户故事：** 作为系统管理员，我希望通过 Web 控制台管理云服务商的 API 凭证，以便系统能够调用云服务商 API。

#### 验收标准

1. WHEN 用户添加云服务商凭证时，THE Web_Console SHALL 要求用户提供服务商类型（如阿里云）、AccessKey ID 和 AccessKey Secret
2. THE Web_Console SHALL 使用 AES-GCM 对称加密算法对 AccessKey ID 和 AccessKey Secret 进行加密后存储到数据库，禁止明文存储
3. WHEN 用户通过 Web_Console 查看已配置的凭证时，THE Web_Console SHALL 仅显示 AccessKey ID 和 AccessKey Secret 的脱敏信息（仅展示前四位和后四位，中间用星号替代）
4. WHEN DNS_Provider 需要使用凭证调用 API 时，THE Web_Console SHALL 在内存中解密凭证，解密后的明文仅在 API 调用期间存在
5. WHEN 用户创建探测任务时，THE Web_Console SHALL 允许用户选择已配置的云服务商凭证
