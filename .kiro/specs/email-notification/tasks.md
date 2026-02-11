# 实现计划：邮件通知功能

## 概述

按照可扩展通知模块架构，先实现后端数据模型和核心接口，再实现邮件渠道和模板，接着集成到调度器，最后实现前端页面。每个步骤增量构建，确保无孤立代码。

## 任务

- [x] 1. 数据模型与数据库迁移
  - [x] 1.1 在 `internal/model/model.go` 中添加 SMTPConfig、NotificationSetting、NotificationLog 三个模型
    - SMTPConfig: Host, Port, Username, PasswordEncrypted, FromAddress, ToAddress
    - NotificationSetting: TaskID(uniqueIndex), NotifyFailover, NotifyRecovery, NotifyConsecFail
    - NotificationLog: TaskID, EventType, ChannelType, Success, ErrorMsg, Detail, SentAt
    - 添加 EventType 常量定义（failover, recovery, consecutive_fail）
    - _Requirements: 2.4, 3.5, 6.1_

  - [x] 1.2 在 `internal/database/database.go` 的 AutoMigrate 中注册三个新模型
    - _Requirements: 2.4, 3.5, 6.1_

- [x] 2. 通知模块核心实现
  - [x] 2.1 创建 `internal/notification/channel.go`，定义 NotificationChannel 接口、NotificationEvent 结构体、ChannelConfig 接口
    - NotificationChannel 接口: Send(ctx, event, config) error, Type() string
    - NotificationEvent 结构体包含所有事件类型所需字段
    - _Requirements: 1.1, 1.2, 1.3_

  - [x] 2.2 创建 `internal/notification/manager.go`，实现 NotificationManager
    - NewNotificationManager(db, encKey, channels) 构造函数
    - Notify(event) 方法：异步 goroutine 查询通知设置和 SMTP 配置，分发到渠道
    - shouldNotify(setting, eventType) 内部方法：根据设置判断是否发送
    - 保存 NotificationLog 记录
    - _Requirements: 1.3, 3.2, 3.3, 3.4, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 6.1_

  - [x]* 2.3 编写 Property 3 属性测试：通知分发匹配
    - **Property 3: 通知分发匹配**
    - 使用 mock channel 验证：对于任意事件类型和通知设置组合，启用时调用 Send，禁用时不调用
    - **Validates: Requirements 3.2, 3.3, 3.4, 5.1, 5.2, 5.3**

- [x] 3. 邮件渠道与模板实现
  - [x] 3.1 创建 `internal/notification/template.go`，实现邮件 HTML 模板渲染
    - RenderEmailHTML(event NotificationEvent) (string, error) 函数
    - 封面区域：系统名称 "DNS 健康监控" + 事件类型标题
    - HTML 表格布局，兼容主流邮件客户端
    - 三种事件类型的不同内容和颜色主题（红/绿/橙）
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_

  - [x]* 3.2 编写 Property 5 属性测试：邮件模板渲染完整性
    - **Property 5: 邮件模板渲染完整性**
    - 使用 gopter 生成随机 NotificationEvent，验证渲染后 HTML 包含系统名称、table 标签、所有必需字段值、对应颜色代码
    - **Validates: Requirements 4.1, 4.2, 4.3, 4.4, 4.5, 4.6**

  - [x] 3.3 创建 `internal/notification/email.go`，实现 EmailChannel
    - 实现 NotificationChannel 接口
    - Send 方法：调用 RenderEmailHTML 渲染模板，通过 net/smtp 发送
    - Type() 返回 "email"
    - SMTPConfig 验证函数 ValidateSMTPConfig
    - TestSMTPConnection 测试连接函数
    - _Requirements: 1.2, 2.2, 2.3, 2.5_

  - [x]* 3.4 编写 Property 1 属性测试：SMTP 配置验证
    - **Property 1: SMTP 配置验证**
    - 使用 gopter 生成随机 SMTP 配置，验证必填字段为空时返回错误，非空时返回成功
    - **Validates: Requirements 2.2**

  - [x]* 3.5 编写 Property 2 属性测试：SMTP 密码加密 round-trip
    - **Property 2: SMTP 密码加密 round-trip**
    - 使用 gopter 生成随机密码字符串，验证加密后不等于明文，解密后等于原始值
    - **Validates: Requirements 2.4**

- [x] 4. 检查点 - 确保所有测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 5. 后端 API 实现
  - [x] 5.1 创建 `internal/api/notification.go`，实现 NotificationHandler
    - GetSMTPConfig: 获取 SMTP 配置（密码脱敏显示）
    - SaveSMTPConfig: 保存 SMTP 配置（密码加密存储）
    - TestSMTP: 测试 SMTP 连接
    - GetNotificationSettings: 获取所有任务的通知设置
    - UpdateNotificationSetting: 更新单个任务的通知设置
    - BatchUpdateSettings: 批量更新（全部启用/禁用）
    - GetNotificationLogs: 获取通知记录（支持 taskId 和 eventType 筛选）
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.5, 3.6, 6.2, 6.3, 6.4_

  - [x] 5.2 在 `internal/api/router.go` 中注册通知相关路由
    - GET/PUT /notification/smtp-config
    - POST /notification/smtp-test
    - GET /notification/settings
    - PUT /notification/settings/:taskId
    - PUT /notification/settings/batch
    - GET /notification/logs
    - _Requirements: 2.1, 3.1, 6.2_

  - [x]* 5.3 编写 Property 4 属性测试：通知设置持久化 round-trip
    - **Property 4: 通知设置持久化 round-trip**
    - 使用内存 SQLite，生成随机通知设置，保存后读取验证一致性
    - **Validates: Requirements 3.5**

  - [x]* 5.4 编写 Property 7 属性测试：通知记录筛选正确性
    - **Property 7: 通知记录筛选正确性**
    - 使用内存 SQLite，生成随机通知记录和筛选条件，验证返回结果匹配筛选条件
    - **Validates: Requirements 6.3**

- [x] 6. 调度器集成
  - [x] 6.1 修改 `internal/scheduler/scheduler.go`，集成 NotificationManager
    - 在 Scheduler 结构体中添加 notificationManager 字段
    - 添加 WithNotificationManager Option
    - 在 triggerCNAMEFailover、triggerDirectFailover 成功后调用 Notify（failover 事件）
    - 在 evaluateCNAMESwitchBack、executeDirectSwitchProbe 回切成功后调用 Notify（recovery 事件）
    - 在 evaluateAndAct 中连续失败达到阈值时调用 Notify（consecutive_fail 事件）
    - _Requirements: 5.1, 5.2, 5.3, 5.6_

  - [x] 6.2 修改 `main.go`，初始化 NotificationManager 并注入调度器和 API
    - 创建 EmailChannel 实例
    - 创建 NotificationManager 实例
    - 通过 WithNotificationManager 注入调度器
    - 创建 NotificationHandler 并注册路由
    - _Requirements: 1.3, 5.1_

- [x] 7. 检查点 - 确保后端编译通过且测试通过
  - 确保所有测试通过，如有问题请询问用户。

- [x] 8. 前端通知设置页面
  - [x] 8.1 创建 `web/src/views/NotificationSettings.vue`
    - SMTP 配置表单区域（el-card）：服务器地址、端口、用户名、密码、发件人、收件人，测试连接按钮，保存按钮
    - 任务通知设置区域（el-card）：el-table 显示任务列表，每行三个 el-switch（故障转移/恢复/连续失败），批量启用/禁用按钮
    - _Requirements: 2.1, 2.2, 2.3, 3.1, 3.5, 3.6, 3.7_

  - [x] 8.2 创建 `web/src/views/NotificationLog.vue`
    - 筛选区域：任务选择器（el-select）、事件类型选择器（el-select）
    - 记录列表（el-table）：发送时间、任务名称、事件类型、渠道、发送状态（el-tag 成功/失败）、错误信息
    - 分页组件
    - _Requirements: 6.2, 6.3, 6.4_

- [x] 9. 前端路由与导航集成
  - [x] 9.1 在 `web/src/router.js` 中添加通知设置和通知记录路由
    - /notifications/settings -> NotificationSettings.vue
    - /notifications/log -> NotificationLog.vue
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [x] 9.2 在 `web/src/App.vue` 侧边栏中添加两个导航菜单项
    - 通知设置：使用 Element Plus 图标（如 Setting 或 Bell）
    - 通知记录：使用 Element Plus 图标（如 Document 或 ChatLineSquare）
    - 更新 activeMenu 计算属性匹配 /notifications 路径
    - _Requirements: 7.1, 7.2_

- [x] 10. 最终检查点 - 确保所有功能正常
  - 确保所有测试通过，如有问题请询问用户。

## 备注

- 标记 `*` 的任务为可选测试任务，可跳过以加快 MVP 开发
- 每个任务引用了具体的需求编号以便追溯
- 属性测试使用项目已有的 `gopter` 库
- 数据库测试使用 SQLite 内存模式
- 测试完成后删除测试文件
- 前端使用 Element Plus 图标库
