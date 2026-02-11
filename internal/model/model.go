// Package model 定义系统数据模型
package model

import "time"

// ========== 任务类型和策略枚举 ==========

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
	// RecordTypeA_AAAA A和AAAA记录（同时监控）
	RecordTypeA_AAAA RecordType = "A_AAAA"
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

// FailThresholdType 失败阈值类型
type FailThresholdType string

const (
	// FailThresholdCount 按个数
	FailThresholdCount FailThresholdType = "count"
	// FailThresholdPercent 按百分比
	FailThresholdPercent FailThresholdType = "percent"
)

// HealthStatus 健康状态
type HealthStatus string

const (
	// HealthStatusHealthy 健康
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusUnhealthy 不健康
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusUnknown 未知
	HealthStatusUnknown HealthStatus = "unknown"
)

// IsValidTaskType 验证任务类型是否有效
func IsValidTaskType(t string) bool {
	switch TaskType(t) {
	case TaskTypePauseDelete, TaskTypeSwitch:
		return true
	default:
		return false
	}
}

// IsValidRecordType 验证解析记录类型是否有效
func IsValidRecordType(t string) bool {
	switch RecordType(t) {
	case RecordTypeA, RecordTypeAAAA, RecordTypeA_AAAA, RecordTypeCNAME:
		return true
	default:
		return false
	}
}

// IsValidSwitchBackPolicy 验证回切策略是否有效
func IsValidSwitchBackPolicy(p string) bool {
	switch SwitchBackPolicy(p) {
	case SwitchBackAuto, SwitchBackManual:
		return true
	default:
		return false
	}
}

// IsValidFailThresholdType 验证失败阈值类型是否有效
func IsValidFailThresholdType(t string) bool {
	switch FailThresholdType(t) {
	case FailThresholdCount, FailThresholdPercent:
		return true
	default:
		return false
	}
}

// User 用户
type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
}

// Credential 云服务商凭证
// CredentialsEncrypted 以 JSON 格式加密存储所有凭证字段，支持不同服务商的不同字段结构
type Credential struct {
	ID                   uint   `gorm:"primaryKey"`
	ProviderType         string `gorm:"not null"` // "aliyun" 等
	Name                 string `gorm:"not null"`
	CredentialsEncrypted string // 加密后的 JSON，包含该服务商所需的所有凭证字段（旧数据可能为空）

	// 以下两个字段保留用于兼容旧数据，新数据不再写入
	AccessKeyIDEncrypted     string
	AccessKeySecretEncrypted string

	CreatedAt time.Time
}

// ProbeTask 探测任务
type ProbeTask struct {
	ID           uint   `gorm:"primaryKey"`
	CredentialID uint   `gorm:"not null"`
	Domain       string `gorm:"not null"`
	SubDomain    string `gorm:"not null"` // "@" 表示根域名

	// 新增字段 - 任务类型和策略
	TaskType         string `gorm:"not null;default:'pause_delete'"` // 任务类型: pause_delete / switch
	RecordType       string `gorm:"not null;default:'A_AAAA'"`       // 解析记录类型: A / AAAA / A_AAAA / CNAME
	PoolID           *uint  `gorm:"index"`                           // 关联的解析池ID（可选，切换类型任务必填）
	SwitchBackPolicy string `gorm:"default:'auto'"`                  // 回切策略: auto / manual

	// CNAME专用字段 - 失败阈值配置
	FailThresholdType  string `gorm:"default:'count'"` // 阈值类型: count / percent
	FailThresholdValue int    `gorm:"default:1"`       // 阈值数值（个数或百分比）

	// 原有字段 - 探测配置
	ProbeProtocol    string `gorm:"not null"` // ICMP/TCP/UDP/HTTP/HTTPS
	ProbePort        int
	ProbeIntervalSec int  `gorm:"not null"`
	TimeoutMs        int  `gorm:"not null"`
	FailThreshold    int  `gorm:"not null"`
	RecoverThreshold int  `gorm:"not null"`
	Enabled          bool `gorm:"not null;default:true"`

	// 切换状态跟踪
	OriginalValue string // 原始解析值（用于回切）
	CurrentValue  string // 当前解析值
	IsSwitched    bool   `gorm:"default:false"` // 是否已切换到备用资源

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DeletedRecord 已删除记录缓存
type DeletedRecord struct {
	ID         uint   `gorm:"primaryKey"`
	TaskID     uint   `gorm:"index;not null"`
	Domain     string `gorm:"not null"`
	SubDomain  string `gorm:"not null"`
	RecordType string `gorm:"not null"` // "A" / "AAAA"
	IP         string `gorm:"not null"`
	TTL        int    `gorm:"not null"`
	DeletedAt  time.Time
}

// ProbeResult 探测结果
type ProbeResult struct {
	ID        uint   `gorm:"primaryKey"`
	TaskID    uint   `gorm:"index;not null"`
	IP        string `gorm:"not null"`
	Success   bool   `gorm:"not null"`
	LatencyMs int
	ErrorMsg  string
	ProbedAt  time.Time `gorm:"index"`
}

// OperationLog 操作日志
type OperationLog struct {
	ID            uint   `gorm:"primaryKey"`
	TaskID        uint   `gorm:"index;not null"`
	OperationType string `gorm:"not null"` // "pause"/"delete"/"resume"/"add"
	RecordID      string
	IP            string `gorm:"not null"`
	RecordType    string `gorm:"not null"`
	Success       bool   `gorm:"not null"`
	Detail        string
	OperatedAt    time.Time `gorm:"index"`
}

// ExcludedIP 用户手动排除的 IP（不再纳入探测）
type ExcludedIP struct {
	ID        uint   `gorm:"primaryKey"`
	TaskID    uint   `gorm:"index;not null"`
	IP        string `gorm:"not null"`
	CreatedAt time.Time
}

// ========== 解析池相关模型 ==========

// ResolutionPool 解析池
// 包含多个备用IP或域名的资源池，用于故障转移时选择健康的备用资源
type ResolutionPool struct {
	ID           uint   `gorm:"primaryKey"`
	Name         string `gorm:"uniqueIndex;not null"` // 池名称（唯一）
	ResourceType string `gorm:"not null"`             // 资源类型: ip / domain
	Description  string // 描述信息

	// 探测配置
	ProbeProtocol    string `gorm:"not null"` // 探测协议: ICMP/TCP/UDP/HTTP/HTTPS
	ProbePort        int    // 探测端口
	ProbeIntervalSec int    `gorm:"not null"` // 探测间隔（秒）
	TimeoutMs        int    `gorm:"not null"` // 超时时间（毫秒）
	FailThreshold    int    `gorm:"not null"` // 失败阈值
	RecoverThreshold int    `gorm:"not null"` // 恢复阈值

	CreatedAt time.Time
	UpdatedAt time.Time
}

// PoolResource 解析池中的资源
// 表示解析池中的一个备用IP地址或域名，包含健康状态和性能指标
type PoolResource struct {
	ID     uint   `gorm:"primaryKey"`
	PoolID uint   `gorm:"index;not null"` // 所属解析池ID
	Value  string `gorm:"not null"`       // IP地址或域名

	// 健康状态
	HealthStatus         string `gorm:"not null;default:'unknown'"` // 健康状态: healthy / unhealthy / unknown
	ConsecutiveFails     int    `gorm:"default:0"`                  // 连续失败次数
	ConsecutiveSuccesses int    `gorm:"default:0"`                  // 连续成功次数

	// 性能指标
	AvgLatencyMs int        `gorm:"default:0"` // 平均延迟（最近10次成功探测，毫秒）
	LastProbeAt  *time.Time // 最后探测时间

	// 启用状态
	Enabled bool `gorm:"not null;default:true"` // 是否启用探测

	CreatedAt time.Time
	UpdatedAt time.Time
}

// PoolProbeResult 解析池资源探测结果
// 记录每次对解析池资源的探测结果，用于计算平均延迟和健康状态
type PoolProbeResult struct {
	ID         uint      `gorm:"primaryKey"`
	ResourceID uint      `gorm:"index;not null"` // 所属资源ID
	Success    bool      `gorm:"not null"`       // 是否成功
	LatencyMs  int       // 延迟（毫秒）
	ErrorMsg   string    // 错误信息
	ProbedAt   time.Time `gorm:"index"` // 探测时间
}

// CNAMETarget CNAME记录解析出的IP目标
// 当任务类型为CNAME时，系统会解析每条CNAME记录指向的所有IP地址，
// 并对每个IP进行独立的健康探测。每个IP关联到它所属的CNAME记录值，
// 以便按CNAME记录维度统计健康状态和执行暂停/删除操作。
type CNAMETarget struct {
	ID         uint   `gorm:"primaryKey"`
	TaskID     uint   `gorm:"index;not null"`      // 所属任务ID
	CNAMEValue string `gorm:"not null;default:''"` // 所属CNAME记录的值（如 download.example.com）
	IP         string `gorm:"not null"`            // 解析出的IP地址

	// 健康状态
	HealthStatus         string `gorm:"not null;default:'unknown'"` // 健康状态: healthy / unhealthy / unknown
	ConsecutiveFails     int    `gorm:"default:0"`                  // 连续失败次数
	ConsecutiveSuccesses int    `gorm:"default:0"`                  // 连续成功次数

	LastProbeAt *time.Time // 最后探测时间
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ========== 通知模块相关模型 ==========

// EventType 通知事件类型
type EventType string

const (
	// EventTypeFailover 故障转移事件
	EventTypeFailover EventType = "failover"
	// EventTypeRecovery 恢复事件
	EventTypeRecovery EventType = "recovery"
	// EventTypeConsecutiveFail 连续失败告警事件
	EventTypeConsecutiveFail EventType = "consecutive_fail"
)

// SMTPConfig SMTP 邮件服务器配置
type SMTPConfig struct {
	ID                uint   `gorm:"primaryKey"`
	Host              string `gorm:"not null"` // SMTP 服务器地址
	Port              int    `gorm:"not null"` // SMTP 端口
	Username          string `gorm:"not null"` // 用户名
	PasswordEncrypted string `gorm:"not null"` // 加密后的密码
	FromAddress       string `gorm:"not null"` // 发件人地址
	ToAddress         string `gorm:"not null"` // 收件人地址
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NotificationSetting 任务通知设置
type NotificationSetting struct {
	ID               uint `gorm:"primaryKey"`
	TaskID           uint `gorm:"uniqueIndex;not null"` // 关联的探测任务ID
	NotifyFailover   bool `gorm:"default:false"`        // 是否通知故障转移
	NotifyRecovery   bool `gorm:"default:false"`        // 是否通知恢复
	NotifyConsecFail bool `gorm:"default:false"`        // 是否通知连续失败
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NotificationLog 通知发送记录
type NotificationLog struct {
	ID          uint      `gorm:"primaryKey"`
	TaskID      uint      `gorm:"index;not null"` // 任务ID
	EventType   string    `gorm:"not null"`       // 事件类型
	ChannelType string    `gorm:"not null"`       // 渠道类型（如 email）
	Success     bool      `gorm:"not null"`       // 是否发送成功
	ErrorMsg    string    // 错误信息
	Detail      string    // 事件详情摘要
	SentAt      time.Time `gorm:"index;not null"` // 发送时间
}
