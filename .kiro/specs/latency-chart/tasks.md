# 实现计划：延迟曲线图表

## 概述

为探测任务详情页和健康监控详情页添加延迟曲线图表功能。后端新增延迟数据查询 API，前端安装 ECharts 并创建可复用的 LatencyChart 组件。

## 任务

- [x] 1. 后端：新增探测任务延迟数据查询接口
  - [x] 1.1 在 `internal/api/status.go` 中新增 `GetTaskLatency` 方法
    - 接收参数：`ip`（必填）、`start_time`（可选，默认24小时前）、`end_time`（可选，默认当前时间）
    - 查询 `ProbeResult` 表，按 `task_id`、`ip`、时间范围过滤
    - 按 `probed_at` 升序返回 `latency_ms`、`probed_at`、`success` 字段
    - 缺少 `ip` 参数返回 400，任务不存在返回 404
    - _Requirements: 1.1, 1.3, 1.4, 1.5, 1.6_
  - [x] 1.2 在 `internal/api/router.go` 中注册路由 `GET /api/tasks/:id/latency`
    - _Requirements: 1.1_

- [x] 2. 后端：新增健康监控延迟数据查询接口
  - [x] 2.1 在 `internal/api/health_monitor.go` 中新增 `GetHealthMonitorLatency` 方法
    - 逻辑与 `GetTaskLatency` 相同，查询 `HealthMonitorResult` 表
    - _Requirements: 1.2, 1.3, 1.4, 1.5, 1.6_
  - [x] 2.2 在 `internal/api/router.go` 中注册路由 `GET /api/health-monitors/:id/latency`
    - _Requirements: 1.2_

- [x] 3. 检查点 - 后端接口完成
  - 确保后端接口可正常返回数据，ask the user if questions arise.

- [x] 4. 前端：安装 ECharts 依赖并创建 LatencyChart 组件
  - [x] 4.1 在 `web/` 目录下安装 `echarts` 和 `vue-echarts` 依赖
    - _Requirements: 2.3, 3.3_
  - [x] 4.2 创建 `web/src/components/LatencyChart.vue` 可复用组件
    - Props: `apiUrl`（API 路径前缀）、`ipList`（IP 列表）、`probeIntervalSec`（检测周期）
    - 包含 IP 选择器（el-select）和日期范围选择器（el-date-picker type="datetimerange"）
    - 默认日期范围为最近 24 小时
    - IP 列表非空时自动选中第一个 IP 并加载数据
    - IP 列表为空时显示提示信息
    - 使用 ECharts 折线图渲染，X 轴为时间，Y 轴为延迟（ms）
    - 显示检测周期粒度信息
    - 支持加载状态和空数据状态
    - 结束日期不能早于开始日期
    - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 4.3, 4.4, 5.1, 5.2, 5.3_

- [x] 5. 前端：集成 LatencyChart 到探测任务详情页
  - [x] 5.1 修改 `web/src/views/TaskDetail.vue`
    - 在基本信息卡片上方引入 LatencyChart 组件
    - 从 IP 管理接口 (`/tasks/:id/ips`) 获取 IP 列表传入组件
    - 传入 `apiUrl` 为 `/tasks/:id`，`probeIntervalSec` 为任务的探测周期
    - _Requirements: 2.1, 4.1_

- [x] 6. 前端：集成 LatencyChart 到健康监控详情页
  - [x] 6.1 修改 `web/src/views/HealthMonitorDetail.vue`
    - 在基本信息卡片上方引入 LatencyChart 组件
    - 从任务详情的 targets 列表中提取 IP 列表传入组件
    - 传入 `apiUrl` 为 `/health-monitors/:id`，`probeIntervalSec` 为任务的探测周期
    - _Requirements: 3.1, 4.2_

- [x] 7. 最终检查点 - 确保所有功能正常
  - 确保探测任务详情页和健康监控详情页的延迟图表正常显示，ask the user if questions arise.

## 备注

- 前端使用 ECharts（`echarts` + `vue-echarts`）作为图表库
- LatencyChart 为可复用组件，通过 props 适配两个详情页
- 不需要新增数据库表，直接查询现有的 ProbeResult 和 HealthMonitorResult 表
- 前后端均使用热重载，修改后自动生效
- 代码注释使用中文
