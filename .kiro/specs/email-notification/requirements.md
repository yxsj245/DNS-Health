# 需求文档

## 简介

为 DNS 健康监控系统增加通知功能模块。当前阶段实现邮件通知渠道，但通知模块需设计为可扩展架构，方便后续添加其他通知渠道（如钉钉、飞书等）。系统在检测到健康状态变化时自动发送通知。邮件通知需要有精美的封面和细致的排版。用户可以在独立的通知设置页面配置 SMTP 信息和任务通知偏好，在独立的通知记录页面查看发送历史。两个页面均在侧边栏中作为独立导航项。

## 术语表

- **Notification_Module**: 通知模块，采用可扩展架构设计，定义统一的通知发送接口，当前实现邮件渠道
- **Notification_Channel**: 通知渠道接口，定义统一的发送方法，邮件渠道为其第一个实现
- **Email_Channel**: 邮件通知渠道，Notification_Channel 的具体实现，通过 SMTP 发送邮件
- **SMTP_Config**: 用户配置的 SMTP 邮件服务器连接信息（服务器地址、端口、用户名、密码、发件人地址、收件人地址）
- **Notification_Setting**: 用户为每个探测任务配置的通知偏好（启用哪些事件类型的通知）
- **Event_Type**: 触发通知的事件类型，包括：故障转移（failover）、恢复（recovery）、连续失败告警（consecutive_fail）
- **Email_Template**: 邮件的 HTML 模板，包含封面设计和排版样式
- **Notification_Settings_Page**: 前端通知设置页面（侧边栏独立入口），用于配置 SMTP 和任务通知偏好
- **Notification_Log_Page**: 前端通知记录页面（侧边栏独立入口），用于查看通知发送历史

## 需求

### 需求 1：通知模块可扩展架构

**用户故事：** 作为开发者，我希望通知模块采用可扩展架构，以便后续能方便地添加其他通知渠道。

#### 验收标准

1. THE Notification_Module SHALL 定义统一的 Notification_Channel 接口，包含发送通知的方法签名
2. THE Email_Channel SHALL 实现 Notification_Channel 接口，作为第一个通知渠道实现
3. WHEN 触发通知事件时，THE Notification_Module SHALL 通过 Notification_Channel 接口调用具体渠道的发送方法

### 需求 2：SMTP 邮件服务器配置

**用户故事：** 作为系统管理员，我希望配置 SMTP 邮件服务器信息，以便系统能够发送邮件通知。

#### 验收标准

1. THE Notification_Settings_Page SHALL 提供 SMTP 配置表单，包含服务器地址、端口、用户名、密码、发件人地址和收件人地址字段
2. WHEN 用户提交 SMTP 配置时，THE Notification_Module SHALL 验证所有必填字段均已填写
3. WHEN 用户点击"测试连接"按钮时，THE Email_Channel SHALL 尝试连接 SMTP 服务器并发送一封测试邮件，返回连接结果
4. WHEN SMTP 配置保存成功时，THE Notification_Module SHALL 将密码字段加密存储到数据库中
5. IF SMTP 连接测试失败，THEN THE Email_Channel SHALL 返回具体的错误信息（如连接超时、认证失败等）

### 需求 3：任务通知类型设置

**用户故事：** 作为用户，我希望在通知设置页面集中管理所有探测任务的通知偏好，以便只接收我关心的事件通知。

#### 验收标准

1. THE Notification_Settings_Page SHALL 在 SMTP 配置区域下方显示所有探测任务的通知设置列表，每个任务旁边有各事件类型的通知开关
2. WHEN 用户在通知设置页面为某个任务启用"故障转移"通知类型时，THE Notification_Module SHALL 在该任务发生故障转移时发送通知
3. WHEN 用户在通知设置页面为某个任务启用"恢复"通知类型时，THE Notification_Module SHALL 在该任务从故障中恢复时发送通知
4. WHEN 用户在通知设置页面为某个任务启用"连续失败告警"通知类型时，THE Notification_Module SHALL 在该任务连续探测失败达到阈值时发送通知
5. WHEN 用户在通知设置页面修改任务的通知设置时，THE Notification_Module SHALL 立即持久化设置到数据库
6. THE Notification_Settings_Page SHALL 提供"全部启用"和"全部禁用"的批量操作按钮
7. THE Notification_Settings_Page SHALL 作为所有通知相关配置的唯一入口，探测任务页面不包含任何通知设置功能

### 需求 4：邮件内容与排版

**用户故事：** 作为用户，我希望收到的通知邮件有精美的封面和清晰的排版，以便快速了解告警详情。

#### 验收标准

1. THE Email_Template SHALL 包含系统品牌封面区域，显示系统名称"DNS 健康监控"和事件类型标题
2. THE Email_Template SHALL 使用 HTML 表格布局，确保在主流邮件客户端（Gmail、Outlook、QQ邮箱）中正确渲染
3. WHEN 发送故障转移通知时，THE Email_Template SHALL 包含以下信息：任务名称（域名+子域名）、故障转移时间、原始解析值、切换后解析值、当前健康状态
4. WHEN 发送恢复通知时，THE Email_Template SHALL 包含以下信息：任务名称、恢复时间、恢复后的解析值、故障持续时长
5. WHEN 发送连续失败告警时，THE Email_Template SHALL 包含以下信息：任务名称、告警时间、连续失败次数、失败的 IP 地址列表、探测协议和端口
6. THE Email_Template SHALL 使用不同的颜色主题区分事件类型：故障转移使用红色系、恢复使用绿色系、连续失败告警使用橙色系
7. THE Email_Template SHALL 生成有效的 HTML 字符串，且该 HTML 字符串经过模板渲染后可被还原解析

### 需求 5：通知触发与发送

**用户故事：** 作为用户，我希望系统在检测到健康状态变化时自动发送邮件通知，以便及时了解 DNS 解析状况。

#### 验收标准

1. WHEN 探测任务触发故障转移操作时，THE Notification_Module SHALL 检查该任务的通知设置并通过对应渠道发送通知
2. WHEN 探测任务从故障中恢复时，THE Notification_Module SHALL 检查该任务的通知设置并发送恢复通知
3. WHEN 探测任务连续失败次数达到用户设定的告警阈值时，THE Notification_Module SHALL 发送连续失败告警通知
4. IF SMTP 配置未设置或无效，THEN THE Notification_Module SHALL 跳过邮件发送并记录警告日志
5. IF 邮件发送失败，THEN THE Notification_Module SHALL 记录错误日志，包含失败原因和任务信息
6. THE Notification_Module SHALL 异步发送通知，避免阻塞探测任务的正常执行

### 需求 6：通知记录与历史查看

**用户故事：** 作为用户，我希望在独立的通知记录页面查看通知发送历史，以便确认通知是否成功送达。

#### 验收标准

1. THE Notification_Module SHALL 将每次通知发送记录保存到数据库，包含发送时间、事件类型、任务ID、渠道类型、发送状态和错误信息
2. THE Notification_Log_Page SHALL 作为侧边栏独立导航项，显示最近的通知发送记录列表
3. THE Notification_Log_Page SHALL 支持按任务和事件类型筛选通知记录
4. WHEN 查看通知记录时，THE Notification_Log_Page SHALL 显示发送状态（成功/失败）和失败原因

### 需求 7：前端页面导航

**用户故事：** 作为用户，我希望通知相关页面在侧边栏中有独立的导航入口，以便快速访问。

#### 验收标准

1. THE Notification_Settings_Page SHALL 在侧边栏中作为独立菜单项显示，使用合适的图标
2. THE Notification_Log_Page SHALL 在侧边栏中作为独立菜单项显示，使用合适的图标
3. WHEN 用户点击侧边栏的通知设置菜单项时，THE Notification_Settings_Page SHALL 正确加载并显示 SMTP 配置和任务通知设置
4. WHEN 用户点击侧边栏的通知记录菜单项时，THE Notification_Log_Page SHALL 正确加载并显示通知发送历史
