# 实现计划：高级DNS故障转移前端适配

## 概述

在现有Vue 3 + Element Plus前端基础上，新增解析池管理页面，增强任务相关页面，更新路由和导航。所有修改遵循现有代码风格，使用Composition API + `<script setup>`，中文UI文本和注释。

## 任务

- [x] 1. 更新路由和导航
  - [x] 1.1 在 router.js 中添加解析池路由（/pools 和 /pools/:id）
    - 添加 PoolList 和 PoolDetail 路由配置，使用懒加载
    - _需求: 6.3, 6.4_
  - [x] 1.2 在 App.vue 侧边栏中添加"解析池"菜单项
    - 在"探测任务"菜单项下方添加，使用 Collection 图标
    - 更新 activeMenu 计算属性支持 /pools 路径匹配
    - _需求: 6.1, 6.2, 6.5_

- [x] 2. 实现解析池列表页面
  - [x] 2.1 创建 PoolList.vue 页面
    - 页面头部（标题 + 创建按钮）+ el-table 展示池列表
    - 表格列：池名称、资源类型、操作（查看详情、删除）
    - 调用 GET /api/pools 获取列表，支持 v-loading 加载状态
    - 删除操作：确认对话框 + DELETE /api/pools/:id，处理被引用错误
    - _需求: 1.1, 1.4, 1.5, 1.6_
  - [x] 2.2 在 PoolList.vue 中实现创建解析池对话框
    - el-dialog 包含表单：池名称、资源类型（IP/域名）、探测协议、端口、间隔、超时、失败阈值、恢复阈值
    - 表单验证规则，提交调用 POST /api/pools
    - _需求: 1.2, 1.3_

- [x] 3. 实现解析池详情页面
  - [x] 3.1 创建 PoolDetail.vue 页面
    - 页面头部（池名称 + 返回按钮）+ 基本信息卡片（el-descriptions）
    - 健康摘要统计（健康数/不健康数/总数）
    - 资源列表 el-table：资源值、健康状态（绿色/红色标签）、延迟、最近探测时间、操作（移除）
    - 添加资源对话框：el-dialog + 资源值输入
    - API调用：GET /api/pools/:id、GET /api/pools/:id/resources、GET /api/pools/:id/health、POST /api/pools/:id/resources、DELETE /api/pools/:id/resources/:resource_id
    - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8_

- [x] 4. 检查点 - 验证解析池页面
  - 确保解析池列表和详情页面正常工作，如有问题请告知。

- [x] 5. 增强任务表单
  - [x] 5.1 在 TaskForm.vue 中添加任务类型和记录类型选择器
    - 在域名字段前添加 task_type 选择器（暂停/删除、切换解析）
    - 添加 record_type 选择器（A/AAAA、CNAME）
    - _需求: 3.1, 3.2_
  - [x] 5.2 在 TaskForm.vue 中添加条件显示字段
    - 解析池选择器：task_type === 'switch' 或 (task_type === 'pause_delete' && record_type === 'CNAME') 时显示
    - 回切策略选择器：task_type === 'switch' 时显示（自动回切/保持当前）
    - CNAME阈值配置：record_type === 'CNAME' 时显示（阈值类型 + 阈值数值）
    - 加载解析池列表 GET /api/pools 作为下拉选项
    - _需求: 3.3, 3.4, 3.5, 3.6, 3.7_
  - [x] 5.3 更新 TaskForm.vue 的提交和回填逻辑
    - 提交时将 task_type、record_type、pool_id、switch_back_policy、fail_threshold_type、fail_threshold_value 包含在请求数据中
    - 编辑模式下正确回填所有新增字段
    - _需求: 3.8, 3.9_

- [x] 6. 增强任务列表
  - [x] 6.1 在 TaskList.vue 中添加新列
    - 添加"任务类型"列：显示"暂停/删除"或"切换解析"
    - 添加"记录类型"列：显示"A/AAAA"或"CNAME"
    - 添加"切换状态"列：is_switched 为 true 时显示红色"已切换"标签
    - 如果任务关联了解析池，显示池名称
    - _需求: 4.1, 4.2, 4.3, 4.4_

- [x] 7. 增强任务详情
  - [x] 7.1 在 TaskDetail.vue 基本信息区域添加新字段
    - 添加任务类型、记录类型、关联解析池、回切策略到 el-descriptions
    - 当 is_switched 为 true 时显示切换状态卡片（原始值、当前值）
    - _需求: 5.1, 5.2_
  - [x] 7.2 在 TaskDetail.vue 中添加 CNAME 信息标签页
    - 当 record_type === 'CNAME' 时显示新标签页
    - 展示目标IP列表、各IP健康状态、失败计数、阈值配置
    - 数据来源：GET /api/tasks/:id/history 返回的 cname_info
    - _需求: 5.3, 5.4_
  - [x] 7.3 增强 TaskDetail.vue 操作日志筛选
    - 添加 operation_type 下拉筛选（暂停、删除、恢复、添加、切换）
    - 添加时间范围选择器（el-date-picker，开始时间和结束时间）
    - 将 operation_type、start_time、end_time 参数传递给 GET /api/tasks/:id/logs
    - _需求: 5.5, 5.6, 5.7_

- [x] 8. 最终检查点 - 验证所有修改
  - 确保所有页面正常工作，如有问题请告知。
