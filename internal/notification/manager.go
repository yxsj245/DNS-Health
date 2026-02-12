// Package notification 通知模块，采用可扩展架构设计
package notification

import (
	"context"
	"fmt"
	"log"
	"time"

	"dns-health-monitor/internal/crypto"
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/sse"

	"gorm.io/gorm"
)

// NotificationManager 通知管理器，负责加载配置、分发事件、异步发送通知
type NotificationManager struct {
	db       *gorm.DB              // 数据库连接
	channels []NotificationChannel // 已注册的通知渠道列表
	encKey   []byte                // 用于解密 SMTP 密码的加密密钥
}

// NewNotificationManager 创建通知管理器实例
// db: 数据库连接
// encKey: 加密密钥（用于解密 SMTP 密码）
// channels: 通知渠道列表
func NewNotificationManager(db *gorm.DB, encKey []byte, channels []NotificationChannel) *NotificationManager {
	return &NotificationManager{
		db:       db,
		channels: channels,
		encKey:   encKey,
	}
}

// Notify 异步发送通知（不阻塞调用方）
// 在 goroutine 中查询通知设置和 SMTP 配置，分发到各渠道
func (m *NotificationManager) Notify(event NotificationEvent) {
	go m.processNotification(event)
}

// processNotification 处理通知发送的核心逻辑
func (m *NotificationManager) processNotification(event NotificationEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 查询该任务的通知设置
	var setting model.NotificationSetting
	if err := m.db.Where("task_id = ?", event.TaskID).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 通知设置不存在，视为所有通知类型均未启用，不发送
			log.Printf("任务 %d 未配置通知设置，跳过通知发送", event.TaskID)
			return
		}
		log.Printf("查询任务 %d 通知设置失败: %v", event.TaskID, err)
		return
	}

	// 2. 根据事件类型判断是否需要发送通知
	if !m.shouldNotify(&setting, event.Type) {
		log.Printf("任务 %d 的 %s 事件通知未启用，跳过发送", event.TaskID, event.Type)
		return
	}

	// 3. 查询 SMTP 配置
	var smtpConfig model.SMTPConfig
	if err := m.db.First(&smtpConfig).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// SMTP 配置未设置，跳过邮件发送并记录警告日志
			log.Printf("警告: SMTP 配置未设置，跳过任务 %d 的通知发送", event.TaskID)
			return
		}
		log.Printf("查询 SMTP 配置失败: %v", err)
		return
	}

	// 4. 解密 SMTP 密码
	password, err := crypto.Decrypt(smtpConfig.PasswordEncrypted, m.encKey)
	if err != nil {
		log.Printf("解密 SMTP 密码失败: %v", err)
		// SMTP 配置无效，跳过发送
		return
	}

	// 5. 构建邮件渠道配置
	emailConfig := &EmailChannelConfig{
		Host:        smtpConfig.Host,
		Port:        smtpConfig.Port,
		Username:    smtpConfig.Username,
		Password:    password,
		FromAddress: smtpConfig.FromAddress,
		ToAddress:   smtpConfig.ToAddress,
	}

	// 6. 遍历所有渠道发送通知
	for _, channel := range m.channels {
		sendErr := channel.Send(ctx, event, emailConfig)

		// 7. 保存通知记录（无论成功或失败）
		notifLog := model.NotificationLog{
			TaskID:      event.TaskID,
			EventType:   string(event.Type),
			ChannelType: channel.Type(),
			Success:     sendErr == nil,
			Detail:      m.buildEventDetail(event),
			SentAt:      time.Now(),
		}

		if sendErr != nil {
			notifLog.ErrorMsg = sendErr.Error()
			log.Printf("通过 %s 渠道发送任务 %d 的 %s 通知失败: %v",
				channel.Type(), event.TaskID, event.Type, sendErr)
		} else {
			log.Printf("通过 %s 渠道成功发送任务 %d 的 %s 通知",
				channel.Type(), event.TaskID, event.Type)
		}

		// 保存通知记录到数据库
		if err := m.db.Create(&notifLog).Error; err != nil {
			log.Printf("保存通知记录失败: %v", err)
		}

		// 发布SSE事件，实时推送通知日志到前端
		sse.GetHub().PublishJSON(sse.EventNotificationLog, map[string]interface{}{
			"id":           notifLog.ID,
			"task_id":      notifLog.TaskID,
			"event_type":   notifLog.EventType,
			"channel_type": notifLog.ChannelType,
			"success":      notifLog.Success,
			"detail":       notifLog.Detail,
			"error_msg":    notifLog.ErrorMsg,
			"sent_at":      notifLog.SentAt.Format("2006-01-02 15:04:05"),
		})
	}
}

// shouldNotify 根据通知设置判断是否需要发送指定事件类型的通知
func (m *NotificationManager) shouldNotify(setting *model.NotificationSetting, eventType model.EventType) bool {
	switch eventType {
	case model.EventTypeFailover:
		return setting.NotifyFailover
	case model.EventTypeRecovery:
		return setting.NotifyRecovery
	case model.EventTypeConsecutiveFail:
		return setting.NotifyConsecFail
	default:
		return false
	}
}

// buildEventDetail 构建事件详情摘要字符串
func (m *NotificationManager) buildEventDetail(event NotificationEvent) string {
	taskName := event.Domain
	if event.SubDomain != "" && event.SubDomain != "@" {
		taskName = event.SubDomain + "." + event.Domain
	}

	switch event.Type {
	case model.EventTypeFailover:
		return fmt.Sprintf("故障转移: %s, 原始值: %s -> 切换值: %s",
			taskName, event.OriginalValue, event.BackupValue)
	case model.EventTypeRecovery:
		return fmt.Sprintf("恢复: %s, 恢复值: %s, 故障持续: %s",
			taskName, event.RecoveredValue, event.DownDuration.String())
	case model.EventTypeConsecutiveFail:
		return fmt.Sprintf("连续失败告警: %s, 连续失败 %d 次",
			taskName, event.FailCount)
	default:
		return fmt.Sprintf("未知事件: %s", taskName)
	}
}

// EmailChannelConfig 邮件渠道配置，实现 ChannelConfig 接口
type EmailChannelConfig struct {
	Host        string // SMTP 服务器地址
	Port        int    // SMTP 端口
	Username    string // 用户名
	Password    string // 密码（已解密的明文）
	FromAddress string // 发件人地址
	ToAddress   string // 收件人地址
}

// ChannelType 返回渠道类型标识
func (c *EmailChannelConfig) ChannelType() string {
	return "email"
}
