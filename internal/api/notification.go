// Package api 通知模块 API 接口实现
package api

import (
	"net/http"
	"strconv"
	"strings"

	"dns-health-monitor/internal/crypto"
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/notification"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NotificationHandler 通知管理处理器
type NotificationHandler struct {
	db                  *gorm.DB                          // 数据库连接
	encKey              []byte                            // 加密密钥（用于 SMTP 密码加密/解密）
	notificationManager *notification.NotificationManager // 通知管理器
}

// NewNotificationHandler 创建通知管理处理器
// db: 数据库连接
// encKey: 加密密钥
// nm: 通知管理器实例
func NewNotificationHandler(db *gorm.DB, encKey []byte, nm *notification.NotificationManager) *NotificationHandler {
	return &NotificationHandler{
		db:                  db,
		encKey:              encKey,
		notificationManager: nm,
	}
}

// ========== 请求/响应结构体 ==========

// SMTPConfigRequest 保存 SMTP 配置请求体
type SMTPConfigRequest struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
}

// SMTPConfigResponse SMTP 配置响应（密码脱敏）
type SMTPConfigResponse struct {
	ID          uint   `json:"id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"` // 脱敏后的密码
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
}

// SMTPTestRequest 测试 SMTP 连接请求体
type SMTPTestRequest struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
}

// NotificationSettingResponse 通知设置响应（包含任务名称）
type NotificationSettingResponse struct {
	ID               uint   `json:"id"`
	TaskID           uint   `json:"task_id"`
	TaskName         string `json:"task_name"`           // 任务名称（域名+子域名）
	TaskType         string `json:"task_type,omitempty"` // 任务类型: probe / health_monitor
	NotifyFailover   bool   `json:"notify_failover"`
	NotifyRecovery   bool   `json:"notify_recovery"`
	NotifyConsecFail bool   `json:"notify_consec_fail"`
}

// UpdateNotificationSettingRequest 更新通知设置请求体
type UpdateNotificationSettingRequest struct {
	NotifyFailover   bool `json:"notify_failover"`
	NotifyRecovery   bool `json:"notify_recovery"`
	NotifyConsecFail bool `json:"notify_consec_fail"`
}

// BatchUpdateSettingsRequest 批量更新通知设置请求体
type BatchUpdateSettingsRequest struct {
	EnableAll bool `json:"enable_all"` // true: 全部启用, false: 全部禁用
}

// NotificationLogResponse 通知记录响应
type NotificationLogResponse struct {
	ID          uint   `json:"id"`
	TaskID      uint   `json:"task_id"`
	TaskName    string `json:"task_name"` // 任务名称
	EventType   string `json:"event_type"`
	ChannelType string `json:"channel_type"`
	Success     bool   `json:"success"`
	ErrorMsg    string `json:"error_msg"`
	Detail      string `json:"detail"`
	SentAt      string `json:"sent_at"`
}

// ========== SMTP 配置相关接口 ==========

// GetSMTPConfig 获取 SMTP 配置（密码脱敏显示）
// GET /api/notification/smtp-config
func (h *NotificationHandler) GetSMTPConfig(c *gin.Context) {
	var config model.SMTPConfig
	if err := h.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 未配置 SMTP，返回空配置
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 SMTP 配置失败"})
		return
	}

	// 解密密码用于脱敏显示
	maskedPassword := "****"
	if config.PasswordEncrypted != "" {
		password, err := crypto.Decrypt(config.PasswordEncrypted, h.encKey)
		if err == nil && password != "" {
			maskedPassword = crypto.MaskSecret(password)
		}
	}

	resp := SMTPConfigResponse{
		ID:          config.ID,
		Host:        config.Host,
		Port:        config.Port,
		Username:    config.Username,
		Password:    maskedPassword,
		FromAddress: config.FromAddress,
		ToAddress:   config.ToAddress,
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// SaveSMTPConfig 保存 SMTP 配置（密码加密存储）
// PUT /api/notification/smtp-config
func (h *NotificationHandler) SaveSMTPConfig(c *gin.Context) {
	var req SMTPConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证必填字段
	if req.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SMTP 服务器地址不能为空"})
		return
	}
	if req.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SMTP 端口必须为正整数"})
		return
	}
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}
	if req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码不能为空"})
		return
	}
	if req.FromAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "发件人地址不能为空"})
		return
	}
	if req.ToAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "收件人地址不能为空"})
		return
	}

	// 加密密码
	encryptedPassword, err := crypto.Encrypt(req.Password, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	// 查找现有配置（系统只保存一条 SMTP 配置）
	var config model.SMTPConfig
	if err := h.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 创建新配置
			config = model.SMTPConfig{
				Host:              req.Host,
				Port:              req.Port,
				Username:          req.Username,
				PasswordEncrypted: encryptedPassword,
				FromAddress:       req.FromAddress,
				ToAddress:         req.ToAddress,
			}
			if err := h.db.Create(&config).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 SMTP 配置失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "SMTP 配置保存成功"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询 SMTP 配置失败"})
		return
	}

	// 更新现有配置
	config.Host = req.Host
	config.Port = req.Port
	config.Username = req.Username
	config.PasswordEncrypted = encryptedPassword
	config.FromAddress = req.FromAddress
	config.ToAddress = req.ToAddress

	if err := h.db.Save(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 SMTP 配置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMTP 配置保存成功"})
}

// TestSMTP 测试 SMTP 连接
// POST /api/notification/smtp-test
func (h *NotificationHandler) TestSMTP(c *gin.Context) {
	var req SMTPTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 如果密码为空或包含脱敏标记（****），从数据库获取已保存的真实密码
	password := req.Password
	if password == "" || strings.Contains(password, "****") {
		var config model.SMTPConfig
		if err := h.db.First(&config).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请提供 SMTP 密码"})
			return
		}
		decrypted, err := crypto.Decrypt(config.PasswordEncrypted, h.encKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "解密已保存的密码失败"})
			return
		}
		password = decrypted
	}

	// 构建邮件渠道配置
	emailConfig := &notification.EmailChannelConfig{
		Host:        req.Host,
		Port:        req.Port,
		Username:    req.Username,
		Password:    password,
		FromAddress: req.FromAddress,
		ToAddress:   req.ToAddress,
	}

	// 测试 SMTP 连接并发送测试邮件
	if err := notification.TestSMTPConnection(emailConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "SMTP 连接测试成功，测试邮件已发送"})
}

// ========== 任务通知设置相关接口 ==========

// GetNotificationSettings 获取所有任务的通知设置
// GET /api/notification/settings
// 返回所有探测任务及其通知设置（join tasks 表获取任务名称）
func (h *NotificationHandler) GetNotificationSettings(c *gin.Context) {
	// 查询所有探测任务
	var probeTasks []model.ProbeTask
	if err := h.db.Order("created_at DESC").Find(&probeTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询探测任务列表失败"})
		return
	}

	// 查询所有健康监控任务
	var healthTasks []model.HealthMonitorTask
	if err := h.db.Order("created_at DESC").Find(&healthTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询健康监控任务列表失败"})
		return
	}

	// 查询所有通知设置
	var settings []model.NotificationSetting
	if err := h.db.Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询通知设置失败"})
		return
	}

	// 构建 taskID -> setting 映射
	settingMap := make(map[uint]*model.NotificationSetting)
	for i := range settings {
		settingMap[settings[i].TaskID] = &settings[i]
	}

	// 构建响应列表
	resp := make([]NotificationSettingResponse, 0, len(probeTasks)+len(healthTasks))

	// 添加探测任务
	for _, task := range probeTasks {
		taskName := task.Domain
		if task.SubDomain != "" && task.SubDomain != "@" {
			taskName = task.SubDomain + "." + task.Domain
		}

		item := NotificationSettingResponse{
			TaskID:   task.ID,
			TaskName: taskName,
			TaskType: "probe",
		}

		if setting, ok := settingMap[task.ID]; ok {
			item.ID = setting.ID
			item.NotifyFailover = setting.NotifyFailover
			item.NotifyRecovery = setting.NotifyRecovery
			item.NotifyConsecFail = setting.NotifyConsecFail
		}

		resp = append(resp, item)
	}

	// 添加健康监控任务
	for _, task := range healthTasks {
		taskName := task.Domain
		if task.SubDomain != "" && task.SubDomain != "@" {
			taskName = task.SubDomain + "." + task.Domain
		}

		item := NotificationSettingResponse{
			TaskID:   task.ID,
			TaskName: taskName,
			TaskType: "health_monitor",
		}

		if setting, ok := settingMap[task.ID]; ok {
			item.ID = setting.ID
			item.NotifyFailover = setting.NotifyFailover
			item.NotifyRecovery = setting.NotifyRecovery
			item.NotifyConsecFail = setting.NotifyConsecFail
		}

		resp = append(resp, item)
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateNotificationSetting 更新单个任务的通知设置
// PUT /api/notification/settings/:taskId
// 使用 upsert 模式：如果设置不存在则创建，存在则更新
func (h *NotificationHandler) UpdateNotificationSetting(c *gin.Context) {
	// 解析任务 ID
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证任务是否存在（探测任务或健康监控任务）
	var probeTask model.ProbeTask
	var healthTask model.HealthMonitorTask
	probeErr := h.db.First(&probeTask, taskID).Error
	healthErr := h.db.First(&healthTask, taskID).Error
	if probeErr != nil && healthErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	var req UpdateNotificationSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// Upsert: 查找或创建通知设置
	var setting model.NotificationSetting
	if err := h.db.Where("task_id = ?", taskID).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 创建新设置
			setting = model.NotificationSetting{
				TaskID:           uint(taskID),
				NotifyFailover:   req.NotifyFailover,
				NotifyRecovery:   req.NotifyRecovery,
				NotifyConsecFail: req.NotifyConsecFail,
			}
			if err := h.db.Create(&setting).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "创建通知设置失败"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "通知设置已保存"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询通知设置失败"})
		return
	}

	// 更新现有设置
	setting.NotifyFailover = req.NotifyFailover
	setting.NotifyRecovery = req.NotifyRecovery
	setting.NotifyConsecFail = req.NotifyConsecFail

	if err := h.db.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新通知设置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "通知设置已保存"})
}

// BatchUpdateSettings 批量更新通知设置（全部启用/禁用）
// PUT /api/notification/settings/batch
func (h *NotificationHandler) BatchUpdateSettings(c *gin.Context) {
	var req BatchUpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 查询所有探测任务
	var probeTasks []model.ProbeTask
	if err := h.db.Find(&probeTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询探测任务列表失败"})
		return
	}

	// 查询所有健康监控任务
	var healthTasks []model.HealthMonitorTask
	if err := h.db.Find(&healthTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询健康监控任务列表失败"})
		return
	}

	// 收集所有任务ID
	allTaskIDs := make([]uint, 0, len(probeTasks)+len(healthTasks))
	for _, task := range probeTasks {
		allTaskIDs = append(allTaskIDs, task.ID)
	}
	for _, task := range healthTasks {
		allTaskIDs = append(allTaskIDs, task.ID)
	}

	if len(allTaskIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "没有任务需要更新"})
		return
	}

	// 在事务中批量更新
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, taskID := range allTaskIDs {
			var setting model.NotificationSetting
			if err := tx.Where("task_id = ?", taskID).First(&setting).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					// 创建新设置
					setting = model.NotificationSetting{
						TaskID:           taskID,
						NotifyFailover:   req.EnableAll,
						NotifyRecovery:   req.EnableAll,
						NotifyConsecFail: req.EnableAll,
					}
					if err := tx.Create(&setting).Error; err != nil {
						return err
					}
					continue
				}
				return err
			}

			// 更新现有设置
			setting.NotifyFailover = req.EnableAll
			setting.NotifyRecovery = req.EnableAll
			setting.NotifyConsecFail = req.EnableAll

			if err := tx.Save(&setting).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量更新通知设置失败"})
		return
	}

	action := "禁用"
	if req.EnableAll {
		action = "启用"
	}
	c.JSON(http.StatusOK, gin.H{"message": "已批量" + action + "所有任务的通知设置"})
}

// ========== 通知记录相关接口 ==========

// GetNotificationLogs 获取通知发送记录（支持 taskId 和 eventType 筛选）
// GET /api/notification/logs?taskId=X&eventType=Y&page=1&page_size=20
func (h *NotificationHandler) GetNotificationLogs(c *gin.Context) {
	// 解析分页参数
	page, pageSize := parsePagination(c)

	// 构建查询
	query := h.db.Model(&model.NotificationLog{})

	// 按任务 ID 筛选（可选）
	if taskIDStr := c.Query("taskId"); taskIDStr != "" {
		if taskID, err := strconv.ParseUint(taskIDStr, 10, 64); err == nil {
			query = query.Where("task_id = ?", taskID)
		}
	}

	// 按事件类型筛选（可选）
	if eventType := c.Query("eventType"); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询通知记录总数失败"})
		return
	}

	// 按发送时间倒序查询
	var logs []model.NotificationLog
	offset := (page - 1) * pageSize
	if err := query.Order("sent_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询通知记录失败"})
		return
	}

	// 收集所有涉及的任务 ID，批量查询任务名称
	taskIDs := make([]uint, 0)
	taskIDSet := make(map[uint]bool)
	for _, log := range logs {
		if !taskIDSet[log.TaskID] {
			taskIDs = append(taskIDs, log.TaskID)
			taskIDSet[log.TaskID] = true
		}
	}

	// 查询任务名称映射
	taskNameMap := make(map[uint]string)
	if len(taskIDs) > 0 {
		// 查询探测任务名称
		var probeTasks []model.ProbeTask
		if err := h.db.Where("id IN ?", taskIDs).Find(&probeTasks).Error; err == nil {
			for _, task := range probeTasks {
				taskName := task.Domain
				if task.SubDomain != "" && task.SubDomain != "@" {
					taskName = task.SubDomain + "." + task.Domain
				}
				taskNameMap[task.ID] = taskName
			}
		}
		// 查询健康监控任务名称
		var healthTasks []model.HealthMonitorTask
		if err := h.db.Where("id IN ?", taskIDs).Find(&healthTasks).Error; err == nil {
			for _, task := range healthTasks {
				taskName := task.Domain
				if task.SubDomain != "" && task.SubDomain != "@" {
					taskName = task.SubDomain + "." + task.Domain
				}
				taskNameMap[task.ID] = "[监控] " + taskName
			}
		}
	}

	// 构建响应
	resp := make([]NotificationLogResponse, 0, len(logs))
	for _, log := range logs {
		item := NotificationLogResponse{
			ID:          log.ID,
			TaskID:      log.TaskID,
			TaskName:    taskNameMap[log.TaskID],
			EventType:   log.EventType,
			ChannelType: log.ChannelType,
			Success:     log.Success,
			ErrorMsg:    log.ErrorMsg,
			Detail:      log.Detail,
			SentAt:      log.SentAt.Format("2006-01-02 15:04:05"),
		}
		resp = append(resp, item)
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Total: total,
		Page:  page,
		Size:  pageSize,
		Data:  resp,
	})
}
