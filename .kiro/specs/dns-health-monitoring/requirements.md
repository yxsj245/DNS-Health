# 需求文档

## 简介

DNS健康监控功能是一个专注于DNS解析监控和通知的系统功能。与现有的探测任务不同,健康监控任务只记录和监控DNS解析的健康状态,不执行任何故障转移操作。该功能复用现有的DNS解析器、探测器和通知系统,为用户提供轻量级的监控能力。

## 术语表

- **System**: DNS健康监控系统
- **Health_Monitor**: 健康监控器,负责执行DNS解析和健康探测
- **Notification_Service**: 通知服务,负责发送告警通知
- **DNS_Resolver**: DNS解析器,负责解析域名到IP地址
- **Prober**: 探测器,负责对IP地址执行健康检测
- **Monitoring_Task**: 健康监控任务,用户创建的监控配置
- **Threshold**: 阈值,触发通知的条件值
- **Health_Status**: 健康状态,包括健康(healthy)、不健康(unhealthy)、未知(unknown)

## 需求

### 需求 1: 创建健康监控任务

**用户故事:** 作为系统用户,我想创建健康监控任务,以便监控特定域名的DNS解析和健康状态

#### 验收标准

1. WHEN 用户提交创建监控任务请求, THE System SHALL 验证所有必填字段是否完整
2. WHEN 用户选择记录类型, THE System SHALL 支持 A、AAAA、A_AAAA、CNAME 四种记录类型
3. WHEN 用户输入域名和子域名, THE System SHALL 验证域名格式的有效性
4. WHEN 用户配置探测参数, THE System SHALL 支持 ICMP、TCP、UDP、HTTP、HTTPS 五种探测协议
5. WHEN 用户设置探测间隔, THE System SHALL 接受大于0的整数值(单位:秒)
6. WHEN 用户设置超时时间, THE System SHALL 接受大于0的整数值(单位:毫秒)
7. WHEN 用户设置失败阈值, THE System SHALL 接受大于0的整数值
8. WHEN 用户设置恢复阈值, THE System SHALL 接受大于0的整数值
9. WHEN 所有验证通过, THE System SHALL 创建监控任务并返回任务ID
10. WHEN 创建任务成功, THE System SHALL 自动启动该监控任务

### 需求 2: DNS解析和IP发现

**用户故事:** 作为健康监控器,我想解析域名获取IP地址列表,以便对这些IP执行健康探测

#### 验收标准

1. WHEN 监控任务启动, THE DNS_Resolver SHALL 根据记录类型解析域名
2. WHEN 记录类型为A, THE DNS_Resolver SHALL 返回所有IPv4地址
3. WHEN 记录类型为AAAA, THE DNS_Resolver SHALL 返回所有IPv6地址
4. WHEN 记录类型为A_AAAA, THE DNS_Resolver SHALL 返回所有IPv4和IPv6地址
5. WHEN 记录类型为CNAME, THE DNS_Resolver SHALL 先解析CNAME记录,再解析CNAME指向的IP地址
6. WHEN DNS解析失败, THE System SHALL 记录错误信息并在下个周期重试
7. WHEN 解析到新的IP地址, THE System SHALL 将其加入监控列表
8. WHEN 某个IP地址不再出现在解析结果中, THE System SHALL 停止对其监控

### 需求 3: 健康探测执行

**用户故事:** 作为健康监控器,我想定期对解析出的IP地址执行健康探测,以便了解每个IP的可用性和性能

#### 验收标准

1. WHEN 探测周期到达, THE Prober SHALL 对所有监控中的IP地址执行探测
2. WHEN 探测协议为ICMP, THE Prober SHALL 发送ICMP Echo请求并等待响应
3. WHEN 探测协议为TCP, THE Prober SHALL 尝试建立TCP连接到指定端口
4. WHEN 探测协议为UDP, THE Prober SHALL 发送UDP数据包到指定端口
5. WHEN 探测协议为HTTP, THE Prober SHALL 发送HTTP GET请求
6. WHEN 探测协议为HTTPS, THE Prober SHALL 发送HTTPS GET请求
7. WHEN 探测成功, THE System SHALL 记录成功状态和响应延迟
8. WHEN 探测失败, THE System SHALL 记录失败状态和错误信息
9. WHEN 探测超时, THE System SHALL 将其视为失败并记录超时错误
10. THE System SHALL 在每次探测后更新IP的连续成功次数和连续失败次数

### 需求 4: 健康状态管理

**用户故事:** 作为健康监控器,我想根据探测结果维护每个IP的健康状态,以便准确反映当前的可用性

#### 验收标准

1. WHEN IP的连续失败次数达到失败阈值, THE System SHALL 将该IP标记为不健康状态
2. WHEN IP的连续成功次数达到恢复阈值, THE System SHALL 将该IP标记为健康状态
3. WHEN IP首次加入监控, THE System SHALL 将其初始状态设置为未知
4. WHEN IP状态从健康变为不健康, THE System SHALL 记录状态变更事件
5. WHEN IP状态从不健康变为健康, THE System SHALL 记录状态恢复事件
6. THE System SHALL 为每个IP维护最近的平均延迟(基于最近10次成功探测)

### 需求 5: 探测结果存储

**用户故事:** 作为系统管理员,我想查看历史探测结果,以便分析监控对象的健康趋势

#### 验收标准

1. WHEN 探测完成, THE System SHALL 将探测结果存储到数据库
2. WHEN 存储探测结果, THE System SHALL 记录任务ID、IP地址、成功状态、延迟和时间戳
3. WHEN 探测失败, THE System SHALL 同时记录错误信息
4. THE System SHALL 保留所有历史探测记录用于查询和分析
5. WHEN 用户查询探测结果, THE System SHALL 支持按任务ID和时间范围过滤

### 需求 6: 通知触发

**用户故事:** 作为系统用户,我想在监控指标达到阈值时收到通知,以便及时了解健康状态变化

#### 验收标准

1. WHEN IP状态变为不健康且通知已启用, THE Notification_Service SHALL 发送故障通知
2. WHEN IP状态恢复为健康且通知已启用, THE Notification_Service SHALL 发送恢复通知
3. WHEN IP连续失败达到阈值且通知已启用, THE Notification_Service SHALL 发送连续失败告警
4. WHEN 通知未启用, THE System SHALL 不发送任何通知
5. WHEN 发送通知, THE System SHALL 包含任务名称、域名、IP地址、状态和时间信息
6. WHEN 通知发送完成, THE System SHALL 记录通知日志(包括成功或失败状态)

### 需求 7: 任务管理

**用户故事:** 作为系统用户,我想管理健康监控任务,以便控制监控的启停和配置

#### 验收标准

1. WHEN 用户请求暂停任务, THE System SHALL 停止该任务的DNS解析和探测
2. WHEN 用户请求恢复任务, THE System SHALL 重新启动该任务的DNS解析和探测
3. WHEN 用户请求删除任务, THE System SHALL 停止任务并删除所有相关数据
4. WHEN 用户请求更新任务配置, THE System SHALL 验证新配置并应用更改
5. WHEN 更新任务配置, THE System SHALL 使用新配置重新启动监控
6. WHEN 用户查询任务列表, THE System SHALL 返回所有监控任务及其当前状态
7. WHEN 用户查询单个任务详情, THE System SHALL 返回任务配置和所有监控IP的健康状态

### 需求 8: CNAME记录特殊处理

**用户故事:** 作为健康监控器,我想正确处理CNAME记录类型,以便监控CNAME指向的实际IP地址

#### 验收标准

1. WHEN 记录类型为CNAME, THE DNS_Resolver SHALL 首先查询CNAME记录值
2. WHEN 获取CNAME记录值后, THE DNS_Resolver SHALL 继续解析CNAME指向的A或AAAA记录
3. WHEN CNAME解析出多个IP, THE System SHALL 对每个IP独立执行健康探测
4. WHEN 存储CNAME相关数据, THE System SHALL 记录CNAME值和对应的IP地址映射关系
5. WHEN CNAME记录值发生变化, THE System SHALL 更新监控的IP列表
6. WHEN 配置失败阈值类型为个数, THE System SHALL 按失败IP个数判断是否触发通知
7. WHEN 配置失败阈值类型为百分比, THE System SHALL 按失败IP占比判断是否触发通知

### 需求 9: 数据隔离和安全

**用户故事:** 作为系统架构师,我想确保健康监控功能与故障转移功能数据隔离,以便两者互不干扰

#### 验收标准

1. THE System SHALL 使用独立的数据表存储健康监控任务
2. THE System SHALL 不修改或依赖探测任务(ProbeTask)的故障转移相关字段
3. THE System SHALL 不触发任何DNS记录的暂停、删除或切换操作
4. THE System SHALL 复用现有的DNS解析器和探测器代码
5. THE System SHALL 复用现有的通知服务和配置

### 需求 10: API接口

**用户故事:** 作为前端开发者,我想通过API管理健康监控任务,以便在用户界面中提供监控功能

#### 验收标准

1. THE System SHALL 提供创建健康监控任务的API接口
2. THE System SHALL 提供查询健康监控任务列表的API接口
3. THE System SHALL 提供查询单个健康监控任务详情的API接口
4. THE System SHALL 提供更新健康监控任务配置的API接口
5. THE System SHALL 提供暂停健康监控任务的API接口
6. THE System SHALL 提供恢复健康监控任务的API接口
7. THE System SHALL 提供删除健康监控任务的API接口
8. THE System SHALL 提供查询探测结果历史的API接口
9. WHEN API请求失败, THE System SHALL 返回明确的错误代码和错误信息
10. THE System SHALL 对所有API接口进行身份验证


### 需求 11: 前端用户界面

**用户故事:** 作为系统用户,我想通过Web界面管理健康监控任务,以便方便地创建、查看和管理监控配置

#### 验收标准

1. THE System SHALL 提供健康监控任务列表页面,页面结构与探测任务列表页面完全一致
2. WHEN 用户访问任务列表页面, THE System SHALL 显示所有监控任务及其状态(运行中/已停止)
3. THE System SHALL 在任务列表中显示域名、记录类型、探测协议、探测周期、超时时间和健康状态
4. THE System SHALL 在任务列表中提供查看详情、编辑、暂停/恢复、删除操作按钮
5. THE System SHALL 提供创建健康监控任务的表单页面,使用与探测任务相同的分组卡片布局
6. WHEN 用户填写创建表单, THE System SHALL 实时验证输入字段的有效性
7. THE System SHALL 将表单分为"域名配置"和"探测配置"两个卡片分组
8. THE System SHALL 提供任务详情页面,使用与探测任务相同的标签页布局
9. THE System SHALL 在详情页提供"基本信息"、"探测历史"和"监控目标"三个标签页
10. WHEN 用户查看探测历史, THE System SHALL 支持按IP和状态筛选,并提供分页功能
11. WHEN 用户查看监控目标, THE System SHALL 显示每个IP的健康状态、连续失败/成功次数、平均延迟和最后探测时间
12. WHEN 用户执行操作, THE System SHALL 显示操作确认对话框
13. WHEN 操作成功或失败, THE System SHALL 显示相应的提示消息
14. THE System SHALL 使用Element Plus组件库构建用户界面
15. THE System SHALL 复用探测任务页面的样式和布局结构
