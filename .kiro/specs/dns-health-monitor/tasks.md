# 实现计划：DNS 健康检测自动暂停/恢复系统

## 概述

按模块化方式逐步实现系统，从底层基础模块开始，逐步向上构建业务逻辑和 Web 层。每个任务构建在前一个任务之上，确保无孤立代码。

## 任务

- [x] 1. 项目初始化与数据模型
  - [x] 1.1 初始化 Go 项目结构和依赖
    - 创建 `go.mod`，引入 gin、gorm、go-sqlite3、golang-jwt、gopter、testify 等依赖
    - 创建目录结构：`internal/model`、`internal/database`、`internal/prober`、`internal/provider`、`internal/scheduler`、`internal/cache`、`internal/crypto`、`internal/api`、`web/`
    - 创建 `main.go` 骨架和 `config.yaml` 配置文件
    - _Requirements: 7.1_

  - [x] 1.2 实现数据模型和数据库初始化
    - 在 `internal/model/model.go` 中定义 User、Credential、ProbeTask、DeletedRecord、ProbeResult、OperationLog 结构体
    - 在 `internal/database/database.go` 中实现 SQLite 初始化和 GORM AutoMigrate
    - 创建默认管理员账户（首次启动时）
    - _Requirements: 7.1, 7.2, 7.3_

- [x] 2. 凭证加密模块
  - [x] 2.1 实现 AES-GCM 加密解密和脱敏函数
    - 在 `internal/crypto/crypto.go` 中实现 Encrypt、Decrypt、MaskSecret 函数
    - Encrypt/Decrypt 使用 AES-256-GCM，密钥 32 字节
    - MaskSecret 显示前四位 + 星号 + 后四位
    - _Requirements: 8.2, 8.3_

  - [x] 2.2 编写加密模块属性测试
    - **Property 12: AES-GCM 加密 Round-Trip**
    - **Validates: Requirements 8.2**

  - [x] 2.3 编写脱敏函数属性测试
    - **Property 13: 凭证脱敏显示**
    - **Validates: Requirements 8.3**

- [x] 3. 健康探测模块
  - [x] 3.1 实现探测器接口和工厂函数
    - 在 `internal/prober/prober.go` 中定义 Prober 接口、ProbeResult 结构体、ProbeProtocol 类型和 NewProber 工厂函数
    - _Requirements: 1.7_

  - [x] 3.2 实现 ICMP 探测器
    - 在 `internal/prober/icmp.go` 中实现 ICMPProber，使用 go-ping 库
    - _Requirements: 1.1, 1.6_

  - [x] 3.3 实现 TCP 探测器
    - 在 `internal/prober/tcp.go` 中实现 TCPProber，使用 net.DialTimeout
    - _Requirements: 1.2, 1.6_

  - [x] 3.4 实现 UDP 探测器
    - 在 `internal/prober/udp.go` 中实现 UDPProber
    - _Requirements: 1.3, 1.6_

  - [x] 3.5 实现 HTTP 探测器
    - 在 `internal/prober/http.go` 中实现 HTTPProber，2xx/3xx 为健康
    - _Requirements: 1.4, 1.6_

  - [x] 3.6 实现 HTTPS 探测器
    - 在 `internal/prober/https.go` 中实现 HTTPSProber
    - _Requirements: 1.5, 1.6_

  - [x] 3.7 编写 HTTP/HTTPS 状态码判定属性测试
    - **Property 1: HTTP/HTTPS 状态码判定**
    - **Validates: Requirements 1.4, 1.5**

  - [x] 3.8 编写探测结果结构完整性属性测试
    - **Property 3: 探测结果结构完整性**
    - **Validates: Requirements 1.7**

- [x] 4. 检查点 - 基础模块验证
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 5. 云服务商对接模块
  - [x] 5.1 实现 DNSProvider 统一接口
    - 在 `internal/provider/provider.go` 中定义 DNSProvider 接口和 DNSRecord 结构体
    - 接口包含 SupportsPause()、ListRecords、AddRecord、UpdateRecord、PauseRecord、ResumeRecord、DeleteRecord
    - _Requirements: 2.1_

  - [x] 5.2 实现阿里云 HMAC-SHA1 签名器
    - 在 `internal/provider/aliyun/signer.go` 中实现签名算法
    - 包含公共参数填充、特殊 URL 编码、HMAC-SHA1 计算
    - _Requirements: 2.8_

  - [x] 5.3 实现阿里云 DNS 客户端
    - 在 `internal/provider/aliyun/client.go` 中实现 AliyunDNSClient
    - 实现 DescribeSubDomainRecords、AddDomainRecord、UpdateDomainRecord、SetDomainRecordStatus、DeleteDomainRecord
    - SupportsPause() 返回 true
    - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_

  - [x] 5.4 编写阿里云签名和 API 调用单元测试
    - 使用 httptest.Server mock 阿里云 API 端点
    - 验证请求参数和签名正确性
    - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_

- [x] 6. 已删除记录缓存模块
  - [x] 6.1 实现 DeletedRecordCache
    - 在 `internal/cache/cache.go` 中实现 Add、Remove、ListByTask、CleanByTask 方法
    - 基于 GORM 操作 DeletedRecord 表
    - _Requirements: 3.6, 3.7_

  - [x] 6.2 编写已删除记录缓存 Round-Trip 属性测试
    - **Property 7: 已删除记录缓存 Round-Trip**
    - **Validates: Requirements 3.6, 3.7**

- [x] 7. 监控调度器
  - [x] 7.1 实现调度器核心逻辑
    - 在 `internal/scheduler/scheduler.go` 中实现 Scheduler 结构体
    - 实现 Start（加载任务并启动 goroutine）、AddTask、UpdateTask、RemoveTask
    - 实现单任务探测循环：获取记录 → 合并缓存 → 探测 → 阈值判定 → 执行操作
    - 实现最后一条记录保护逻辑
    - 实现探测结果和操作日志写入数据库
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.8, 6.2, 6.4, 7.4_

  - [x] 7.2 编写 IP 列表合并属性测试
    - **Property 4: IP 列表合并完整性**
    - **Validates: Requirements 3.2**

  - [x] 7.3 编写失败阈值触发操作属性测试
    - **Property 5: 失败阈值触发正确操作**
    - **Validates: Requirements 3.3, 3.4**

  - [x] 7.4 编写恢复阈值触发恢复属性测试
    - **Property 6: 恢复阈值触发恢复操作**
    - **Validates: Requirements 3.5**

  - [x] 7.5 编写最后一条记录保护属性测试
    - **Property 8: 最后一条记录保护**
    - **Validates: Requirements 3.8**

- [x] 8. 检查点 - 核心业务逻辑验证
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 9. Web API 层 - 认证
  - [x] 9.1 实现 JWT 认证中间件和登录/登出接口
    - 在 `internal/api/middleware.go` 中实现 JWT 验证中间件
    - 在 `internal/api/auth.go` 中实现 POST /api/login 和 POST /api/logout
    - 密码使用 bcrypt 哈希验证
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_

  - [x] 9.2 编写过期 JWT Token 属性测试
    - **Property 9: 过期 JWT Token 拒绝**
    - **Validates: Requirements 4.4**

- [x] 10. Web API 层 - 业务接口
  - [x] 10.1 实现凭证管理接口
    - 在 `internal/api/credential.go` 中实现 GET/POST/DELETE /api/credentials
    - 创建时加密存储，查询时脱敏返回
    - _Requirements: 8.1, 8.2, 8.3, 8.5_

  - [x] 10.2 实现探测任务 CRUD 接口
    - 在 `internal/api/task.go` 中实现 GET/POST/PUT/DELETE /api/tasks 和 GET /api/tasks/:id
    - 创建/更新时验证参数，操作后通知 Scheduler
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_

  - [x] 10.3 实现状态与历史查询接口
    - 在 `internal/api/status.go` 中实现 GET /api/tasks/:id/history 和 GET /api/tasks/:id/logs
    - 历史记录按时间倒序，支持 IP 筛选
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [x] 10.4 实现路由注册和 DTO 定义
    - 在 `internal/api/router.go` 中注册所有路由，挂载中间件
    - 在 `internal/api/dto.go` 中定义请求/响应 DTO 结构体
    - _Requirements: 4.1_

  - [x] 10.5 编写探测任务参数验证属性测试
    - **Property 10: 探测任务数值参数验证**
    - **Validates: Requirements 5.2**

  - [x] 10.6 编写探测历史排序属性测试
    - **Property 11: 探测历史时间倒序**
    - **Validates: Requirements 6.3**

- [x] 11. 检查点 - API 层验证
  - 确保所有测试通过，如有问题请向用户确认。

- [x] 12. 主程序入口集成
  - [x] 12.1 实现 main.go 启动逻辑
    - 加载配置文件
    - 初始化数据库
    - 创建 Scheduler 并启动
    - 创建 Gin 路由并启动 HTTP 服务
    - 嵌入前端静态文件服务（`web/dist/`）
    - 优雅关闭（监听系统信号，停止调度器）
    - _Requirements: 7.4_

- [x] 13. 前端 Web 控制台
  - [x] 13.1 初始化 Vue 3 + Vite + Element Plus 项目
    - 在 `web/` 目录初始化前端项目
    - 配置 API 代理、路由、axios 封装
    - _Requirements: 4.1_

  - [x] 13.2 实现登录页面和路由守卫
    - 实现 Login.vue 登录表单
    - 实现前端路由守卫（未登录重定向到登录页）
    - Token 存储在 localStorage
    - _Requirements: 4.1, 4.2, 4.3, 4.5_

  - [x] 13.3 实现凭证管理页面
    - 实现 Credentials.vue，展示凭证列表（脱敏）、添加和删除凭证
    - _Requirements: 8.1, 8.3, 8.5_

  - [x] 13.4 实现探测任务列表和表单页面
    - 实现 TaskList.vue 展示任务列表（域名、协议、周期、状态）
    - 实现 TaskForm.vue 创建/编辑任务表单（含凭证选择）
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_

  - [x] 13.5 实现任务详情和历史页面
    - 实现 TaskDetail.vue 展示 IP 状态列表和探测历史
    - 实现 Dashboard.vue 总览页面
    - _Requirements: 6.1, 6.3_

- [x] 14. 最终检查点
  - 确保所有测试通过，如有问题请向用户确认。

## 备注

- 标记 `*` 的子任务为可选测试任务，可跳过以加快 MVP 进度
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点确保增量验证
- 属性测试验证通用正确性属性，单元测试验证具体示例和边界情况
- 新功能只运行该功能的测试，最终做全量测试
