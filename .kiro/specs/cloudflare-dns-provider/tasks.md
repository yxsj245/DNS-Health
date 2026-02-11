# 实现计划：Cloudflare DNS Provider 及 CDN 故障转移

## 概述

按照增量方式实现 Cloudflare DNS Provider 支持和 CDN 故障转移功能。从底层 Provider 实现开始，逐步向上集成到调度器和前端。

## 任务

- [x] 1. 实现 Cloudflare DNS Provider 核心
  - [x] 1.1 创建 `internal/provider/cloudflare/client.go`，实现 CloudflareDNSClient 结构体和基础 HTTP 请求方法
    - 实现 `NewCloudflareDNSClient(apiToken string)` 构造函数
    - 实现 `doRequest` 通用请求方法（Bearer Token 认证、JSON 序列化/反序列化、错误处理）
    - 实现 `getZoneID` 方法（含 sync.Map 缓存）
    - 定义 Cloudflare API 响应结构体（CFStatus, ZonesResp, RecordsResp, CFDNSRecord）
    - _Requirements: 1.2, 1.3, 1.11, 1.12_

  - [x] 1.2 实现 DNSProvider 接口方法
    - 实现 `SupportsPause()` 返回 false
    - 实现 `ListRecords`：获取 ZoneID → 查询 DNS 记录 → 转换为统一 DNSRecord 格式
    - 实现 `AddRecord`：获取 ZoneID → POST 创建记录 → 返回记录 ID
    - 实现 `UpdateRecord`：PUT 更新记录
    - 实现 `PauseRecord` 和 `ResumeRecord`：返回不支持错误
    - 实现 `DeleteRecord`：DELETE 删除记录
    - 实现 `UpdateRecordValue`：先获取记录详情，仅更新 content 字段
    - 实现 `GetRecordValue`：获取记录详情，返回 content 值
    - _Requirements: 1.1, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 1.10_

  - [x] 1.3 实现 Proxied 控制方法（ProxiedController 接口）
    - 在 `internal/provider/provider.go` 中定义 `ProxiedController` 接口（SetProxied, GetProxied）
    - 在 CloudflareDNSClient 中实现 `SetProxied`：获取记录详情 → PUT 更新 proxied 字段
    - 在 CloudflareDNSClient 中实现 `GetProxied`：获取记录详情 → 返回 proxied 值
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [ ]* 1.4 编写 CloudflareProvider 单元测试
    - 使用 httptest.NewServer mock Cloudflare API
    - 测试所有 DNSProvider 接口方法的正常和异常路径
    - 测试 SetProxied/GetProxied 方法
    - 测试 Zone ID 缓存行为
    - _Requirements: 1.1-1.12, 2.1-2.5_

  - [ ]* 1.5 编写属性测试：API 错误响应传播
    - **Property 2: API 错误响应传播**
    - **Validates: Requirements 1.11, 2.5**

  - [ ]* 1.6 编写属性测试：UpdateRecordValue 属性不变量
    - **Property 3: UpdateRecordValue 属性不变量**
    - **Validates: Requirements 1.9, 4.8**

- [x] 2. 集成 Cloudflare Provider 到后端
  - [x] 2.1 扩展 ProviderFactory 支持 Cloudflare
    - 在 `main.go` 的 `createProviderFactory` 中新增 "cloudflare" case
    - 从解密后的凭证字段中提取 api_token
    - 验证 api_token 非空，为空时返回中文错误信息
    - _Requirements: 3.3, 3.4_

  - [ ]* 2.2 编写属性测试：ProviderFactory Cloudflare 支持
    - **Property 4: ProviderFactory Cloudflare 支持**
    - **Validates: Requirements 3.3**

- [x] 3. 扩展数据模型支持 CDN 故障转移
  - [x] 3.1 扩展 ProbeTask 数据模型
    - 在 `internal/model/model.go` 中新增 `TaskTypeCDNSwitch` 常量
    - 在 ProbeTask 结构体中新增 `CDNTarget string` 字段
    - 更新 `IsValidTaskType` 函数支持 "cdn_switch"
    - _Requirements: 4.1, 6.4_

  - [x] 3.2 扩展任务 API 验证逻辑
    - 在 `internal/api/task.go` 的 `validateTaskRequest` 中新增 cdn_switch 验证：
      - 验证关联凭证的 provider_type 为 "cloudflare"
      - 验证 CDNTarget 非空
      - cdn_switch 不需要解析池
    - 在 `CreateTaskRequest` 中新增 `CDNTarget` 字段
    - 在任务创建和更新逻辑中处理 CDNTarget 字段
    - _Requirements: 4.2, 4.7, 6.1, 6.2, 6.3_

  - [ ]* 3.3 编写属性测试：cdn_switch 任务类型凭证约束
    - **Property 7: cdn_switch 任务类型凭证约束**
    - **Validates: Requirements 4.7, 6.1, 6.2**

- [x] 4. Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 5. 实现 CDN 故障转移调度逻辑
  - [x] 5.1 实现 CDN 故障转移探测方法
    - 在 `internal/scheduler/scheduler.go` 中新增 `executeCDNSwitchProbe` 方法
    - 在 `executeProbe` 中新增 `cdn_switch` 任务类型分发
    - 实现探测逻辑：获取 DNS 记录 → 探测 IP 健康状态 → 根据阈值判断
    - 故障转移：通过类型断言获取 ProxiedController → 调用 SetProxied(true)
    - 回切：检查回切策略 → 调用 SetProxied(false)
    - 更新任务状态（IsSwitched、OriginalValue 等）
    - 记录操作日志（cdn_enable/cdn_disable）
    - _Requirements: 4.3, 4.4, 4.5, 4.6, 4.8, 6.5, 6.6_

  - [ ]* 5.2 编写属性测试：CDN 故障转移触发
    - **Property 5: CDN 故障转移触发**
    - **Validates: Requirements 4.3, 4.5**

  - [ ]* 5.3 编写属性测试：CDN 自动回切
    - **Property 6: CDN 自动回切**
    - **Validates: Requirements 4.4**

- [x] 6. 前端支持 Cloudflare 凭证和 CDN 故障转移
  - [x] 6.1 扩展前端服务商配置
    - 在 `web/src/providerConfig.js` 中新增 cloudflare 配置（api_token 字段）
    - _Requirements: 3.1, 3.2_

  - [x] 6.2 扩展 TaskForm 支持 CDN 故障转移
    - 在 `web/src/views/TaskForm.vue` 中：
      - 在任务类型步骤新增"CDN 故障转移"卡片选项
      - 根据所选凭证的 provider_type 动态显示/隐藏 CDN 故障转移选项
      - 选择 CDN 故障转移时显示 CNAME 目标值输入字段和回切策略选择
      - 选择 CDN 故障转移时隐藏解析池选择器
      - 提交时验证 CNAME 目标值非空
      - 在请求数据中包含 task_type="cdn_switch" 和 cdn_target 字段
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

- [x] 7. 最终 Checkpoint - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

## 备注

- 标记 `*` 的任务为可选任务，可跳过以加快 MVP 开发
- 每个任务引用了具体的需求编号，确保可追溯性
- 属性测试验证通用正确性属性，单元测试验证具体示例和边界情况
- 按照用户规则，测试仅针对 Cloudflare 新功能，不做全量测试
- 测试文件在测试完毕后删除
