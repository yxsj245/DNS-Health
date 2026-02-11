// Package api 健康监控任务 API 处理器
// 实现健康监控任务的创建、查询列表、查询详情接口
// 需求: 10.1, 10.2, 10.3, 10.9
package api

import (
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/monitor"
	"dns-health-monitor/internal/prober"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthMonitorHandler 健康监控任务管理处理器
type HealthMonitorHandler struct {
	Manager   monitor.HealthMonitorManager // 健康监控管理器
	Scheduler *monitor.MonitorScheduler    // 健康监控调度器
	DB        *gorm.DB                     // 数据库连接（用于凭证验证）
}

// NewHealthMonitorHandler 创建健康监控任务管理处理器
func NewHealthMonitorHandler(manager monitor.HealthMonitorManager, scheduler *monitor.MonitorScheduler, db *gorm.DB) *HealthMonitorHandler {
	return &HealthMonitorHandler{
		Manager:   manager,
		Scheduler: scheduler,
		DB:        db,
	}
}

// CreateHealthMonitorRequest 创建健康监控任务请求体
type CreateHealthMonitorRequest struct {
	CredentialID     *uint  `json:"credential_id"` // 可选，为空时直接DNS解析
	Domain           string `json:"domain"`
	SubDomain        string `json:"sub_domain"`
	RecordType       string `json:"record_type"`
	ProbeProtocol    string `json:"probe_protocol"`
	ProbePort        int    `json:"probe_port"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	TimeoutMs        int    `json:"timeout_ms"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
	// CNAME专用字段
	FailThresholdType  string `json:"fail_threshold_type"`
	FailThresholdValue int    `json:"fail_threshold_value"`
}

// HealthMonitorResponse 健康监控任务响应
type HealthMonitorResponse struct {
	ID                 uint   `json:"id"`
	CredentialID       *uint  `json:"credential_id"`
	Domain             string `json:"domain"`
	SubDomain          string `json:"sub_domain"`
	RecordType         string `json:"record_type"`
	ProbeProtocol      string `json:"probe_protocol"`
	ProbePort          int    `json:"probe_port"`
	ProbeIntervalSec   int    `json:"probe_interval_sec"`
	TimeoutMs          int    `json:"timeout_ms"`
	FailThreshold      int    `json:"fail_threshold"`
	RecoverThreshold   int    `json:"recover_threshold"`
	FailThresholdType  string `json:"fail_threshold_type"`
	FailThresholdValue int    `json:"fail_threshold_value"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// HealthMonitorListItem 任务列表项（包含健康统计）
type HealthMonitorListItem struct {
	ID               uint   `json:"id"`
	CredentialID     *uint  `json:"credential_id"`
	Domain           string `json:"domain"`
	SubDomain        string `json:"sub_domain"`
	RecordType       string `json:"record_type"`
	ProbeProtocol    string `json:"probe_protocol"`
	ProbePort        int    `json:"probe_port"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	TimeoutMs        int    `json:"timeout_ms"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
	Enabled          bool   `json:"enabled"`
	TargetCount      int    `json:"target_count"`
	HealthyCount     int    `json:"healthy_count"`
	UnhealthyCount   int    `json:"unhealthy_count"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// HealthMonitorTargetResponse 监控目标响应
type HealthMonitorTargetResponse struct {
	ID                   uint   `json:"id"`
	IP                   string `json:"ip"`
	CNAMEValue           string `json:"cname_value,omitempty"`
	HealthStatus         string `json:"health_status"`
	ConsecutiveFails     int    `json:"consecutive_fails"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	AvgLatencyMs         int    `json:"avg_latency_ms"`
	LastProbeAt          string `json:"last_probe_at,omitempty"`
}

// HealthMonitorDetailResponse 任务详情响应（包含监控目标列表）
type HealthMonitorDetailResponse struct {
	HealthMonitorResponse
	Targets []HealthMonitorTargetResponse `json:"targets"`
}

// healthMonitorToResponse 将模型转换为响应DTO
func healthMonitorToResponse(task *model.HealthMonitorTask) HealthMonitorResponse {
	return HealthMonitorResponse{
		ID:                 task.ID,
		CredentialID:       task.CredentialID,
		Domain:             task.Domain,
		SubDomain:          task.SubDomain,
		RecordType:         task.RecordType,
		ProbeProtocol:      task.ProbeProtocol,
		ProbePort:          task.ProbePort,
		ProbeIntervalSec:   task.ProbeIntervalSec,
		TimeoutMs:          task.TimeoutMs,
		FailThreshold:      task.FailThreshold,
		RecoverThreshold:   task.RecoverThreshold,
		FailThresholdType:  task.FailThresholdType,
		FailThresholdValue: task.FailThresholdValue,
		Enabled:            task.Enabled,
		CreatedAt:          task.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:          task.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// targetToResponse 将监控目标模型转换为响应DTO
func targetToResponse(target *model.HealthMonitorTarget) HealthMonitorTargetResponse {
	resp := HealthMonitorTargetResponse{
		ID:                   target.ID,
		IP:                   target.IP,
		CNAMEValue:           target.CNAMEValue,
		HealthStatus:         target.HealthStatus,
		ConsecutiveFails:     target.ConsecutiveFails,
		ConsecutiveSuccesses: target.ConsecutiveSuccesses,
		AvgLatencyMs:         target.AvgLatencyMs,
	}
	if target.LastProbeAt != nil {
		resp.LastProbeAt = target.LastProbeAt.Format("2006-01-02 15:04:05")
	}
	return resp
}

// validateHealthMonitorRequest 验证创建健康监控任务请求参数
// 需求 10.9: API请求失败时返回明确的错误代码和错误信息
func validateHealthMonitorRequest(req *CreateHealthMonitorRequest, db *gorm.DB) string {
	// 验证必填字段
	if req.Domain == "" {
		return "域名不能为空"
	}
	if req.SubDomain == "" {
		return "主机记录不能为空"
	}
	// 凭证为可选项：如果提供了凭证ID，验证其是否存在
	if req.CredentialID != nil && *req.CredentialID > 0 {
		var credential model.Credential
		if err := db.First(&credential, *req.CredentialID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return "指定的凭证不存在"
			}
			return "查询凭证失败"
		}
	}

	// 验证记录类型（需求 1.2）
	if req.RecordType == "" {
		return "记录类型不能为空"
	}
	if !model.IsValidRecordType(req.RecordType) {
		return "记录类型无效，必须是 A、AAAA、A_AAAA 或 CNAME"
	}

	// 验证探测协议（需求 1.4）
	if req.ProbeProtocol == "" {
		return "探测协议不能为空"
	}
	if !prober.IsValidProtocol(prober.ProbeProtocol(req.ProbeProtocol)) {
		return "探测协议无效，必须是 ICMP、TCP、UDP、HTTP 或 HTTPS"
	}

	// 验证数值参数（需求 1.5, 1.6, 1.7, 1.8）
	if req.ProbeIntervalSec <= 0 {
		return "探测间隔必须为正整数"
	}
	if req.TimeoutMs <= 0 {
		return "超时时间必须为正整数"
	}
	if req.FailThreshold <= 0 {
		return "失败阈值必须为正整数"
	}
	if req.RecoverThreshold <= 0 {
		return "恢复阈值必须为正整数"
	}

	// 验证CNAME专用字段
	if req.FailThresholdType != "" && !model.IsValidFailThresholdType(req.FailThresholdType) {
		return "失败阈值类型无效，必须是 count 或 percent"
	}

	return ""
}

// CreateHealthMonitor 创建健康监控任务
// POST /api/health-monitors
// 需求 10.1: 提供创建健康监控任务的API接口
func (h *HealthMonitorHandler) CreateHealthMonitor(c *gin.Context) {
	var req CreateHealthMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请求参数无效"})
		return
	}

	// 验证参数
	if errMsg := validateHealthMonitorRequest(&req, h.DB); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": errMsg})
		return
	}

	// 设置CNAME字段默认值
	failThresholdType := req.FailThresholdType
	if failThresholdType == "" {
		failThresholdType = string(model.FailThresholdCount)
	}
	failThresholdValue := req.FailThresholdValue
	if failThresholdValue <= 0 {
		failThresholdValue = 1
	}

	// 构建任务模型
	task := &model.HealthMonitorTask{
		CredentialID:       req.CredentialID,
		Domain:             req.Domain,
		SubDomain:          req.SubDomain,
		RecordType:         req.RecordType,
		ProbeProtocol:      req.ProbeProtocol,
		ProbePort:          req.ProbePort,
		ProbeIntervalSec:   req.ProbeIntervalSec,
		TimeoutMs:          req.TimeoutMs,
		FailThreshold:      req.FailThreshold,
		RecoverThreshold:   req.RecoverThreshold,
		FailThresholdType:  failThresholdType,
		FailThresholdValue: failThresholdValue,
	}

	// 调用管理器创建任务（需求 1.9: 创建任务并返回ID）
	if err := h.Manager.CreateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "创建监控任务失败: " + err.Error()})
		return
	}

	// 调用调度器启动任务（需求 1.10: 创建成功后自动启动）
	if h.Scheduler != nil {
		if err := h.Scheduler.AddTask(*task); err != nil {
			// 调度器通知失败不影响任务创建，记录警告
			c.JSON(http.StatusCreated, gin.H{
				"code":    0,
				"message": "success",
				"data":    healthMonitorToResponse(task),
				"warning": "任务已创建，但启动调度失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    0,
		"message": "success",
		"data":    healthMonitorToResponse(task),
	})
}

// ListHealthMonitors 查询健康监控任务列表
// GET /api/health-monitors
// 需求 10.2: 提供查询健康监控任务列表的API接口
func (h *HealthMonitorHandler) ListHealthMonitors(c *gin.Context) {
	// 获取任务列表
	tasks, err := h.Manager.ListTasks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "查询任务列表失败: " + err.Error()})
		return
	}

	// 构建响应列表，包含每个任务的健康统计
	items := make([]HealthMonitorListItem, 0, len(tasks))
	for _, task := range tasks {
		item := HealthMonitorListItem{
			ID:               task.ID,
			CredentialID:     task.CredentialID,
			Domain:           task.Domain,
			SubDomain:        task.SubDomain,
			RecordType:       task.RecordType,
			ProbeProtocol:    task.ProbeProtocol,
			ProbePort:        task.ProbePort,
			ProbeIntervalSec: task.ProbeIntervalSec,
			TimeoutMs:        task.TimeoutMs,
			FailThreshold:    task.FailThreshold,
			RecoverThreshold: task.RecoverThreshold,
			Enabled:          task.Enabled,
			CreatedAt:        task.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:        task.UpdatedAt.Format("2006-01-02 15:04:05"),
		}

		// 查询该任务的监控目标，统计健康/不健康数量
		targets, err := h.Manager.GetTaskTargets(task.ID)
		if err == nil {
			item.TargetCount = len(targets)
			for _, t := range targets {
				switch t.HealthStatus {
				case "healthy":
					item.HealthyCount++
				case "unhealthy":
					item.UnhealthyCount++
				}
			}
		}

		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    items,
	})
}

// GetHealthMonitor 查询健康监控任务详情
// GET /api/health-monitors/:id
// 需求 10.3: 提供查询单个健康监控任务详情的API接口
func (h *HealthMonitorHandler) GetHealthMonitor(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 获取任务详情
	task, err := h.Manager.GetTask(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
		return
	}

	// 获取监控目标列表
	targets, err := h.Manager.GetTaskTargets(task.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "查询监控目标失败: " + err.Error()})
		return
	}

	// 构建详情响应
	detail := HealthMonitorDetailResponse{
		HealthMonitorResponse: healthMonitorToResponse(task),
		Targets:               make([]HealthMonitorTargetResponse, 0, len(targets)),
	}
	for _, t := range targets {
		detail.Targets = append(detail.Targets, targetToResponse(t))
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    detail,
	})
}

// UpdateHealthMonitorRequest 更新健康监控任务请求体
// 只包含允许更新的字段，使用指针类型区分"未传"和"传了零值"
type UpdateHealthMonitorRequest struct {
	ProbeProtocol    *string `json:"probe_protocol"`
	ProbePort        *int    `json:"probe_port"`
	ProbeIntervalSec *int    `json:"probe_interval_sec"`
	TimeoutMs        *int    `json:"timeout_ms"`
	FailThreshold    *int    `json:"fail_threshold"`
	RecoverThreshold *int    `json:"recover_threshold"`
	// CNAME专用字段
	FailThresholdType  *string `json:"fail_threshold_type"`
	FailThresholdValue *int    `json:"fail_threshold_value"`
}

// toUpdatesMap 将更新请求转换为map，仅包含非nil字段
func (r *UpdateHealthMonitorRequest) toUpdatesMap() map[string]interface{} {
	updates := make(map[string]interface{})
	if r.ProbeProtocol != nil {
		updates["probe_protocol"] = *r.ProbeProtocol
	}
	if r.ProbePort != nil {
		updates["probe_port"] = *r.ProbePort
	}
	if r.ProbeIntervalSec != nil {
		updates["probe_interval_sec"] = *r.ProbeIntervalSec
	}
	if r.TimeoutMs != nil {
		updates["timeout_ms"] = *r.TimeoutMs
	}
	if r.FailThreshold != nil {
		updates["fail_threshold"] = *r.FailThreshold
	}
	if r.RecoverThreshold != nil {
		updates["recover_threshold"] = *r.RecoverThreshold
	}
	if r.FailThresholdType != nil {
		updates["fail_threshold_type"] = *r.FailThresholdType
	}
	if r.FailThresholdValue != nil {
		updates["fail_threshold_value"] = *r.FailThresholdValue
	}
	return updates
}

// UpdateHealthMonitor 更新健康监控任务配置
// PUT /api/health-monitors/:id
// 需求 10.4: 提供更新健康监控任务配置的API接口
// 需求 7.4: 验证新配置并应用更改
// 需求 7.5: 更新任务配置时使用新配置重新启动监控
func (h *HealthMonitorHandler) UpdateHealthMonitor(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 解析请求体
	var req UpdateHealthMonitorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请求参数无效"})
		return
	}

	// 转换为更新map
	updates := req.toUpdatesMap()
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "没有需要更新的字段"})
		return
	}

	// 调用管理器更新任务（内部会验证字段有效性）
	if err := h.Manager.UpdateTask(uint(id), updates); err != nil {
		// 根据错误信息判断HTTP状态码
		if err.Error() == "任务不存在" {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
		return
	}

	// 获取更新后的任务，用于重启调度器
	task, err := h.Manager.GetTask(uint(id))
	if err != nil {
		// 更新已成功，但获取任务失败，仍返回成功
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
		return
	}

	// 调用调度器使用新配置重启任务（需求 7.5）
	if h.Scheduler != nil {
		if err := h.Scheduler.RestartTask(*task); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "success",
				"warning": "任务已更新，但重启调度失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// PauseHealthMonitor 暂停健康监控任务
// POST /api/health-monitors/:id/pause
// 需求 10.5: 提供暂停健康监控任务的API接口
// 需求 7.1: 停止该任务的DNS解析和探测
func (h *HealthMonitorHandler) PauseHealthMonitor(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 调用管理器暂停任务（更新数据库状态为disabled）
	if err := h.Manager.PauseTask(uint(id)); err != nil {
		if err.Error() == "任务不存在" {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
			return
		}
		if err.Error() == "任务已经处于暂停状态" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "暂停任务失败: " + err.Error()})
		return
	}

	// 调用调度器停止任务调度
	if h.Scheduler != nil {
		if err := h.Scheduler.StopTask(uint(id)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "success",
				"warning": "任务已暂停，但停止调度失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// ResumeHealthMonitor 恢复健康监控任务
// POST /api/health-monitors/:id/resume
// 需求 10.6: 提供恢复健康监控任务的API接口
// 需求 7.2: 重新启动该任务的DNS解析和探测
func (h *HealthMonitorHandler) ResumeHealthMonitor(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 调用管理器恢复任务（更新数据库状态为enabled）
	if err := h.Manager.ResumeTask(uint(id)); err != nil {
		if err.Error() == "任务不存在" {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
			return
		}
		if err.Error() == "任务已经在运行中" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "恢复任务失败: " + err.Error()})
		return
	}

	// 获取任务信息，用于重启调度器
	task, err := h.Manager.GetTask(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"warning": "任务已恢复，但获取任务信息失败: " + err.Error(),
		})
		return
	}

	// 调用调度器重新启动任务
	if h.Scheduler != nil {
		if err := h.Scheduler.RestartTask(*task); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "success",
				"warning": "任务已恢复，但启动调度失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// DeleteHealthMonitor 删除健康监控任务
// DELETE /api/health-monitors/:id
// 需求 10.7: 提供删除健康监控任务的API接口
// 需求 7.3: 停止任务并删除所有相关数据
func (h *HealthMonitorHandler) DeleteHealthMonitor(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 先调用调度器停止任务调度（在删除数据之前停止）
	if h.Scheduler != nil {
		if err := h.Scheduler.RemoveTask(uint(id)); err != nil {
			// 调度器停止失败不阻塞删除，记录警告
			_ = err
		}
	}

	// 调用管理器删除任务及关联数据（targets和results）
	if err := h.Manager.DeleteTask(uint(id)); err != nil {
		if err.Error() == "任务不存在" {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "删除任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// HealthMonitorResultResponse 探测结果响应项
type HealthMonitorResultResponse struct {
	IP        string `json:"ip"`
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	ErrorMsg  string `json:"error_msg"`
	ProbedAt  string `json:"probed_at"`
}

// GetHealthMonitorResults 查询探测结果历史
// GET /api/health-monitors/:id/results
// 需求 10.8: 提供查询探测结果历史的API接口
// 需求 5.5: 支持按任务ID和时间范围过滤
func (h *HealthMonitorHandler) GetHealthMonitorResults(c *gin.Context) {
	// 解析任务ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的任务ID"})
		return
	}

	// 验证任务是否存在
	_, err = h.Manager.GetTask(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": err.Error()})
		return
	}

	// 构建查询条件
	query := h.DB.Model(&model.HealthMonitorResult{}).Where("task_id = ?", id)

	// 时间范围过滤（需求 5.5）
	if startTime := c.Query("start_time"); startTime != "" {
		query = query.Where("probed_at >= ?", startTime)
	}
	if endTime := c.Query("end_time"); endTime != "" {
		query = query.Where("probed_at <= ?", endTime)
	}

	// 按IP过滤（可选）
	if ip := c.Query("ip"); ip != "" {
		query = query.Where("ip = ?", ip)
	}

	// 按成功/失败过滤（可选）
	if successStr := c.Query("success"); successStr != "" {
		success, err := strconv.ParseBool(successStr)
		if err == nil {
			query = query.Where("success = ?", success)
		}
	}

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "查询探测结果总数失败: " + err.Error()})
		return
	}

	// 分页参数
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
		}
	}
	offset := (page - 1) * pageSize

	// 查询结果列表，按探测时间倒序
	var results []model.HealthMonitorResult
	if err := query.Order("probed_at DESC").Offset(offset).Limit(pageSize).Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "查询探测结果失败: " + err.Error()})
		return
	}

	// 构建响应
	items := make([]HealthMonitorResultResponse, 0, len(results))
	for _, r := range results {
		items = append(items, HealthMonitorResultResponse{
			IP:        r.IP,
			Success:   r.Success,
			LatencyMs: r.LatencyMs,
			ErrorMsg:  r.ErrorMsg,
			ProbedAt:  r.ProbedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total": total,
			"items": items,
		},
	})
}

// GetHealthMonitorLatency 获取健康监控任务的延迟数据
// GET /api/health-monitors/:id/latency?ip=xxx&start_time=xxx&end_time=xxx
// 按 probed_at 升序返回指定任务、IP、时间范围内的延迟数据
// 验证需求：1.2, 1.3, 1.4, 1.5, 1.6
func (h *HealthMonitorHandler) GetHealthMonitorLatency(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证 ip 参数（必填）
	ip := c.Query("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必填参数: ip"})
		return
	}

	// 验证任务是否存在
	task, err := h.Manager.GetTask(uint(taskID))
	if err != nil || task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// 解析时间范围，默认最近 24 小时
	now := time.Now()
	startTime := now.Add(-24 * time.Hour)
	endTime := now

	if st := c.Query("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339Nano, st); err == nil {
			startTime = t
		} else if t, err := time.Parse(time.RFC3339, st); err == nil {
			startTime = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", st); err == nil {
			startTime = t
		}
	}
	if et := c.Query("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339Nano, et); err == nil {
			endTime = t
		} else if t, err := time.Parse(time.RFC3339, et); err == nil {
			endTime = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", et); err == nil {
			endTime = t
		}
	}

	// 查询延迟数据，按 probed_at 升序
	// 将时间统一转为本地时间，确保与数据库中存储的时区一致
	startTime = startTime.Local()
	endTime = endTime.Local()

	var results []model.HealthMonitorResult
	if err := h.DB.Where("task_id = ? AND ip = ? AND probed_at BETWEEN ? AND ?",
		taskID, ip, startTime, endTime).
		Order("probed_at ASC").
		Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询延迟数据失败"})
		return
	}

	// 构建响应，复用 LatencyDataPoint 结构体（定义在 status.go 中）
	data := make([]LatencyDataPoint, 0, len(results))
	for _, r := range results {
		data = append(data, LatencyDataPoint{
			LatencyMs: r.LatencyMs,
			ProbedAt:  r.ProbedAt.Format("2006-01-02 15:04:05"),
			Success:   r.Success,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}
