# 设计文档：高级DNS故障转移

## 概述

本设计文档描述了高级DNS故障转移功能的技术实现方案。该功能在现有DNS健康监控系统基础上，增加了多种任务类型、解析池管理和智能故障转移能力。

### 核心功能

1. **任务类型扩展**：支持暂停/删除和切换解析两大类任务类型
2. **解析池管理**：创建和管理备用IP/域名资源池，持续监控健康状态
3. **智能故障转移**：基于健康状态和性能指标智能选择备用资源
4. **CNAME多IP探测**：解析CNAME指向的所有IP并基于阈值触发转移
5. **灵活回切策略**：支持自动回切和保持当前两种恢复策略

### 设计原则

- **向后兼容**：保持现有暂停/删除功能不变，新功能作为扩展
- **模块化设计**：解析池管理、资源选择、故障转移逻辑独立模块
- **可扩展性**：支持未来添加更多任务类型和选择策略
- **性能优先**：并发探测、内存缓存、智能调度

## 架构设计

### 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                         Web API 层                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  任务管理API  │  │  解析池API   │  │  状态查询API  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                        业务逻辑层                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  任务调度器   │  │  解析池管理器 │  │  资源选择器   │      │
│  │  (Scheduler)  │  │ (PoolManager) │  │  (Selector)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │  故障转移器   │  │  CNAME解析器  │                        │
│  │  (Failover)   │  │ (CNAMEResolver)│                       │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                        基础设施层                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  探测器       │  │  DNS服务商    │  │  数据库       │      │
│  │  (Prober)     │  │  (Provider)   │  │  (Database)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### 模块职责


**任务调度器 (Scheduler)**
- 管理所有探测任务的生命周期
- 按配置的间隔调度探测
- 评估探测结果并触发故障转移
- 扩展支持新的任务类型（切换解析）

**解析池管理器 (PoolManager)**
- 管理解析池的CRUD操作
- 调度解析池中资源的健康探测
- 维护资源的健康状态和性能指标
- 提供资源查询和筛选接口

**资源选择器 (Selector)**
- 从解析池中选择最优备用资源
- 基于健康状态、延迟、轮询策略选择
- 支持可插拔的选择策略

**故障转移器 (Failover)**
- 执行DNS记录的切换操作
- 管理回切策略（自动/保持）
- 记录操作日志和审计信息

**CNAME解析器 (CNAMEResolver)**
- 解析CNAME记录指向的所有IP地址
- 监控IP列表变化并更新探测目标
- 计算失败IP数量和阈值

## 组件和接口

### 1. 数据模型扩展

#### 任务类型枚举

```go
// TaskType 任务类型
type TaskType string

const (
    // TaskTypePauseDelete 暂停/删除类型（兼容现有功能）
    TaskTypePauseDelete TaskType = "pause_delete"
    // TaskTypeSwitch 切换解析类型（新功能）
    TaskTypeSwitch TaskType = "switch"
)

// RecordType 解析记录类型
type RecordType string

const (
    // RecordTypeA A记录
    RecordTypeA RecordType = "A"
    // RecordTypeAAAA AAAA记录
    RecordTypeAAAA RecordType = "AAAA"
    // RecordTypeCNAME CNAME记录
    RecordTypeCNAME RecordType = "CNAME"
)

// SwitchBackPolicy 回切策略
type SwitchBackPolicy string

const (
    // SwitchBackAuto 自动回切
    SwitchBackAuto SwitchBackPolicy = "auto"
    // SwitchBackManual 保持当前（手动回切）
    SwitchBackManual SwitchBackPolicy = "manual"
)
```

#### ProbeTask 模型扩展

```go
// ProbeTask 探测任务（扩展）
type ProbeTask struct {
    ID               uint   `gorm:"primaryKey"`
    CredentialID     uint   `gorm:"not null"`
    Domain           string `gorm:"not null"`
    SubDomain        string `gorm:"not null"`
    
    // 新增字段
    TaskType         string `gorm:"not null;default:'pause_delete'"` // 任务类型
    RecordType       string `gorm:"not null;default:'A'"`            // 解析记录类型
    PoolID           *uint  `gorm:"index"`                           // 关联的解析池ID（可选）
    SwitchBackPolicy string `gorm:"default:'auto'"`                  // 回切策略
    
    // CNAME专用字段
    FailThresholdType  string `gorm:"default:'count'"`  // 阈值类型: count/percent
    FailThresholdValue int    `gorm:"default:1"`        // 阈值数值
    
    // 原有字段
    ProbeProtocol    string `gorm:"not null"`
    ProbePort        int
    ProbeIntervalSec int  `gorm:"not null"`
    TimeoutMs        int  `gorm:"not null"`
    FailThreshold    int  `gorm:"not null"`
    RecoverThreshold int  `gorm:"not null"`
    Enabled          bool `gorm:"not null;default:true"`
    
    // 切换状态跟踪
    OriginalValue    string // 原始解析值（用于回切）
    CurrentValue     string // 当前解析值
    IsSwitched       bool   // 是否已切换
    
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```


#### ResolutionPool 解析池模型

```go
// ResolutionPool 解析池
type ResolutionPool struct {
    ID          uint   `gorm:"primaryKey"`
    Name        string `gorm:"uniqueIndex;not null"` // 池名称
    ResourceType string `gorm:"not null"`             // 资源类型: ip/domain
    Description string                                // 描述
    
    // 探测配置
    ProbeProtocol    string `gorm:"not null"`
    ProbePort        int
    ProbeIntervalSec int `gorm:"not null"`
    TimeoutMs        int `gorm:"not null"`
    FailThreshold    int `gorm:"not null"`
    RecoverThreshold int `gorm:"not null"`
    
    CreatedAt time.Time
    UpdatedAt time.Time
}

// PoolResource 解析池中的资源
type PoolResource struct {
    ID       uint   `gorm:"primaryKey"`
    PoolID   uint   `gorm:"index;not null"`
    Value    string `gorm:"not null"` // IP地址或域名
    
    // 健康状态
    HealthStatus         string `gorm:"not null;default:'unknown'"` // healthy/unhealthy/unknown
    ConsecutiveFails     int    `gorm:"default:0"`
    ConsecutiveSuccesses int    `gorm:"default:0"`
    
    // 性能指标
    AvgLatencyMs int       `gorm:"default:0"` // 平均延迟（最近10次）
    LastProbeAt  time.Time                    // 最后探测时间
    
    CreatedAt time.Time
    UpdatedAt time.Time
}

// PoolProbeResult 解析池资源探测结果
type PoolProbeResult struct {
    ID         uint   `gorm:"primaryKey"`
    ResourceID uint   `gorm:"index;not null"`
    Success    bool   `gorm:"not null"`
    LatencyMs  int
    ErrorMsg   string
    ProbedAt   time.Time `gorm:"index"`
}
```

#### CNAMETarget CNAME解析目标

```go
// CNAMETarget CNAME记录解析出的IP目标
type CNAMETarget struct {
    ID       uint   `gorm:"primaryKey"`
    TaskID   uint   `gorm:"index;not null"`
    IP       string `gorm:"not null"`
    
    // 健康状态
    HealthStatus         string `gorm:"not null;default:'unknown'"`
    ConsecutiveFails     int    `gorm:"default:0"`
    ConsecutiveSuccesses int    `gorm:"default:0"`
    
    LastProbeAt time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 2. 解析池管理器接口

```go
// PoolManager 解析池管理器接口
type PoolManager interface {
    // CreatePool 创建解析池
    CreatePool(ctx context.Context, pool ResolutionPool) (uint, error)
    
    // DeletePool 删除解析池（检查引用）
    DeletePool(ctx context.Context, poolID uint) error
    
    // AddResource 向解析池添加资源
    AddResource(ctx context.Context, poolID uint, value string) error
    
    // RemoveResource 从解析池移除资源
    RemoveResource(ctx context.Context, resourceID uint) error
    
    // GetPoolResources 获取解析池中的所有资源及健康状态
    GetPoolResources(ctx context.Context, poolID uint) ([]PoolResource, error)
    
    // StartProbing 启动解析池的探测
    StartProbing(ctx context.Context, poolID uint) error
    
    // StopProbing 停止解析池的探测
    StopProbing(ctx context.Context, poolID uint) error
}
```

### 3. 资源选择器接口

```go
// ResourceSelector 资源选择器接口
type ResourceSelector interface {
    // SelectBestResource 从解析池中选择最优资源
    // 返回资源值，如果没有健康资源则返回错误
    SelectBestResource(ctx context.Context, poolID uint) (string, error)
}

// SelectionStrategy 选择策略
type SelectionStrategy interface {
    // Select 从健康资源列表中选择一个
    Select(resources []PoolResource) (*PoolResource, error)
}

// LowestLatencyStrategy 最低延迟优先策略
type LowestLatencyStrategy struct{}

// RoundRobinStrategy 轮询策略
type RoundRobinStrategy struct {
    lastIndex int
    mu        sync.Mutex
}
```

### 4. 故障转移器接口

```go
// FailoverExecutor 故障转移执行器接口
type FailoverExecutor interface {
    // SwitchToBackup 切换到备用资源
    SwitchToBackup(ctx context.Context, task *ProbeTask, backupValue string) error
    
    // SwitchBack 切换回原始资源
    SwitchBack(ctx context.Context, task *ProbeTask) error
    
    // ShouldSwitchBack 判断是否应该回切
    ShouldSwitchBack(task *ProbeTask) bool
}
```

### 5. CNAME解析器接口

```go
// CNAMEResolver CNAME解析器接口
type CNAMEResolver interface {
    // ResolveIPs 解析CNAME记录指向的所有IP
    ResolveIPs(ctx context.Context, domain string) ([]string, error)
    
    // UpdateTargets 更新任务的CNAME目标列表
    UpdateTargets(ctx context.Context, taskID uint, ips []string) error
    
    // GetFailedIPCount 获取失败IP数量
    GetFailedIPCount(ctx context.Context, taskID uint) (int, error)
    
    // CalculateThreshold 计算实际失败阈值
    CalculateThreshold(task *ProbeTask, totalIPs int) int
}
```


### 6. DNS Provider 接口扩展

```go
// DNSProvider 接口扩展（在现有基础上添加）
type DNSProvider interface {
    // 现有方法...
    
    // UpdateRecordValue 更新解析记录的值（用于切换）
    // recordID: 记录ID
    // newValue: 新的IP或域名
    UpdateRecordValue(ctx context.Context, recordID, newValue string) error
    
    // GetRecordValue 获取解析记录的当前值
    GetRecordValue(ctx context.Context, recordID string) (string, error)
}
```

## 数据模型

### 实体关系图

```
┌─────────────────┐
│   ProbeTask     │
│─────────────────│
│ ID              │
│ TaskType        │◄──────┐
│ RecordType      │       │
│ PoolID          │───┐   │
│ SwitchBackPolicy│   │   │
│ OriginalValue   │   │   │
│ CurrentValue    │   │   │
│ IsSwitched      │   │   │
└─────────────────┘   │   │
                      │   │
                      │   │
┌─────────────────┐   │   │
│ ResolutionPool  │◄──┘   │
│─────────────────│       │
│ ID              │       │
│ Name            │       │
│ ResourceType    │       │
│ ProbeConfig     │       │
└─────────────────┘       │
        │                 │
        │ 1:N             │
        ▼                 │
┌─────────────────┐       │
│  PoolResource   │       │
│─────────────────│       │
│ ID              │       │
│ PoolID          │       │
│ Value           │       │
│ HealthStatus    │       │
│ AvgLatencyMs    │       │
└─────────────────┘       │
        │                 │
        │ 1:N             │
        ▼                 │
┌─────────────────┐       │
│PoolProbeResult  │       │
│─────────────────│       │
│ ResourceID      │       │
│ Success         │       │
│ LatencyMs       │       │
└─────────────────┘       │
                          │
                          │ 1:N
                          ▼
                  ┌─────────────────┐
                  │  CNAMETarget    │
                  │─────────────────│
                  │ TaskID          │
                  │ IP              │
                  │ HealthStatus    │
                  └─────────────────┘
```

### 数据库迁移策略

1. **向后兼容**：为 `ProbeTask` 添加新字段时使用默认值
2. **渐进式迁移**：现有任务自动标记为 `pause_delete` 类型
3. **可选关联**：`PoolID` 为可空字段，仅切换类型任务需要

## 正确性属性

*属性是一个特征或行为，应该在系统的所有有效执行中保持为真——本质上是关于系统应该做什么的形式化陈述。属性作为人类可读规范和机器可验证正确性保证之间的桥梁。*

### 属性反思

在编写正确性属性之前，我需要识别并消除冗余：

**冗余分析：**

1. **资源选择相关**：需求4.2、5.2、9.1都要求"优先选择健康状态的资源"，这些可以合并为一个通用属性
2. **日志记录相关**：需求2.4、3.7、4.6、5.6、10.1都要求记录操作日志，可以合并为一个通用属性
3. **API调用相关**：需求2.3、12.1-12.4都是关于调用DNS Provider API，可以合并
4. **探测结果记录**：需求8.3、8.4、10.4都是关于记录探测结果，可以合并
5. **阈值触发**：需求2.1、4.1的失败触发逻辑相似，可以合并为一个通用属性
6. **恢复逻辑**：需求2.2的恢复逻辑已被现有系统覆盖，不需要重复
7. **回切策略**：需求4.4、4.5、5.4、5.5的回切逻辑可以合并为一个条件属性

**保留的核心属性：**
- CNAME多IP解析和动态更新（3.1、3.2）
- 基于阈值的CNAME故障转移（3.3、7.2-7.5）
- 资源健康状态管理（6.5、6.6）
- 智能资源选择（9.2、9.3、9.5）
- 解析池引用检查（6.8）
- 配置验证（6.2、11.3、11.4）
- 探测生命周期管理（6.4、6.9、8.5、11.7）

### 核心属性

**属性 1：CNAME多IP解析**
*对于任意* CNAME类型的探测任务，当系统解析该CNAME记录时，应该返回该CNAME指向的所有IP地址列表
**验证需求：3.1**

**属性 2：CNAME目标动态更新**
*对于任意* CNAME类型的探测任务，当CNAME记录指向的IP列表发生变化时，系统应该在下一个探测周期内更新CNAMETarget表中的IP列表
**验证需求：3.2**

**属性 3：百分比阈值计算正确性**
*对于任意* 使用百分比作为阈值单位的CNAME任务，计算出的实际失败个数阈值应该等于 `ceil(总IP数 * 百分比 / 100)` 且至少为1
**验证需求：7.2, 7.4**

**属性 4：阈值动态重算**
*对于任意* 使用百分比阈值的CNAME任务，当解析出的IP总数从N变为M时，系统应该重新计算阈值从 `ceil(N * 百分比 / 100)` 变为 `ceil(M * 百分比 / 100)`
**验证需求：7.3**

**属性 5：CNAME故障转移触发**
*对于任意* CNAME类型的切换任务，当失败IP数量达到或超过配置的阈值时，系统应该从关联的解析池中选择一个健康的域名并更新CNAME记录值
**验证需求：3.3, 5.1, 7.5**

**属性 6：资源健康状态转换**
*对于任意* 解析池中的资源，当连续探测失败次数达到失败阈值时，该资源的健康状态应该从healthy变为unhealthy；当连续成功次数达到恢复阈值时，应该从unhealthy变为healthy
**验证需求：6.5, 6.6**

**属性 7：资源选择仅限健康资源**
*对于任意* 解析池，当系统从中选择备用资源时，返回的资源必须是健康状态（HealthStatus = 'healthy'）
**验证需求：4.2, 5.2, 9.1**

**属性 8：最低延迟优先选择**
*对于任意* 包含多个健康资源的解析池，当使用最低延迟策略选择时，返回的资源应该是所有健康资源中AvgLatencyMs最小的
**验证需求：9.2**

**属性 9：轮询策略负载均衡**
*对于任意* 包含N个延迟相近的健康资源的解析池，连续N次使用轮询策略选择时，每个资源应该被选中恰好一次
**验证需求：9.3**

**属性 10：平均延迟计算窗口**
*对于任意* 解析池资源，其AvgLatencyMs应该等于该资源最近10次成功探测的延迟平均值（如果成功次数少于10次，则使用所有成功探测的平均值）
**验证需求：9.5**

**属性 11：解析池引用检查**
*对于任意* 解析池，如果存在至少一个ProbeTask的PoolID字段引用该池，则删除该池的操作应该失败并返回错误
**验证需求：6.8**

**属性 12：资源格式验证**
*对于任意* 解析池，当添加资源时，如果资源类型为ip则值必须是有效的IPv4或IPv6地址，如果资源类型为domain则值必须是有效的域名格式，否则应该拒绝添加
**验证需求：6.2**

**属性 13：池类型匹配验证**
*对于任意* 切换类型任务，当修改其关联的解析池时，如果新池的ResourceType与任务的RecordType不匹配（A/AAAA需要ip池，CNAME需要domain池），则应该拒绝修改
**验证需求：11.4**

**属性 14：添加资源自动启动探测**
*对于任意* 解析池资源，当该资源被添加到池中后，系统应该在下一个探测周期内开始对该资源进行健康探测
**验证需求：6.4**

**属性 15：移除资源停止探测**
*对于任意* 解析池资源，当该资源从池中移除后，系统应该停止对该资源的所有探测活动，且不再记录该资源的探测结果
**验证需求：6.9**

**属性 16：任务禁用停止探测**
*对于任意* 探测任务，当该任务的Enabled字段从true变为false时，系统应该停止对该任务目标的所有探测活动
**验证需求：8.5**

**属性 17：任务删除清理探测**
*对于任意* 探测任务，当该任务被删除后，系统应该停止该任务的所有探测活动，并清理相关的CNAMETarget记录
**验证需求：11.7**

**属性 18：回切策略遵守**
*对于任意* 切换类型任务，当原始资源恢复健康时：如果SwitchBackPolicy为'auto'，系统应该将记录切换回OriginalValue；如果为'manual'，系统应该保持CurrentValue不变
**验证需求：4.4, 4.5, 5.4, 5.5**

**属性 19：操作日志完整性**
*对于任意* 故障转移操作（暂停、删除、切换、恢复），系统应该在OperationLog表中记录一条日志，包含TaskID、OperationType、IP/域名、时间戳和操作结果
**验证需求：2.4, 3.7, 4.6, 5.6, 10.1**

**属性 20：探测结果记录完整性**
*对于任意* 探测操作（任务探测或池资源探测），系统应该记录探测结果，包含目标标识、成功状态、延迟（成功时）或错误信息（失败时）和时间戳
**验证需求：8.3, 8.4, 10.4**

**属性 21：DNS Provider API调用正确性**
*对于任意* 故障转移操作，系统应该调用DNS Provider的相应API方法：暂停调用PauseRecord，删除调用DeleteRecord，切换调用UpdateRecordValue，恢复调用ResumeRecord或AddRecord
**验证需求：2.3, 12.1, 12.2, 12.3, 12.4**

**属性 22：API失败重试**
*对于任意* DNS Provider API调用失败，系统应该记录错误日志，并在下一个探测周期重新尝试该操作
**验证需求：12.5**

**属性 23：限流退避策略**
*对于任意* DNS Provider API返回限流错误（如HTTP 429），系统应该实施指数退避重试策略，重试间隔应该递增
**验证需求：12.6**


## 错误处理

### 错误类型

1. **验证错误**
   - 无效的任务配置（缺少必填字段、无效的枚举值）
   - 资源格式错误（无效的IP地址或域名）
   - 类型不匹配（池类型与任务类型不匹配）

2. **业务逻辑错误**
   - 解析池被引用时尝试删除
   - 切换类型任务未关联解析池
   - 解析池中没有健康资源

3. **外部依赖错误**
   - DNS Provider API调用失败
   - DNS Provider API限流
   - CNAME解析失败
   - 数据库操作失败

4. **并发错误**
   - 资源竞争（多个任务同时修改同一记录）
   - 死锁（循环依赖）

### 错误处理策略

**验证错误**
- 在API层立即返回400错误
- 提供清晰的错误消息
- 不记录到错误日志（属于正常的用户输入错误）

**业务逻辑错误**
- 返回适当的HTTP状态码（400/409）
- 记录警告日志
- 提供详细的错误原因

**外部依赖错误**
- 记录详细的错误日志
- 实施重试机制（指数退避）
- 对于DNS Provider限流，使用退避策略
- 对于CNAME解析失败，保持现有目标列表不变

**并发错误**
- 使用数据库事务保证原子性
- 使用乐观锁或悲观锁防止竞争
- 记录错误日志并返回503状态码

### 降级策略

1. **解析池无健康资源**
   - 保持当前DNS记录不变
   - 记录错误日志
   - 发送告警通知（未来扩展）

2. **CNAME解析失败**
   - 继续使用上一次成功解析的IP列表
   - 记录警告日志
   - 在下一个周期重试解析

3. **DNS Provider API持续失败**
   - 累计失败次数超过阈值后暂停任务
   - 记录严重错误日志
   - 需要人工介入恢复

## 测试策略

### 双重测试方法

本功能采用单元测试和基于属性的测试相结合的方法：

- **单元测试**：验证特定示例、边界情况和错误条件
- **属性测试**：验证跨所有输入的通用属性
- 两者互补，共同确保全面覆盖

### 单元测试重点

单元测试应该专注于：
- 特定示例（如创建任务时验证必填字段）
- 集成点（如API与调度器的交互）
- 边界情况（如解析池无健康资源）
- 错误条件（如DNS Provider API失败）

避免编写过多单元测试，属性测试已经覆盖了大量输入组合。

### 属性测试配置

**测试库选择**：使用 `gopter` 库进行基于属性的测试（Go语言的QuickCheck实现）

**测试配置**：
- 每个属性测试最少运行100次迭代
- 使用随机生成器生成测试数据
- 每个测试必须引用设计文档中的属性编号

**标签格式**：
```go
// Feature: advanced-dns-failover, Property 3: 百分比阈值计算正确性
// 对于任意使用百分比作为阈值单位的CNAME任务，计算出的实际失败个数阈值
// 应该等于 ceil(总IP数 * 百分比 / 100) 且至少为1
func TestProperty_ThresholdCalculation(t *testing.T) {
    // 属性测试实现
}
```

### 测试覆盖范围

**必须测试的属性**：
- 属性1-23：所有核心正确性属性

**单元测试覆盖**：
- API输入验证
- 错误处理路径
- 边界情况（空列表、单个资源、全部失败）
- DNS Provider集成（使用mock）

**集成测试**：
- 完整的故障转移流程
- 多任务并发执行
- 系统重启后恢复

### 测试数据生成

**随机生成器**：
- IP地址生成器（IPv4和IPv6）
- 域名生成器（有效的DNS域名格式）
- 任务配置生成器（随机但有效的配置）
- 探测结果生成器（成功/失败、延迟范围）

**约束条件**：
- 阈值范围：1-100
- 延迟范围：1-5000ms
- IP数量：1-100
- 百分比：1-100

## 实施计划

### 阶段1：数据模型和基础设施（优先级：高）

1. 数据库迁移脚本
2. 扩展ProbeTask模型
3. 创建ResolutionPool、PoolResource、CNAMETarget表
4. 更新API模型和验证逻辑

### 阶段2：解析池管理（优先级：高）

1. 实现PoolManager接口
2. 解析池CRUD API
3. 资源添加/移除API
4. 解析池探测调度器

### 阶段3：资源选择器（优先级：中）

1. 实现ResourceSelector接口
2. 最低延迟策略
3. 轮询策略
4. 平均延迟计算逻辑

### 阶段4：CNAME解析器（优先级：高）

1. 实现CNAMEResolver接口
2. CNAME记录解析逻辑
3. IP列表动态更新
4. 阈值计算（个数/百分比）

### 阶段5：故障转移器（优先级：高）

1. 实现FailoverExecutor接口
2. 切换到备用资源逻辑
3. 回切策略实现
4. DNS Provider API调用

### 阶段6：任务调度器扩展（优先级：高）

1. 扩展Scheduler支持新任务类型
2. 集成CNAME解析器
3. 集成故障转移器
4. 集成资源选择器

### 阶段7：API层扩展（优先级：中）

1. 任务创建/更新API扩展
2. 解析池管理API
3. 状态查询API扩展
4. 操作日志查询API

### 阶段8：测试（优先级：高）

1. 属性测试实现（23个属性）
2. 单元测试（边界情况和错误处理）
3. 集成测试
4. 性能测试

### 阶段9：文档和部署（优先级：低）

1. API文档更新
2. 用户手册
3. 部署指南
4. 监控和告警配置

## 性能考虑

### 并发探测

- 使用goroutine池并发探测多个目标
- 限制并发数量避免资源耗尽
- 使用context控制超时和取消

### 缓存策略

- 内存缓存资源健康状态（减少数据库查询）
- 缓存CNAME解析结果（TTL内复用）
- 缓存DNS Provider客户端实例

### 数据库优化

- 为常用查询字段添加索引
- 使用批量插入减少数据库往返
- 定期清理过期的探测结果和日志

### 调度优化

- 使用时间轮算法优化定时任务调度
- 合并相同间隔的探测任务
- 动态调整探测频率（基于负载）

## 安全考虑

### 输入验证

- 严格验证所有用户输入
- 防止SQL注入（使用参数化查询）
- 防止DNS重绑定攻击

### 权限控制

- 解析池操作需要认证
- 任务操作需要认证
- 敏感操作记录审计日志

### 资源限制

- 限制单个解析池的资源数量
- 限制单个用户的任务数量
- 限制并发探测数量

## 监控和可观测性

### 关键指标

- 探测成功率
- 故障转移次数
- 平均探测延迟
- API调用失败率
- 解析池健康资源比例

### 日志记录

- 结构化日志（JSON格式）
- 日志级别：DEBUG/INFO/WARN/ERROR
- 关键操作必须记录日志

### 告警规则

- 解析池无健康资源
- DNS Provider API持续失败
- 探测延迟异常升高
- 数据库连接失败

## 未来扩展

### 短期扩展（3-6个月）

1. **多DNS服务商支持**：腾讯云、AWS Route53
2. **告警通知**：邮件、Webhook、钉钉
3. **可视化仪表板**：实时健康状态、历史趋势图
4. **批量操作**：批量创建任务、批量添加资源

### 长期扩展（6-12个月）

1. **智能调度**：基于历史数据预测故障
2. **多区域支持**：跨区域故障转移
3. **自定义脚本**：用户自定义探测和转移逻辑
4. **API限流和配额**：防止滥用

## 总结

本设计文档详细描述了高级DNS故障转移功能的技术实现方案。通过模块化设计、清晰的接口定义和全面的测试策略，确保功能的可靠性、可维护性和可扩展性。

核心设计原则：
- **向后兼容**：不影响现有功能
- **模块化**：清晰的职责划分
- **可测试**：23个正确性属性确保质量
- **高性能**：并发探测、缓存优化
- **可扩展**：支持未来功能扩展
