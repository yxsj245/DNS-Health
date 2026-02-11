# 需求文档：高级DNS故障转移前端适配

## 简介

本功能为已完成的高级DNS故障转移后端实现提供前端适配。在现有Vue 3 + Element Plus前端基础上，新增解析池管理页面，增强任务表单、任务列表和任务详情页面，以支持新的任务类型、记录类型、解析池关联、回切策略和CNAME阈值配置等功能。

## 术语表

- **Frontend（前端）**: 基于Vue 3 + Element Plus的Web前端应用
- **Pool_Page（解析池页面）**: 解析池管理的列表和详情页面
- **Task_Form（任务表单）**: 创建和编辑探测任务的表单组件
- **Task_List（任务列表）**: 展示所有探测任务的列表页面
- **Task_Detail（任务详情）**: 展示单个任务详细信息的页面
- **Sidebar（侧边栏）**: 应用左侧导航菜单
- **Router（路由）**: Vue Router路由配置
- **API_Client（API客户端）**: 基于Axios的后端API调用层

## 需求

### 需求 1：解析池列表页面

**用户故事：** 作为系统管理员，我希望在前端看到所有解析池的列表，并能创建和删除解析池，以便管理备用资源。

#### 验收标准

1. WHEN 用户访问解析池列表页面时 THEN THE Frontend SHALL 调用 GET /api/pools 接口并以表格形式展示所有解析池
2. WHEN 用户点击"创建解析池"按钮时 THEN THE Frontend SHALL 弹出对话框，包含池名称、资源类型（IP/域名）和探测配置（协议、端口、间隔、超时、阈值）的输入项
3. WHEN 用户提交创建解析池表单时 THEN THE Frontend SHALL 调用 POST /api/pools 接口并在成功后刷新列表
4. WHEN 用户点击删除解析池按钮时 THEN THE Frontend SHALL 弹出确认对话框，确认后调用 DELETE /api/pools/:id 接口
5. IF 删除解析池接口返回错误（池被任务引用） THEN THE Frontend SHALL 显示错误提示信息
6. WHEN 解析池列表加载中时 THEN THE Frontend SHALL 显示加载状态指示器

### 需求 2：解析池详情页面

**用户故事：** 作为系统管理员，我希望查看解析池的详细信息，包括池中资源及其健康状态，并能添加和移除资源。

#### 验收标准

1. WHEN 用户从列表点击某个解析池时 THEN THE Frontend SHALL 导航到解析池详情页面，调用 GET /api/pools/:id 获取池信息
2. WHEN 解析池详情页面加载时 THEN THE Frontend SHALL 调用 GET /api/pools/:id/resources 展示资源列表及健康状态
3. WHEN 解析池详情页面加载时 THEN THE Frontend SHALL 调用 GET /api/pools/:id/health 展示池健康摘要信息
4. WHEN 用户点击"添加资源"按钮时 THEN THE Frontend SHALL 弹出对话框，包含资源值（IP或域名）的输入项
5. WHEN 用户提交添加资源表单时 THEN THE Frontend SHALL 调用 POST /api/pools/:id/resources 接口并在成功后刷新资源列表
6. WHEN 用户点击移除资源按钮时 THEN THE Frontend SHALL 弹出确认对话框，确认后调用 DELETE /api/pools/:id/resources/:resource_id 接口
7. WHEN 资源健康状态为健康时 THEN THE Frontend SHALL 以绿色标签显示"健康"
8. WHEN 资源健康状态为不健康时 THEN THE Frontend SHALL 以红色标签显示"不健康"

### 需求 3：任务表单增强

**用户故事：** 作为系统管理员，我希望在创建或编辑任务时能够选择任务类型、记录类型、关联解析池、配置回切策略和CNAME阈值，以便使用高级故障转移功能。

#### 验收标准

1. WHEN 任务表单加载时 THEN THE Task_Form SHALL 显示任务类型选择器，提供"暂停/删除"和"切换解析"两个选项
2. WHEN 任务表单加载时 THEN THE Task_Form SHALL 显示记录类型选择器，提供"A/AAAA"和"CNAME"两个选项
3. WHEN 用户选择"切换解析"任务类型时 THEN THE Task_Form SHALL 显示解析池选择器和回切策略选择器
4. WHEN 用户选择"暂停/删除"任务类型且记录类型为"CNAME"时 THEN THE Task_Form SHALL 显示解析池选择器
5. WHEN 解析池选择器显示时 THEN THE Task_Form SHALL 调用 GET /api/pools 加载解析池列表作为下拉选项
6. WHEN 用户选择"切换解析"任务类型时 THEN THE Task_Form SHALL 显示回切策略选择器，提供"自动回切"和"保持当前"两个选项
7. WHEN 用户选择"CNAME"记录类型时 THEN THE Task_Form SHALL 显示CNAME阈值配置区域，包含阈值类型（个数/百分比）和阈值数值
8. WHEN 用户提交表单时 THEN THE Task_Form SHALL 将 task_type、record_type、pool_id、switch_back_policy、fail_threshold_type、fail_threshold_value 包含在请求数据中
9. WHEN 编辑模式加载任务数据时 THEN THE Task_Form SHALL 正确回填所有新增字段的值

### 需求 4：任务列表增强

**用户故事：** 作为系统管理员，我希望在任务列表中看到任务类型、记录类型和切换状态等新信息，以便快速了解每个任务的配置和当前状态。

#### 验收标准

1. WHEN 任务列表加载时 THEN THE Task_List SHALL 在表格中显示"任务类型"列，展示"暂停/删除"或"切换解析"
2. WHEN 任务列表加载时 THEN THE Task_List SHALL 在表格中显示"记录类型"列，展示"A/AAAA"或"CNAME"
3. WHEN 任务已发生切换时 THEN THE Task_List SHALL 在表格中以醒目标签显示"已切换"状态
4. WHEN 任务关联了解析池时 THEN THE Task_List SHALL 在表格中显示关联的解析池名称

### 需求 5：任务详情增强

**用户故事：** 作为系统管理员，我希望在任务详情页面看到完整的高级故障转移信息，包括切换状态、CNAME信息和增强的日志筛选功能。

#### 验收标准

1. WHEN 任务详情页面加载时 THEN THE Task_Detail SHALL 在基本信息区域显示任务类型、记录类型、关联解析池和回切策略
2. WHEN 任务处于已切换状态时 THEN THE Task_Detail SHALL 显示切换状态卡片，包含原始值、当前值和切换状态标签
3. WHEN 任务为CNAME类型时 THEN THE Task_Detail SHALL 在标签页中显示"CNAME信息"标签页
4. WHEN "CNAME信息"标签页激活时 THEN THE Task_Detail SHALL 展示CNAME解析的目标IP列表、各IP健康状态、失败计数和阈值配置
5. WHEN 操作日志标签页激活时 THEN THE Task_Detail SHALL 提供操作类型筛选下拉框（暂停、删除、恢复、添加、切换）
6. WHEN 操作日志标签页激活时 THEN THE Task_Detail SHALL 提供时间范围筛选器（开始时间和结束时间）
7. WHEN 用户设置日志筛选条件时 THEN THE Task_Detail SHALL 将 operation_type、start_time、end_time 参数传递给 GET /api/tasks/:id/logs 接口

### 需求 6：导航和路由更新

**用户故事：** 作为系统管理员，我希望在侧边栏中看到"解析池"菜单项，并能通过URL直接访问解析池相关页面。

#### 验收标准

1. WHEN 应用加载时 THEN THE Sidebar SHALL 在"探测任务"菜单项下方显示"解析池"菜单项
2. WHEN 用户点击"解析池"菜单项时 THEN THE Router SHALL 导航到 /pools 路径
3. WHEN 用户访问 /pools 路径时 THEN THE Router SHALL 加载解析池列表页面组件
4. WHEN 用户访问 /pools/:id 路径时 THEN THE Router SHALL 加载解析池详情页面组件
5. WHEN 用户在解析池相关页面时 THEN THE Sidebar SHALL 高亮"解析池"菜单项
