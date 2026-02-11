# 需求文档：延迟曲线图表

## 简介

在探测任务详情页和健康监控详情页的最上方添加延迟曲线图表功能，用于可视化展示每个 IP 的延迟变化趋势。图表支持按日期范围筛选、按 IP 筛选，并以检测周期为粒度显示延迟数据。

## 术语表

- **延迟曲线图表（Latency_Chart）**：以折线图形式展示 IP 延迟随时间变化的图表组件
- **探测任务详情页（Task_Detail_Page）**：展示探测任务（ProbeTask）详细信息的前端页面
- **健康监控详情页（Health_Monitor_Detail_Page）**：展示健康监控任务（HealthMonitorTask）详细信息的前端页面
- **探测结果（Probe_Result）**：每次探测产生的结果记录，包含 IP、延迟、成功状态、探测时间
- **检测周期（Probe_Interval）**：任务配置中的探测间隔时间（秒），作为图表的时间粒度
- **延迟数据 API（Latency_Data_API）**：后端提供的延迟数据查询接口，支持按任务 ID、IP、时间范围查询

## 需求

### 需求 1：后端延迟数据查询接口

**用户故事：** 作为前端开发者，我希望有专用的延迟数据查询 API，以便获取指定时间范围内某个 IP 的延迟数据用于图表展示。

#### 验收标准

1. WHEN 前端请求探测任务的延迟数据时，THE Latency_Data_API SHALL 返回指定任务 ID、IP 地址和时间范围内的所有探测结果，包含延迟值和探测时间
2. WHEN 前端请求健康监控任务的延迟数据时，THE Latency_Data_API SHALL 返回指定任务 ID、IP 地址和时间范围内的所有探测结果，包含延迟值和探测时间
3. WHEN 请求参数中缺少必填的 IP 地址时，THE Latency_Data_API SHALL 返回 400 错误码和明确的错误信息
4. WHEN 请求参数中缺少开始时间或结束时间时，THE Latency_Data_API SHALL 使用默认时间范围（最近 24 小时）
5. WHEN 指定的任务 ID 不存在时，THE Latency_Data_API SHALL 返回 404 错误码和明确的错误信息
6. THE Latency_Data_API SHALL 按探测时间升序返回数据，以便前端直接用于图表渲染

### 需求 2：探测任务详情页延迟曲线图表

**用户故事：** 作为运维人员，我希望在探测任务详情页顶部看到延迟曲线图表，以便直观了解各 IP 的延迟变化趋势。

#### 验收标准

1. WHEN 用户打开探测任务详情页时，THE Task_Detail_Page SHALL 在页面基本信息卡片上方显示延迟曲线图表区域
2. WHEN 延迟曲线图表加载时，THE Latency_Chart SHALL 显示一个 IP 选择器和一个日期范围选择器
3. WHEN 用户选择一个 IP 地址和日期范围后，THE Latency_Chart SHALL 调用 Latency_Data_API 获取数据并以折线图形式展示延迟随时间的变化
4. WHEN 图表数据加载中时，THE Latency_Chart SHALL 显示加载状态指示
5. WHEN 查询的时间范围内无延迟数据时，THE Latency_Chart SHALL 显示空状态提示信息
6. THE Latency_Chart SHALL 在图表标题或说明区域显示当前任务的检测周期（探测间隔秒数）作为数据粒度参考
7. WHEN 用户切换 IP 选择或修改日期范围时，THE Latency_Chart SHALL 重新请求数据并更新图表展示

### 需求 3：健康监控详情页延迟曲线图表

**用户故事：** 作为运维人员，我希望在健康监控详情页顶部看到延迟曲线图表，以便直观了解各监控目标 IP 的延迟变化趋势。

#### 验收标准

1. WHEN 用户打开健康监控详情页时，THE Health_Monitor_Detail_Page SHALL 在页面基本信息卡片上方显示延迟曲线图表区域
2. WHEN 延迟曲线图表加载时，THE Latency_Chart SHALL 显示一个 IP 选择器和一个日期范围选择器
3. WHEN 用户选择一个 IP 地址和日期范围后，THE Latency_Chart SHALL 调用 Latency_Data_API 获取数据并以折线图形式展示延迟随时间的变化
4. WHEN 查询的时间范围内无延迟数据时，THE Latency_Chart SHALL 显示空状态提示信息
5. THE Latency_Chart SHALL 在图表标题或说明区域显示当前任务的检测周期（探测间隔秒数）作为数据粒度参考
6. WHEN 用户切换 IP 选择或修改日期范围时，THE Latency_Chart SHALL 重新请求数据并更新图表展示

### 需求 4：IP 选择器数据源

**用户故事：** 作为运维人员，我希望 IP 选择器能自动列出当前任务关联的所有 IP 地址，以便快速选择要查看的目标。

#### 验收标准

1. WHEN 探测任务详情页的延迟图表加载时，THE Latency_Chart SHALL 从任务的 IP 管理接口获取所有关联 IP 并填充到 IP 选择器中
2. WHEN 健康监控详情页的延迟图表加载时，THE Latency_Chart SHALL 从任务详情中的监控目标列表获取所有 IP 并填充到 IP 选择器中
3. WHEN IP 列表加载完成且列表非空时，THE Latency_Chart SHALL 自动选中第一个 IP 并加载对应的延迟数据
4. WHEN IP 列表为空时，THE Latency_Chart SHALL 显示提示信息说明暂无可选 IP

### 需求 5：日期范围选择器

**用户故事：** 作为运维人员，我希望通过日期范围选择器指定查看延迟数据的时间段，以便分析特定时间段内的延迟变化。

#### 验收标准

1. THE Latency_Chart SHALL 提供日期范围选择器，允许用户选择开始日期和结束日期
2. WHEN 延迟图表首次加载时，THE Latency_Chart SHALL 将日期范围默认设置为最近 24 小时
3. WHEN 用户选择的结束日期早于开始日期时，THE Latency_Chart SHALL 阻止该选择并保持原有日期范围
