// Package api 探测任务 CRUD 接口实现
package api

import (
	"context"
	"net/http"
	"strconv"

	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/pool"
	"dns-health-monitor/internal/prober"
	"dns-health-monitor/internal/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TaskHandler 探测任务管理处理器
type TaskHandler struct {
	DB          *gorm.DB
	Scheduler   *scheduler.Scheduler // 可选，用于通知调度器（测试时可为 nil）
	PoolManager pool.PoolManager     // 可选，用于验证解析池关联（测试时可为 nil）
}

// NewTaskHandler 创建探测任务管理处理器
// db: 数据库连接
// sched: 调度器实例（可为 nil，测试时不需要真正的调度器）
// pm: 解析池管理器实例（可为 nil，测试时不需要真正的管理器）
func NewTaskHandler(db *gorm.DB, sched *scheduler.Scheduler, pm pool.PoolManager) *TaskHandler {
	return &TaskHandler{
		DB:          db,
		Scheduler:   sched,
		PoolManager: pm,
	}
}

// CreateTaskRequest 创建/更新探测任务请求体
type CreateTaskRequest struct {
	CredentialID     uint   `json:"credential_id"`
	Domain           string `json:"domain"`
	SubDomain        string `json:"sub_domain"`
	ProbeProtocol    string `json:"probe_protocol"`
	ProbePort        int    `json:"probe_port"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	TimeoutMs        int    `json:"timeout_ms"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`

	// 新增字段 - 任务类型和策略（均有默认值，保持向后兼容）
	TaskType         string `json:"task_type"`          // 任务类型: pause_delete / switch（默认 pause_delete）
	RecordType       string `json:"record_type"`        // 解析记录类型: A / AAAA / CNAME（默认 A）
	PoolID           *uint  `json:"pool_id"`            // 关联的解析池ID（切换类型任务必填）
	SwitchBackPolicy string `json:"switch_back_policy"` // 回切策略: auto / manual（默认 auto）

	// CNAME专用字段 - 失败阈值配置
	FailThresholdType  string `json:"fail_threshold_type"`  // 阈值类型: count / percent（默认 count）
	FailThresholdValue int    `json:"fail_threshold_value"` // 阈值数值（默认 1）

	// CDN故障转移专用字段
	CDNTarget string `json:"cdn_target"` // CDN故障转移的目标IP（故障时将记录值切换为此IP）
}

// TaskResponse 探测任务响应
type TaskResponse struct {
	ID               uint   `json:"id"`
	CredentialID     uint   `json:"credential_id"`
	Domain           string `json:"domain"`
	SubDomain        string `json:"sub_domain"`
	ProbeProtocol    string `json:"probe_protocol"`
	ProbePort        int    `json:"probe_port"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	TimeoutMs        int    `json:"timeout_ms"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
	Enabled          bool   `json:"enabled"`

	// 新增字段 - 任务类型和策略
	TaskType           string `json:"task_type"`
	RecordType         string `json:"record_type"`
	PoolID             *uint  `json:"pool_id,omitempty"`
	SwitchBackPolicy   string `json:"switch_back_policy"`
	FailThresholdType  string `json:"fail_threshold_type"`
	FailThresholdValue int    `json:"fail_threshold_value"`

	// 切换状态跟踪字段 - 用于展示切换类型任务的当前状态
	OriginalValue string `json:"original_value,omitempty"` // 原始解析值（用于回切）
	CurrentValue  string `json:"current_value,omitempty"`  // 当前解析值
	IsSwitched    bool   `json:"is_switched"`              // 是否已切换到备用资源

	// CDN故障转移专用字段
	CDNTarget string `json:"cdn_target,omitempty"` // CDN故障转移的目标IP

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// taskToResponse 将模型转换为响应 DTO
func taskToResponse(task model.ProbeTask) TaskResponse {
	return TaskResponse{
		ID:               task.ID,
		CredentialID:     task.CredentialID,
		Domain:           task.Domain,
		SubDomain:        task.SubDomain,
		ProbeProtocol:    task.ProbeProtocol,
		ProbePort:        task.ProbePort,
		ProbeIntervalSec: task.ProbeIntervalSec,
		TimeoutMs:        task.TimeoutMs,
		FailThreshold:    task.FailThreshold,
		RecoverThreshold: task.RecoverThreshold,
		Enabled:          task.Enabled,

		// 新增字段 - 任务类型和策略
		TaskType:           task.TaskType,
		RecordType:         task.RecordType,
		PoolID:             task.PoolID,
		SwitchBackPolicy:   task.SwitchBackPolicy,
		FailThresholdType:  task.FailThresholdType,
		FailThresholdValue: task.FailThresholdValue,

		// 切换状态跟踪字段
		OriginalValue: task.OriginalValue,
		CurrentValue:  task.CurrentValue,
		IsSwitched:    task.IsSwitched,

		// CDN故障转移专用字段
		CDNTarget: task.CDNTarget,

		CreatedAt: task.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: task.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// applyTaskRequestDefaults 为请求体中未设置的新字段填充默认值
// 保持向后兼容：不传新字段时使用默认值
func applyTaskRequestDefaults(req *CreateTaskRequest) {
	if req.TaskType == "" {
		req.TaskType = string(model.TaskTypePauseDelete)
	}
	if req.RecordType == "" {
		req.RecordType = string(model.RecordTypeA_AAAA)
	}
	if req.SwitchBackPolicy == "" {
		req.SwitchBackPolicy = string(model.SwitchBackAuto)
	}
	if req.FailThresholdType == "" {
		req.FailThresholdType = string(model.FailThresholdCount)
	}
	if req.FailThresholdValue <= 0 {
		req.FailThresholdValue = 1
	}
}

// validateTaskRequest 验证探测任务请求参数
// 返回错误信息，如果验证通过返回空字符串
// pm: 解析池管理器，用于验证池类型匹配（可为 nil，跳过池验证）
func validateTaskRequest(req *CreateTaskRequest, db *gorm.DB, pm pool.PoolManager) string {
	// 验证必填字段非空
	if req.Domain == "" {
		return "域名不能为空"
	}
	if req.SubDomain == "" {
		return "主机记录不能为空"
	}
	if req.ProbeProtocol == "" {
		return "探测协议不能为空"
	}

	// 验证探测协议是否有效（必须是 ICMP/TCP/UDP/HTTP/HTTPS 之一）
	if !prober.IsValidProtocol(prober.ProbeProtocol(req.ProbeProtocol)) {
		return "探测协议无效，必须是 ICMP/TCP/UDP/HTTP/HTTPS 之一"
	}

	// 验证数值参数为正整数
	if req.ProbeIntervalSec <= 0 {
		return "探测周期必须为正整数"
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

	// 验证 credential_id 必须存在
	if req.CredentialID == 0 {
		return "凭证 ID 不能为空"
	}
	var credential model.Credential
	if err := db.First(&credential, req.CredentialID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "指定的凭证不存在"
		}
		return "查询凭证失败"
	}

	// ========== 新增字段验证 ==========

	// 填充默认值
	applyTaskRequestDefaults(req)

	// 验证任务类型
	if !model.IsValidTaskType(req.TaskType) {
		return "任务类型无效，必须是 pause_delete、switch 或 cdn_switch"
	}

	// 验证解析记录类型
	if !model.IsValidRecordType(req.RecordType) {
		return "解析记录类型无效，必须是 A、AAAA、A_AAAA 或 CNAME"
	}

	// 验证回切策略
	if !model.IsValidSwitchBackPolicy(req.SwitchBackPolicy) {
		return "回切策略无效，必须是 auto 或 manual"
	}

	// 验证失败阈值类型
	if !model.IsValidFailThresholdType(req.FailThresholdType) {
		return "失败阈值类型无效，必须是 count 或 percent"
	}

	// 验证失败阈值数值为正整数
	if req.FailThresholdValue <= 0 {
		return "失败阈值数值必须为正整数"
	}

	// cdn_switch 类型任务的专用验证
	if req.TaskType == string(model.TaskTypeCDNSwitch) {
		// 验证关联凭证的 provider_type 为 cloudflare
		if credential.ProviderType != "cloudflare" {
			return "CDN 故障转移仅支持 Cloudflare 服务商"
		}
		// 验证故障转移目标 IP 非空
		if req.CDNTarget == "" {
			return "CDN 故障转移任务必须指定目标 IP"
		}
		// cdn_switch 不需要解析池，跳过后续池验证
		return ""
	}

	// 验证切换类型任务必须关联解析池
	if req.TaskType == string(model.TaskTypeSwitch) && req.PoolID == nil {
		return "切换类型任务必须关联解析池"
	}

	// 如果指定了解析池，验证池是否存在以及池类型与任务类型是否匹配
	if req.PoolID != nil {
		if pm == nil {
			return "系统错误：解析池管理器未初始化"
		}

		poolInfo, err := pm.GetPool(context.Background(), *req.PoolID)
		if err != nil {
			return "指定的解析池不存在"
		}

		// 验证池资源类型与解析记录类型匹配
		// A/AAAA/A_AAAA 记录需要 ip 类型的池，CNAME 记录需要 domain 类型的池
		expectedResourceType := "ip"
		if req.RecordType == string(model.RecordTypeCNAME) {
			expectedResourceType = "domain"
		}

		if poolInfo.ResourceType != expectedResourceType {
			if expectedResourceType == "ip" {
				return "A/AAAA 类型任务需要关联 ip 类型的解析池"
			}
			return "CNAME 类型任务需要关联 domain 类型的解析池"
		}
	}

	return ""
}

// ListTasks 获取探测任务列表
// GET /api/tasks
// 返回所有探测任务（域名、协议、周期、状态）
func (h *TaskHandler) ListTasks(c *gin.Context) {
	var tasks []model.ProbeTask
	if err := h.DB.Order("created_at DESC").Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务列表失败"})
		return
	}

	// 构建响应列表
	resp := make([]TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		resp = append(resp, taskToResponse(task))
	}

	c.JSON(http.StatusOK, resp)
}

// GetTask 获取任务详情
// GET /api/tasks/:id
// 返回指定任务的详细信息
func (h *TaskHandler) GetTask(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// CreateTask 创建探测任务
// POST /api/tasks
// 请求体: CreateTaskRequest
// 创建成功后通知 Scheduler 添加任务
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证参数（包含新字段验证和默认值填充）
	if errMsg := validateTaskRequest(&req, h.DB, h.PoolManager); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// 创建任务记录（包含新字段）
	task := model.ProbeTask{
		CredentialID:     req.CredentialID,
		Domain:           req.Domain,
		SubDomain:        req.SubDomain,
		ProbeProtocol:    req.ProbeProtocol,
		ProbePort:        req.ProbePort,
		ProbeIntervalSec: req.ProbeIntervalSec,
		TimeoutMs:        req.TimeoutMs,
		FailThreshold:    req.FailThreshold,
		RecoverThreshold: req.RecoverThreshold,
		Enabled:          true,

		// 新增字段
		TaskType:           req.TaskType,
		RecordType:         req.RecordType,
		PoolID:             req.PoolID,
		SwitchBackPolicy:   req.SwitchBackPolicy,
		FailThresholdType:  req.FailThresholdType,
		FailThresholdValue: req.FailThresholdValue,

		// CDN故障转移专用字段
		CDNTarget: req.CDNTarget,
	}

	if err := h.DB.Create(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建任务失败"})
		return
	}

	// 通知调度器添加任务（如果调度器不为 nil）
	if h.Scheduler != nil {
		if err := h.Scheduler.AddTask(context.Background(), task); err != nil {
			// 调度器通知失败不影响任务创建，记录日志即可
			c.JSON(http.StatusCreated, gin.H{
				"data":    taskToResponse(task),
				"warning": "任务已创建，但通知调度器失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusCreated, taskToResponse(task))
}

// UpdateTask 更新探测任务
// PUT /api/tasks/:id
// 请求体: CreateTaskRequest
// 更新成功后通知 Scheduler 使用新配置
func (h *TaskHandler) UpdateTask(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 查找任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证参数（包含新字段验证和默认值填充）
	if errMsg := validateTaskRequest(&req, h.DB, h.PoolManager); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// 更新任务字段
	task.CredentialID = req.CredentialID
	task.Domain = req.Domain
	task.SubDomain = req.SubDomain
	task.ProbeProtocol = req.ProbeProtocol
	task.ProbePort = req.ProbePort
	task.ProbeIntervalSec = req.ProbeIntervalSec
	task.TimeoutMs = req.TimeoutMs
	task.FailThreshold = req.FailThreshold
	task.RecoverThreshold = req.RecoverThreshold

	// 更新新增字段
	task.TaskType = req.TaskType
	task.RecordType = req.RecordType
	task.PoolID = req.PoolID
	task.SwitchBackPolicy = req.SwitchBackPolicy
	task.FailThresholdType = req.FailThresholdType
	task.FailThresholdValue = req.FailThresholdValue

	// 更新CDN故障转移专用字段
	task.CDNTarget = req.CDNTarget

	if err := h.DB.Save(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新任务失败"})
		return
	}

	// 通知调度器更新任务（如果调度器不为 nil）
	// 需求 5.3: 修改任务后 Scheduler 在下一周期使用新配置
	if h.Scheduler != nil {
		if err := h.Scheduler.UpdateTask(context.Background(), task); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"data":    taskToResponse(task),
				"warning": "任务已更新，但通知调度器失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// PauseTask 暂停探测任务
// POST /api/tasks/:id/pause
// 暂停任务后通知 Scheduler 停止探测
func (h *TaskHandler) PauseTask(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 查找任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 检查任务是否已经暂停
	if !task.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务已经处于暂停状态"})
		return
	}

	// 更新任务状态为暂停
	task.Enabled = false
	if err := h.DB.Save(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "暂停任务失败"})
		return
	}

	// 通知调度器停止任务（如果调度器不为 nil）
	if h.Scheduler != nil {
		if err := h.Scheduler.RemoveTask(uint(id)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"data":    taskToResponse(task),
				"warning": "任务已暂停，但通知调度器失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// ResumeTask 恢复探测任务
// POST /api/tasks/:id/resume
// 恢复任务后通知 Scheduler 重新启动探测
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 查找任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 检查任务是否已经在运行
	if task.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务已经在运行中"})
		return
	}

	// 更新任务状态为启用
	task.Enabled = true
	if err := h.DB.Save(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "恢复任务失败"})
		return
	}

	// 通知调度器启动任务（如果调度器不为 nil）
	if h.Scheduler != nil {
		if err := h.Scheduler.AddTask(context.Background(), task); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"data":    taskToResponse(task),
				"warning": "任务已恢复，但通知调度器失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, taskToResponse(task))
}

// DeleteTask 删除探测任务
// DELETE /api/tasks/:id
// 删除成功后通知 Scheduler 停止探测并清理缓存
func (h *TaskHandler) DeleteTask(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 查找任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 删除任务
	if err := h.DB.Delete(&task).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除任务失败"})
		return
	}

	// 清理关联的记录切换状态
	h.DB.Where("task_id = ?", id).Delete(&model.RecordSwitchState{})

	// 通知调度器移除任务（如果调度器不为 nil）
	// 需求 5.4: 删除任务后 Scheduler 停止探测并清理缓存
	if h.Scheduler != nil {
		if err := h.Scheduler.RemoveTask(uint(id)); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "任务已删除",
				"warning": "通知调度器失败: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "任务已删除"})
}

// RecordSwitchStateResponse 记录切换状态响应
type RecordSwitchStateResponse struct {
	ID            uint   `json:"id"`
	TaskID        uint   `json:"task_id"`
	RecordID      string `json:"record_id"`
	RecordType    string `json:"record_type"`
	RecordIP      string `json:"record_ip"`
	IsSwitched    bool   `json:"is_switched"`
	OriginalValue string `json:"original_value"`
	CurrentValue  string `json:"current_value"`
	BackupSource  string `json:"backup_source,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// GetRecordSwitchStates 获取任务的记录级别切换状态
// GET /api/tasks/:id/switch-states
// 返回该任务下每条DNS记录的独立切换状态
func (h *TaskHandler) GetRecordSwitchStates(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证任务存在
	var task model.ProbeTask
	if err := h.DB.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 查询记录切换状态
	var states []model.RecordSwitchState
	if err := h.DB.Where("task_id = ?", id).Order("created_at ASC").Find(&states).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询记录切换状态失败"})
		return
	}

	resp := make([]RecordSwitchStateResponse, 0, len(states))
	for _, s := range states {
		resp = append(resp, RecordSwitchStateResponse{
			ID:            s.ID,
			TaskID:        s.TaskID,
			RecordID:      s.RecordID,
			RecordType:    s.RecordType,
			RecordIP:      s.RecordIP,
			IsSwitched:    s.IsSwitched,
			OriginalValue: s.OriginalValue,
			CurrentValue:  s.CurrentValue,
			BackupSource:  s.BackupSource,
			CreatedAt:     s.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:     s.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, resp)
}
