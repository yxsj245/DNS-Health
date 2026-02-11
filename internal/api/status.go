// Package api 状态与历史查询接口实现
package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"dns-health-monitor/internal/cname"
	"dns-health-monitor/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StatusHandler 状态与历史查询处理器
type StatusHandler struct {
	DB *gorm.DB
}

// NewStatusHandler 创建状态与历史查询处理器
// db: 数据库连接
func NewStatusHandler(db *gorm.DB) *StatusHandler {
	return &StatusHandler{
		DB: db,
	}
}

// ProbeResultResponse 探测结果响应
type ProbeResultResponse struct {
	ID        uint   `json:"id"`
	TaskID    uint   `json:"task_id"`
	IP        string `json:"ip"`
	Success   bool   `json:"success"`
	LatencyMs int    `json:"latency_ms"`
	ErrorMsg  string `json:"error_msg"`
	ProbedAt  string `json:"probed_at"`
}

// OperationLogResponse 操作日志响应
type OperationLogResponse struct {
	ID            uint   `json:"id"`
	TaskID        uint   `json:"task_id"`
	OperationType string `json:"operation_type"`
	RecordID      string `json:"record_id"`
	IP            string `json:"ip"`
	RecordType    string `json:"record_type"`
	Success       bool   `json:"success"`
	Detail        string `json:"detail"`
	OperatedAt    string `json:"operated_at"`
}

// PaginatedResponse 分页响应
type PaginatedResponse struct {
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"page_size"`
	Data  interface{} `json:"data"`
}

// CNAMETargetResponse CNAME目标IP响应
// 展示CNAME记录解析出的每个IP地址及其健康状态
type CNAMETargetResponse struct {
	IP           string `json:"ip"`            // IP地址
	HealthStatus string `json:"health_status"` // 健康状态: healthy / unhealthy / unknown
	LastProbeAt  string `json:"last_probe_at"` // 最后探测时间
}

// CNAMEInfoResponse CNAME信息响应
// 当任务类型为CNAME时，在历史查询响应中附带CNAME相关信息
type CNAMEInfoResponse struct {
	Targets       []CNAMETargetResponse `json:"targets"`         // CNAME目标IP列表及健康状态
	TotalIPCount  int                   `json:"total_ip_count"`  // IP总数
	FailedIPCount int                   `json:"failed_ip_count"` // 失败IP数量
	Threshold     int                   `json:"threshold"`       // 当前计算的失败阈值
}

// HistoryWithCNAMEResponse 带CNAME信息的历史查询响应
// 在分页响应基础上，增加可选的CNAME信息字段
type HistoryWithCNAMEResponse struct {
	Total     int64                 `json:"total"`
	Page      int                   `json:"page"`
	Size      int                   `json:"page_size"`
	Data      []ProbeResultResponse `json:"data"`
	CNAMEInfo *CNAMEInfoResponse    `json:"cname_info,omitempty"` // 仅CNAME类型任务返回
}

// parsePagination 解析分页参数，返回 page 和 pageSize
// 默认 page=1, pageSize=20
func parsePagination(c *gin.Context) (int, int) {
	page := 1
	pageSize := 20

	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 {
		// 限制最大每页数量为 100
		if ps > 100 {
			ps = 100
		}
		pageSize = ps
	}

	return page, pageSize
}

// GetHistory 获取探测历史
// GET /api/tasks/:id/history?ip=xxx&page=1&page_size=20
// 按 probed_at 时间倒序排列，支持 IP 筛选和分页
// 当任务类型为CNAME时，响应中额外包含CNAME目标IP列表、失败IP数量和阈值信息
// 验证需求：3.1, 3.2, 7.5
func (h *StatusHandler) GetHistory(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 解析分页参数
	page, pageSize := parsePagination(c)

	// 构建查询
	query := h.DB.Where("task_id = ?", taskID)

	// 支持 IP 筛选
	if ip := c.Query("ip"); ip != "" {
		query = query.Where("ip = ?", ip)
	}

	// 支持状态筛选（success 参数：true/false）
	if successStr := c.Query("success"); successStr != "" {
		if successStr == "true" {
			query = query.Where("success = ?", true)
		} else if successStr == "false" {
			query = query.Where("success = ?", false)
		}
	}

	// 查询总数
	var total int64
	if err := query.Model(&model.ProbeResult{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询探测历史总数失败"})
		return
	}

	// 按时间倒序查询（需求 6.3）
	var results []model.ProbeResult
	offset := (page - 1) * pageSize
	if err := query.Order("probed_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询探测历史失败"})
		return
	}

	// 构建探测结果响应
	probeResults := make([]ProbeResultResponse, 0, len(results))
	for _, r := range results {
		probeResults = append(probeResults, ProbeResultResponse{
			ID:        r.ID,
			TaskID:    r.TaskID,
			IP:        r.IP,
			Success:   r.Success,
			LatencyMs: r.LatencyMs,
			ErrorMsg:  r.ErrorMsg,
			ProbedAt:  r.ProbedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 判断是否为CNAME类型任务，如果是则附带CNAME信息
	if task.RecordType == string(model.RecordTypeCNAME) {
		cnameInfo := h.buildCNAMEInfo(&task)

		c.JSON(http.StatusOK, HistoryWithCNAMEResponse{
			Total:     total,
			Page:      page,
			Size:      pageSize,
			Data:      probeResults,
			CNAMEInfo: cnameInfo,
		})
		return
	}

	// 非CNAME类型任务，返回标准分页响应（保持向后兼容）
	c.JSON(http.StatusOK, PaginatedResponse{
		Total: total,
		Page:  page,
		Size:  pageSize,
		Data:  probeResults,
	})
}

// GetCNAMEInfo 获取任务的CNAME信息
// GET /api/tasks/:id/cname
func (h *StatusHandler) GetCNAMEInfo(c *gin.Context) {
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 查询任务
	var task model.ProbeTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	// 确认是CNAME类型
	if task.RecordType != string(model.RecordTypeCNAME) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该任务不是CNAME类型"})
		return
	}

	info := h.buildCNAMEInfo(&task)
	c.JSON(http.StatusOK, gin.H{
		"targets":        info.Targets,
		"total_ip_count": info.TotalIPCount,
		"failed_count":   info.FailedIPCount,
		"threshold":      info.Threshold,
		"threshold_type": task.FailThresholdType,
	})
}

// buildCNAMEInfo 构建CNAME信息响应
// 查询该任务关联的所有CNAME目标IP及其健康状态，
// 统计失败IP数量，并计算当前阈值。
// 验证需求：3.1 - 显示CNAME目标IP列表
// 验证需求：7.5 - 显示失败IP数量和阈值
func (h *StatusHandler) buildCNAMEInfo(task *model.ProbeTask) *CNAMEInfoResponse {
	// 查询该任务的所有CNAME目标
	var targets []model.CNAMETarget
	if err := h.DB.Where("task_id = ?", task.ID).
		Order("ip ASC").
		Find(&targets).Error; err != nil {
		// 查询失败时返回空的CNAME信息
		return &CNAMEInfoResponse{
			Targets:       []CNAMETargetResponse{},
			TotalIPCount:  0,
			FailedIPCount: 0,
			Threshold:     0,
		}
	}

	// 构建CNAME目标响应列表
	targetResponses := make([]CNAMETargetResponse, 0, len(targets))
	failedCount := 0
	for _, t := range targets {
		lastProbe := ""
		if t.LastProbeAt != nil {
			lastProbe = t.LastProbeAt.Format("2006-01-02 15:04:05")
		}
		targetResponses = append(targetResponses, CNAMETargetResponse{
			IP:           t.IP,
			HealthStatus: t.HealthStatus,
			LastProbeAt:  lastProbe,
		})
		// 统计失败IP数量
		if t.HealthStatus == string(model.HealthStatusUnhealthy) {
			failedCount++
		}
	}

	// 计算当前阈值
	totalIPs := len(targets)
	threshold := cname.CalculateThreshold(task.FailThresholdType, task.FailThresholdValue, totalIPs)

	return &CNAMEInfoResponse{
		Targets:       targetResponses,
		TotalIPCount:  totalIPs,
		FailedIPCount: failedCount,
		Threshold:     threshold,
	}
}

// applyLogFilters 为操作日志查询应用通用筛选条件
// 支持按操作类型、时间范围、IP、成功状态筛选
// 验证需求：10.3 - 支持按任务ID、操作类型、时间范围进行筛选
func (h *StatusHandler) applyLogFilters(c *gin.Context, query *gorm.DB) *gorm.DB {
	// 支持按操作类型筛选（operation_type 参数）
	if opType := c.Query("operation_type"); opType != "" {
		query = query.Where("operation_type = ?", opType)
	}

	// 支持按时间范围筛选（start_time / end_time 参数，格式：2006-01-02 15:04:05）
	if startTime := c.Query("start_time"); startTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", startTime); err == nil {
			query = query.Where("operated_at >= ?", t)
		}
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", endTime); err == nil {
			query = query.Where("operated_at <= ?", t)
		}
	}

	// 支持 IP 筛选
	if ip := c.Query("ip"); ip != "" {
		query = query.Where("ip = ?", ip)
	}

	// 支持状态筛选（success 参数：true/false）
	if successStr := c.Query("success"); successStr != "" {
		switch successStr {
		case "true":
			query = query.Where("success = ?", true)
		case "false":
			query = query.Where("success = ?", false)
		}
	}

	return query
}

// buildLogResponse 将操作日志模型列表转换为响应列表
func buildLogResponse(logs []model.OperationLog) []OperationLogResponse {
	resp := make([]OperationLogResponse, 0, len(logs))
	for _, l := range logs {
		resp = append(resp, OperationLogResponse{
			ID:            l.ID,
			TaskID:        l.TaskID,
			OperationType: l.OperationType,
			RecordID:      l.RecordID,
			IP:            l.IP,
			RecordType:    l.RecordType,
			Success:       l.Success,
			Detail:        l.Detail,
			OperatedAt:    l.OperatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return resp
}

// GetLogs 获取指定任务的操作日志
// GET /api/tasks/:id/logs?operation_type=xxx&start_time=xxx&end_time=xxx&ip=xxx&success=true&page=1&page_size=20
// 按 operated_at 时间倒序排列，支持按操作类型、时间范围、IP、成功状态筛选和分页
// 验证需求：10.3 - 支持按任务ID、操作类型、时间范围进行筛选
func (h *StatusHandler) GetLogs(c *gin.Context) {
	// 解析任务 ID
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证任务是否存在
	var task model.ProbeTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 解析分页参数
	page, pageSize := parsePagination(c)

	// 构建查询：按任务ID筛选
	query := h.DB.Where("task_id = ?", taskID)

	// 应用通用筛选条件（操作类型、时间范围、IP、成功状态）
	query = h.applyLogFilters(c, query)

	// 查询总数
	var total int64
	if err := query.Model(&model.OperationLog{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询操作日志总数失败"})
		return
	}

	// 按时间倒序查询
	var logs []model.OperationLog
	offset := (page - 1) * pageSize
	if err := query.Order("operated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询操作日志失败"})
		return
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Total: total,
		Page:  page,
		Size:  pageSize,
		Data:  buildLogResponse(logs),
	})
}

// GetAllLogs 获取所有任务的操作日志（全局查询）
// GET /api/logs?task_id=xxx&operation_type=xxx&start_time=xxx&end_time=xxx&ip=xxx&success=true&page=1&page_size=20
// 按 operated_at 时间倒序排列，支持按任务ID、操作类型、时间范围、IP、成功状态筛选和分页
// 验证需求：10.3 - 支持按任务ID、操作类型、时间范围进行筛选
func (h *StatusHandler) GetAllLogs(c *gin.Context) {
	// 解析分页参数
	page, pageSize := parsePagination(c)

	// 构建查询（不限定任务ID，支持跨任务查询）
	query := h.DB.Model(&model.OperationLog{})

	// 支持按任务ID筛选（可选）
	if taskIDStr := c.Query("task_id"); taskIDStr != "" {
		if taskID, err := strconv.ParseUint(taskIDStr, 10, 64); err == nil {
			query = query.Where("task_id = ?", taskID)
		}
	}

	// 应用通用筛选条件（操作类型、时间范围、IP、成功状态）
	query = h.applyLogFilters(c, query)

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询操作日志总数失败"})
		return
	}

	// 按时间倒序查询
	var logs []model.OperationLog
	offset := (page - 1) * pageSize
	if err := query.Order("operated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询操作日志失败"})
		return
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Total: total,
		Page:  page,
		Size:  pageSize,
		Data:  buildLogResponse(logs),
	})
}

// TaskHealthStatus 任务健康状态
type TaskHealthStatus struct {
	TaskID       uint   `json:"task_id"`
	HealthStatus string `json:"health_status"` // "normal" / "abnormal" / "failed" / "unknown"
}

// SystemLogEntry 统一系统日志条目（合并操作日志和通知记录）
type SystemLogEntry struct {
	ID        uint   `json:"id"`
	Source    string `json:"source"` // 日志来源: "operation" / "notification"
	TaskID    uint   `json:"task_id"`
	TaskName  string `json:"task_name"` // 任务名称
	Type      string `json:"type"`      // 操作类型或事件类型
	Success   bool   `json:"success"`
	Detail    string `json:"detail"`
	Extra     string `json:"extra"` // 附加信息（IP、渠道类型等）
	ErrorMsg  string `json:"error_msg"`
	Timestamp string `json:"timestamp"`
}

// DashboardStatsResponse 系统总览统计响应
type DashboardStatsResponse struct {
	TotalTasks    int64 `json:"total_tasks"`
	RunningTasks  int64 `json:"running_tasks"`
	StoppedTasks  int64 `json:"stopped_tasks"`
	TotalProbes   int64 `json:"total_probes"`
	SuccessProbes int64 `json:"success_probes"`
	FailedProbes  int64 `json:"failed_probes"`
	// 健康状态分布
	NormalTasks   int `json:"normal_tasks"`
	AbnormalTasks int `json:"abnormal_tasks"`
	FailedTasks   int `json:"failed_tasks"`
	UnknownTasks  int `json:"unknown_tasks"`
	// 最近系统日志（合并操作日志和通知记录）
	RecentEvents []SystemLogEntry `json:"recent_events"`
}

// getTaskHealthStatus 计算单个任务的健康状态
// 逻辑：取该任务最近一轮探测（同一时间点）的所有 IP 结果
// 全部成功 → normal，部分失败 → abnormal，全部失败 → failed，无记录 → unknown
func (h *StatusHandler) getTaskHealthStatus(taskID uint) string {
	// 获取该任务最近一次探测时间
	var latestResult model.ProbeResult
	if err := h.DB.Where("task_id = ?", taskID).
		Order("probed_at DESC").
		First(&latestResult).Error; err != nil {
		return "unknown"
	}

	// 获取该时间点的所有探测结果
	var results []model.ProbeResult
	if err := h.DB.Where("task_id = ? AND probed_at = ?", taskID, latestResult.ProbedAt).
		Find(&results).Error; err != nil || len(results) == 0 {
		return "unknown"
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	if successCount == len(results) {
		return "normal"
	}
	if successCount == 0 {
		return "failed"
	}
	return "abnormal"
}

// GetDashboardStats 获取系统总览统计数据
// GET /api/dashboard/stats
func (h *StatusHandler) GetDashboardStats(c *gin.Context) {
	var resp DashboardStatsResponse

	// 统计任务数量
	h.DB.Model(&model.ProbeTask{}).Count(&resp.TotalTasks)
	h.DB.Model(&model.ProbeTask{}).Where("enabled = ?", true).Count(&resp.RunningTasks)
	h.DB.Model(&model.ProbeTask{}).Where("enabled = ?", false).Count(&resp.StoppedTasks)

	// 统计探测次数
	h.DB.Model(&model.ProbeResult{}).Count(&resp.TotalProbes)
	h.DB.Model(&model.ProbeResult{}).Where("success = ?", true).Count(&resp.SuccessProbes)
	h.DB.Model(&model.ProbeResult{}).Where("success = ?", false).Count(&resp.FailedProbes)

	// 计算每个任务的健康状态分布
	var tasks []model.ProbeTask
	h.DB.Find(&tasks)
	for _, t := range tasks {
		status := h.getTaskHealthStatus(t.ID)
		switch status {
		case "normal":
			resp.NormalTasks++
		case "abnormal":
			resp.AbnormalTasks++
		case "failed":
			resp.FailedTasks++
		default:
			resp.UnknownTasks++
		}
	}

	// 获取最近系统日志（合并操作日志和通知记录，按时间倒序取最近 10 条）
	resp.RecentEvents = h.getRecentSystemLogs(10)

	c.JSON(http.StatusOK, resp)
}

// getRecentSystemLogs 获取最近的统一系统日志（合并操作日志和通知记录）
// limit: 返回的最大条目数
func (h *StatusHandler) getRecentSystemLogs(limit int) []SystemLogEntry {
	// 构建任务名称映射
	taskNameMap := h.buildTaskNameMap()

	entries := make([]SystemLogEntry, 0, limit*2)

	// 查询最近的操作日志
	var opLogs []model.OperationLog
	h.DB.Order("operated_at DESC").Limit(limit).Find(&opLogs)
	for _, l := range opLogs {
		entries = append(entries, SystemLogEntry{
			ID:        l.ID,
			Source:    "operation",
			TaskID:    l.TaskID,
			TaskName:  taskNameMap[l.TaskID],
			Type:      l.OperationType,
			Success:   l.Success,
			Detail:    l.Detail,
			Extra:     l.IP,
			Timestamp: l.OperatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 查询最近的通知记录
	var notifLogs []model.NotificationLog
	h.DB.Order("sent_at DESC").Limit(limit).Find(&notifLogs)
	for _, l := range notifLogs {
		entries = append(entries, SystemLogEntry{
			ID:        l.ID,
			Source:    "notification",
			TaskID:    l.TaskID,
			TaskName:  taskNameMap[l.TaskID],
			Type:      l.EventType,
			Success:   l.Success,
			Detail:    l.Detail,
			Extra:     l.ChannelType,
			ErrorMsg:  l.ErrorMsg,
			Timestamp: l.SentAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 按时间倒序排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	// 截取前 limit 条
	if len(entries) > limit {
		entries = entries[:limit]
	}

	return entries
}

// buildTaskNameMap 构建任务ID到任务名称的映射
func (h *StatusHandler) buildTaskNameMap() map[uint]string {
	var tasks []model.ProbeTask
	h.DB.Find(&tasks)
	nameMap := make(map[uint]string, len(tasks))
	for _, t := range tasks {
		name := t.Domain
		if t.SubDomain != "" && t.SubDomain != "@" {
			name = t.SubDomain + "." + t.Domain
		}
		nameMap[t.ID] = name
	}
	return nameMap
}

// GetSystemLogs 获取统一系统日志（合并操作日志和通知记录）
// GET /api/system-logs?source=operation|notification&task_id=X&type=Y&success=true|false&start_time=X&end_time=Y&page=1&page_size=20
func (h *StatusHandler) GetSystemLogs(c *gin.Context) {
	page, pageSize := parsePagination(c)
	source := c.Query("source") // "operation" / "notification" / "" (全部)

	taskNameMap := h.buildTaskNameMap()
	entries := make([]SystemLogEntry, 0)
	var totalOp, totalNotif int64

	// 筛选条件
	taskIDStr := c.Query("task_id")
	logType := c.Query("type")
	successStr := c.Query("success")
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	// 查询操作日志
	if source == "" || source == "operation" {
		opQuery := h.DB.Model(&model.OperationLog{})
		if taskIDStr != "" {
			if taskID, err := strconv.ParseUint(taskIDStr, 10, 64); err == nil {
				opQuery = opQuery.Where("task_id = ?", taskID)
			}
		}
		if logType != "" {
			opQuery = opQuery.Where("operation_type = ?", logType)
		}
		if successStr == "true" {
			opQuery = opQuery.Where("success = ?", true)
		} else if successStr == "false" {
			opQuery = opQuery.Where("success = ?", false)
		}
		if startTime != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", startTime); err == nil {
				opQuery = opQuery.Where("operated_at >= ?", t)
			}
		}
		if endTime != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", endTime); err == nil {
				opQuery = opQuery.Where("operated_at <= ?", t)
			}
		}
		opQuery.Count(&totalOp)

		var opLogs []model.OperationLog
		opQuery.Order("operated_at DESC").Find(&opLogs)
		for _, l := range opLogs {
			entries = append(entries, SystemLogEntry{
				ID:        l.ID,
				Source:    "operation",
				TaskID:    l.TaskID,
				TaskName:  taskNameMap[l.TaskID],
				Type:      l.OperationType,
				Success:   l.Success,
				Detail:    l.Detail,
				Extra:     l.IP,
				Timestamp: l.OperatedAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	// 查询通知记录
	if source == "" || source == "notification" {
		notifQuery := h.DB.Model(&model.NotificationLog{})
		if taskIDStr != "" {
			if taskID, err := strconv.ParseUint(taskIDStr, 10, 64); err == nil {
				notifQuery = notifQuery.Where("task_id = ?", taskID)
			}
		}
		if logType != "" {
			notifQuery = notifQuery.Where("event_type = ?", logType)
		}
		if successStr == "true" {
			notifQuery = notifQuery.Where("success = ?", true)
		} else if successStr == "false" {
			notifQuery = notifQuery.Where("success = ?", false)
		}
		if startTime != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", startTime); err == nil {
				notifQuery = notifQuery.Where("sent_at >= ?", t)
			}
		}
		if endTime != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", endTime); err == nil {
				notifQuery = notifQuery.Where("sent_at <= ?", t)
			}
		}
		notifQuery.Count(&totalNotif)

		var notifLogs []model.NotificationLog
		notifQuery.Order("sent_at DESC").Find(&notifLogs)
		for _, l := range notifLogs {
			entries = append(entries, SystemLogEntry{
				ID:        l.ID,
				Source:    "notification",
				TaskID:    l.TaskID,
				TaskName:  taskNameMap[l.TaskID],
				Type:      l.EventType,
				Success:   l.Success,
				Detail:    l.Detail,
				Extra:     l.ChannelType,
				ErrorMsg:  l.ErrorMsg,
				Timestamp: l.SentAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	// 按时间倒序排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	// 手动分页
	total := int64(len(entries))
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if offset > int(total) {
		offset = int(total)
	}
	if end > int(total) {
		end = int(total)
	}
	paged := entries[offset:end]

	c.JSON(http.StatusOK, PaginatedResponse{
		Total: total,
		Page:  page,
		Size:  pageSize,
		Data:  paged,
	})
}

// GetTasksHealthStatus 获取所有任务的健康状态
// GET /api/tasks/health
func (h *StatusHandler) GetTasksHealthStatus(c *gin.Context) {
	var tasks []model.ProbeTask
	if err := h.DB.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务列表失败"})
		return
	}

	result := make(map[uint]string, len(tasks))
	for _, t := range tasks {
		result[t.ID] = h.getTaskHealthStatus(t.ID)
	}

	c.JSON(http.StatusOK, result)
}

// IPStatusResponse 单个 IP 的状态响应
type IPStatusResponse struct {
	IP        string `json:"ip"`
	LastProbe string `json:"last_probe"` // 最近探测时间
	Success   *bool  `json:"success"`    // 最近探测结果（nil 表示无记录）
	LatencyMs int    `json:"latency_ms"` // 最近延迟
	Excluded  bool   `json:"excluded"`   // 是否被排除
}

// GetTaskIPs 获取任务关联的所有 IP 及其探测状态
// GET /api/tasks/:id/ips
func (h *StatusHandler) GetTaskIPs(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 验证任务存在
	var task model.ProbeTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 获取该任务所有出现过的不同 IP（从探测结果中提取）
	var ips []string
	h.DB.Model(&model.ProbeResult{}).
		Where("task_id = ?", taskID).
		Distinct("ip").
		Pluck("ip", &ips)

	// 获取被排除的 IP 列表
	var excludedIPs []model.ExcludedIP
	h.DB.Where("task_id = ?", taskID).Find(&excludedIPs)
	excludedSet := make(map[string]bool, len(excludedIPs))
	for _, e := range excludedIPs {
		excludedSet[e.IP] = true
	}

	// 构建每个 IP 的状态
	result := make([]IPStatusResponse, 0, len(ips))
	for _, ip := range ips {
		item := IPStatusResponse{
			IP:       ip,
			Excluded: excludedSet[ip],
		}

		// 获取该 IP 最近一条探测结果
		var lastResult model.ProbeResult
		if err := h.DB.Where("task_id = ? AND ip = ?", taskID, ip).
			Order("probed_at DESC").
			First(&lastResult).Error; err == nil {
			item.LastProbe = lastResult.ProbedAt.Format("2006-01-02 15:04:05")
			item.Success = &lastResult.Success
			item.LatencyMs = lastResult.LatencyMs
		}

		result = append(result, item)
	}

	c.JSON(http.StatusOK, result)
}

// ExcludeIPRequest 排除/恢复 IP 请求体
type ExcludeIPRequest struct {
	IP string `json:"ip" binding:"required"`
}

// ExcludeIP 排除某个 IP，使其不再纳入探测
// POST /api/tasks/:id/ips/exclude
func (h *StatusHandler) ExcludeIP(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	var req ExcludeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，需要提供 ip 字段"})
		return
	}

	// 验证任务存在
	var task model.ProbeTask
	if err := h.DB.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败"})
		return
	}

	// 检查是否已排除
	var existing model.ExcludedIP
	if err := h.DB.Where("task_id = ? AND ip = ?", taskID, req.IP).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该 IP 已被排除"})
		return
	}

	// 创建排除记录
	excluded := model.ExcludedIP{
		TaskID: uint(taskID),
		IP:     req.IP,
	}
	if err := h.DB.Create(&excluded).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "排除 IP 失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已排除 IP: " + req.IP})
}

// IncludeIP 恢复某个 IP 的探测
// POST /api/tasks/:id/ips/include
func (h *StatusHandler) IncludeIP(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	var req ExcludeIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，需要提供 ip 字段"})
		return
	}

	// 删除排除记录
	result := h.DB.Where("task_id = ? AND ip = ?", taskID, req.IP).Delete(&model.ExcludedIP{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该 IP 未被排除"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已恢复 IP: " + req.IP})
}
