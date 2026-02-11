// Package notification 通知模块，采用可扩展架构设计，定义统一的通知发送接口
package notification

import (
	"context"
	"time"

	"dns-health-monitor/internal/model"
)

// NotificationChannel 通知渠道接口，所有通知渠道需实现此接口
// 当前实现邮件渠道，后续可扩展钉钉、飞书等渠道
type NotificationChannel interface {
	// Send 发送通知，返回错误信息
	Send(ctx context.Context, event NotificationEvent, config ChannelConfig) error
	// Type 返回渠道类型标识（如 "email"）
	Type() string
}

// ChannelConfig 渠道配置接口，不同渠道实现各自的配置结构
type ChannelConfig interface {
	// ChannelType 返回该配置对应的渠道类型标识
	ChannelType() string
}

// NotificationEvent 通知事件，包含所有事件类型所需的字段
type NotificationEvent struct {
	// 基础信息
	Type       model.EventType // 事件类型: failover / recovery / consecutive_fail
	TaskID     uint            // 任务ID
	Domain     string          // 域名
	SubDomain  string          // 子域名
	OccurredAt time.Time       // 事件发生时间

	// 故障转移相关字段
	OriginalValue string // 原始解析值
	BackupValue   string // 切换后解析值

	// 恢复相关字段
	RecoveredValue string        // 恢复后的解析值
	DownDuration   time.Duration // 故障持续时长

	// 连续失败相关字段
	FailCount     int      // 连续失败次数
	FailedIPs     []string // 失败的 IP 列表
	ProbeProtocol string   // 探测协议
	ProbePort     int      // 探测端口
	HealthStatus  string   // 当前健康状态
}
