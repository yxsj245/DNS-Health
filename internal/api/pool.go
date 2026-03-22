// Package api 解析池管理 API 接口实现
// 提供解析池的创建、查询、删除以及池中资源的添加、移除、列表等接口
package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/pool"
	"dns-health-monitor/internal/prober"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PoolHandler 解析池管理处理器
type PoolHandler struct {
	PoolManager pool.PoolManager
	PoolProber  *pool.PoolProber // 可选，用于通知探测调度器（测试时可为 nil）
	DB          *gorm.DB         // 数据库连接，用于查询资源使用状态

	// 域名解析缓存：key 为 "poolID:resourceID"，缓存24小时
	resolveCache   map[string]*resolveCacheEntry
	resolveCacheMu sync.RWMutex
}

// resolveCacheEntry 域名解析缓存条目
type resolveCacheEntry struct {
	Domain   string     `json:"domain"`
	IPs      []ipStatus `json:"ips"`
	Error    string     `json:"error,omitempty"`
	CachedAt time.Time  `json:"cached_at"`
}

// ipStatus 单个IP的探测状态
type ipStatus struct {
	IP        string `json:"ip"`
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// NewPoolHandler 创建解析池管理处理器
// pm: 解析池管理器实例
// pp: 解析池探测调度器实例（可为 nil，测试时不需要真正的探测器）
// db: 数据库连接（可为 nil，测试时不需要）
func NewPoolHandler(pm pool.PoolManager, pp *pool.PoolProber, db *gorm.DB) *PoolHandler {
	return &PoolHandler{
		PoolManager:  pm,
		PoolProber:   pp,
		DB:           db,
		resolveCache: make(map[string]*resolveCacheEntry),
	}
}

// ========== 请求/响应结构体 ==========

// CreatePoolRequest 创建解析池请求体
type CreatePoolRequest struct {
	Name             string `json:"name"`               // 池名称（必填）
	ResourceType     string `json:"resource_type"`      // 资源类型: ip / domain（必填）
	Description      string `json:"description"`        // 描述（可选）
	ProbeProtocol    string `json:"probe_protocol"`     // 探测协议（必填）
	ProbePort        int    `json:"probe_port"`         // 探测端口（可选）
	ProbeIntervalSec int    `json:"probe_interval_sec"` // 探测间隔秒数（必填）
	TimeoutMs        int    `json:"timeout_ms"`         // 超时时间毫秒（必填）
	FailThreshold    int    `json:"fail_threshold"`     // 失败阈值（必填）
	RecoverThreshold int    `json:"recover_threshold"`  // 恢复阈值（必填）
}

// PoolResponse 解析池响应
type PoolResponse struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	ResourceType     string `json:"resource_type"`
	Description      string `json:"description"`
	ProbeProtocol    string `json:"probe_protocol"`
	ProbePort        int    `json:"probe_port"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	TimeoutMs        int    `json:"timeout_ms"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// AddResourceRequest 添加资源请求体
type AddResourceRequest struct {
	Value string `json:"value"` // 资源值：IP地址或域名（必填）
}

// BatchAddResourcesRequest 批量添加资源请求体
type BatchAddResourcesRequest struct {
	Values []string `json:"values"` // 资源值列表（必填）
}

// BatchAddResourceResult 单条批量添加结果
type BatchAddResourceResult struct {
	Value   string `json:"value"`             // 资源值
	Success bool   `json:"success"`           // 是否添加成功
	Skipped bool   `json:"skipped,omitempty"` // 是否因重复而跳过
	Error   string `json:"error,omitempty"`   // 错误信息（若失败）
}

// BatchAddResourcesResponse 批量添加资源响应
type BatchAddResourcesResponse struct {
	Total     int                      `json:"total"`      // 总提交数
	Succeeded int                      `json:"succeeded"`  // 成功数
	Skipped   int                      `json:"skipped"`    // 跳过（重复）数
	Failed    int                      `json:"failed"`     // 失败数
	Results   []BatchAddResourceResult `json:"results"`    // 每条结果详情
}

// ResourceResponse 资源响应
type ResourceResponse struct {
	ID                   uint   `json:"id"`
	PoolID               uint   `json:"pool_id"`
	Value                string `json:"value"`
	HealthStatus         string `json:"health_status"`
	ConsecutiveFails     int    `json:"consecutive_fails"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	AvgLatencyMs         int    `json:"avg_latency_ms"`
	Enabled              bool   `json:"enabled"`
	InUse                bool   `json:"in_use"`              // 是否已被故障转移使用
	InUseBy              string `json:"in_use_by,omitempty"` // 使用该资源的任务域名
	LastProbeAt          string `json:"last_probe_at,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

// PoolHealthResponse 解析池健康状态响应
type PoolHealthResponse struct {
	TotalCount     int `json:"total_count"`     // 资源总数
	HealthyCount   int `json:"healthy_count"`   // 健康资源数量
	UnhealthyCount int `json:"unhealthy_count"` // 不健康资源数量
	UnknownCount   int `json:"unknown_count"`   // 未知状态资源数量
	AvgLatencyMs   int `json:"avg_latency_ms"`  // 健康资源的平均延迟（毫秒）
}

// ========== 模型转换 ==========

// poolToResponse 将解析池模型转换为响应 DTO
func poolToResponse(p model.ResolutionPool) PoolResponse {
	return PoolResponse{
		ID:               p.ID,
		Name:             p.Name,
		ResourceType:     p.ResourceType,
		Description:      p.Description,
		ProbeProtocol:    p.ProbeProtocol,
		ProbePort:        p.ProbePort,
		ProbeIntervalSec: p.ProbeIntervalSec,
		TimeoutMs:        p.TimeoutMs,
		FailThreshold:    p.FailThreshold,
		RecoverThreshold: p.RecoverThreshold,
		CreatedAt:        p.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        p.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// resourceToResponse 将资源模型转换为响应 DTO
func resourceToResponse(r model.PoolResource) ResourceResponse {
	resp := ResourceResponse{
		ID:                   r.ID,
		PoolID:               r.PoolID,
		Value:                r.Value,
		HealthStatus:         r.HealthStatus,
		ConsecutiveFails:     r.ConsecutiveFails,
		ConsecutiveSuccesses: r.ConsecutiveSuccesses,
		AvgLatencyMs:         r.AvgLatencyMs,
		Enabled:              r.Enabled,
		CreatedAt:            r.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:            r.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if r.LastProbeAt != nil {
		resp.LastProbeAt = r.LastProbeAt.Format("2006-01-02 15:04:05")
	}
	return resp
}

// ========== 输入验证 ==========

// validateCreatePoolRequest 验证创建解析池请求参数
// 返回错误信息，如果验证通过返回空字符串
func validateCreatePoolRequest(req *CreatePoolRequest) string {
	// 验证池名称不能为空
	if req.Name == "" {
		return "池名称不能为空"
	}

	// 验证资源类型必须是 ip 或 domain
	if req.ResourceType != "ip" && req.ResourceType != "domain" {
		return "资源类型无效，必须是 ip 或 domain"
	}

	// 验证探测协议不能为空且必须有效
	if req.ProbeProtocol == "" {
		return "探测协议不能为空"
	}
	if !prober.IsValidProtocol(prober.ProbeProtocol(req.ProbeProtocol)) {
		return "探测协议无效，必须是 ICMP/TCP/UDP/HTTP/HTTPS 之一"
	}

	// 验证数值参数为正整数
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

	return ""
}

// ========== 解析池 CRUD 接口 ==========

// CreatePool 创建解析池
// POST /api/pools
// 请求体: CreatePoolRequest
func (h *PoolHandler) CreatePool(c *gin.Context) {
	var req CreatePoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证参数
	if errMsg := validateCreatePoolRequest(&req); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}

	// 构建解析池模型
	poolModel := model.ResolutionPool{
		Name:             req.Name,
		ResourceType:     req.ResourceType,
		Description:      req.Description,
		ProbeProtocol:    req.ProbeProtocol,
		ProbePort:        req.ProbePort,
		ProbeIntervalSec: req.ProbeIntervalSec,
		TimeoutMs:        req.TimeoutMs,
		FailThreshold:    req.FailThreshold,
		RecoverThreshold: req.RecoverThreshold,
	}

	// 调用管理器创建解析池
	poolID, err := h.PoolManager.CreatePool(c.Request.Context(), poolModel)
	if err != nil {
		// 检查是否为唯一约束冲突（池名称重复）
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "池名称已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建解析池失败"})
		return
	}

	// 查询创建后的完整记录（包含时间戳）
	createdPool, err := h.PoolManager.GetPool(c.Request.Context(), poolID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询创建的解析池失败"})
		return
	}

	// 通知探测调度器启动该池的探测（如果探测器不为 nil）
	if h.PoolProber != nil {
		_ = h.PoolProber.StartPoolProbing(c.Request.Context(), poolID)
	}

	c.JSON(http.StatusCreated, poolToResponse(*createdPool))
}

// ListPools 获取所有解析池列表
// GET /api/pools
func (h *PoolHandler) ListPools(c *gin.Context) {
	pools, err := h.PoolManager.ListPools(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询解析池列表失败"})
		return
	}

	// 构建响应列表
	resp := make([]PoolResponse, 0, len(pools))
	for _, p := range pools {
		resp = append(resp, poolToResponse(p))
	}

	c.JSON(http.StatusOK, resp)
}

// GetPool 获取解析池详情
// GET /api/pools/:id
func (h *PoolHandler) GetPool(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return // parsePoolID 已经返回了错误响应
	}

	p, err := h.PoolManager.GetPool(c.Request.Context(), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询解析池失败"})
		return
	}

	c.JSON(http.StatusOK, poolToResponse(*p))
}

// DeletePool 删除解析池
// DELETE /api/pools/:id
// 如果有任务引用该池，返回 409 Conflict
func (h *PoolHandler) DeletePool(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	if err := h.PoolManager.DeletePool(c.Request.Context(), poolID); err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		if errors.Is(err, pool.ErrPoolReferenced) {
			c.JSON(http.StatusConflict, gin.H{"error": "解析池正在被任务引用，无法删除"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除解析池失败"})
		return
	}

	// 通知探测调度器停止该池的探测（如果探测器不为 nil）
	if h.PoolProber != nil {
		h.PoolProber.StopPoolProbing(poolID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "解析池已删除"})
}

// ========== 资源管理接口 ==========

// AddResource 向解析池添加资源
// POST /api/pools/:id/resources
// 请求体: AddResourceRequest
func (h *PoolHandler) AddResource(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	var req AddResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证资源值不能为空
	if req.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "资源值不能为空"})
		return
	}

	if err := h.PoolManager.AddResource(c.Request.Context(), poolID, req.Value); err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		if errors.Is(err, pool.ErrInvalidResourceFormat) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, pool.ErrDuplicateResource) {
			c.JSON(http.StatusConflict, gin.H{"error": "该资源已存在于解析池中"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "添加资源失败"})
		return
	}

	// 通知探测调度器启动新资源的探测（如果探测器不为 nil）
	// 需要先查询刚添加的资源ID
	if h.PoolProber != nil {
		resources, err := h.PoolManager.GetPoolResources(c.Request.Context(), poolID)
		if err == nil {
			// 找到刚添加的资源（值匹配的最后一个）
			for i := len(resources) - 1; i >= 0; i-- {
				if resources[i].Value == req.Value {
					_ = h.PoolProber.StartResourceProbing(c.Request.Context(), poolID, resources[i].ID)
					break
				}
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{"message": "资源已添加"})
}

// BatchAddResources 批量向解析池添加资源
// POST /api/pools/:id/resources/batch
// 请求体: BatchAddResourcesRequest
// 返回每条资源的添加结果，重复资源跳过（不报错），格式错误返回具体原因
func (h *PoolHandler) BatchAddResources(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	var req BatchAddResourcesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 验证请求不为空
	if len(req.Values) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "资源列表不能为空"})
		return
	}

	// 限制单次批量最大数量
	const maxBatchSize = 500
	if len(req.Values) > maxBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("单次批量最多添加 %d 条资源", maxBatchSize)})
		return
	}

	results := make([]BatchAddResourceResult, 0, len(req.Values))
	succeeded, skippedCount, failed := 0, 0, 0

	// 逐条添加，统计结果
	for _, v := range req.Values {
		// 跳过空行
		if v == "" {
			continue
		}

		result := BatchAddResourceResult{Value: v}
		addErr := h.PoolManager.AddResource(c.Request.Context(), poolID, v)
		if addErr == nil {
			// 添加成功
			result.Success = true
			succeeded++
		} else if errors.Is(addErr, pool.ErrDuplicateResource) {
			// 已存在，跳过
			result.Success = true
			result.Skipped = true
			skippedCount++
		} else if errors.Is(addErr, pool.ErrPoolNotFound) {
			// 解析池不存在，直接返回错误（不继续处理）
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		} else if errors.Is(addErr, pool.ErrInvalidResourceFormat) {
			// 格式错误
			result.Success = false
			result.Error = addErr.Error()
			failed++
		} else {
			// 其他错误
			result.Success = false
			result.Error = "添加失败"
			failed++
		}
		results = append(results, result)
	}

	// 对成功添加的资源通知探测调度器
	if h.PoolProber != nil && succeeded > skippedCount {
		resources, listErr := h.PoolManager.GetPoolResources(c.Request.Context(), poolID)
		if listErr == nil {
			// 构建已成功添加（非跳过）的资源值集合
			newValues := make(map[string]bool)
			for _, r := range results {
				if r.Success && !r.Skipped {
					newValues[r.Value] = true
				}
			}
			// 对这批新资源逐一启动探测
			for i := len(resources) - 1; i >= 0; i-- {
				if newValues[resources[i].Value] {
					_ = h.PoolProber.StartResourceProbing(c.Request.Context(), poolID, resources[i].ID)
					delete(newValues, resources[i].Value)
					if len(newValues) == 0 {
						break
					}
				}
			}
		}
	}

	resp := BatchAddResourcesResponse{
		Total:     len(results),
		Succeeded: succeeded - skippedCount, // 实际新增数（不含跳过）
		Skipped:   skippedCount,
		Failed:    failed,
		Results:   results,
	}

	c.JSON(http.StatusOK, resp)
}

// RemoveResource 从解析池移除资源
// DELETE /api/pools/:id/resources/:resource_id
func (h *PoolHandler) RemoveResource(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	resourceID, err := parseResourceID(c)
	if err != nil {
		return
	}

	if err := h.PoolManager.RemoveResource(c.Request.Context(), resourceID); err != nil {
		if errors.Is(err, pool.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除资源失败"})
		return
	}

	// 通知探测调度器停止该资源的探测（如果探测器不为 nil）
	if h.PoolProber != nil {
		h.PoolProber.StopResourceProbing(poolID, resourceID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "资源已移除"})
}

// ListResources 列出解析池中的所有资源及健康状态
// GET /api/pools/:id/resources
// 会标记已被故障转移使用的资源
func (h *PoolHandler) ListResources(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	resources, err := h.PoolManager.GetPoolResources(c.Request.Context(), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询资源列表失败"})
		return
	}

	// 查询该解析池中已被故障转移使用的资源值
	// 通过 RecordSwitchState 表关联 ProbeTask 的 pool_id 查询
	usedValueMap := make(map[string]string) // value -> 使用该资源的任务域名
	if h.DB != nil {
		type usedInfo struct {
			CurrentValue string
			SubDomain    string
			Domain       string
		}
		var usedInfos []usedInfo
		h.DB.Raw(`
			SELECT rss.current_value, pt.sub_domain, pt.domain
			FROM record_switch_states rss
			JOIN probe_tasks pt ON rss.task_id = pt.id
			WHERE pt.pool_id = ? AND rss.is_switched = ?
		`, poolID, true).Scan(&usedInfos)

		for _, info := range usedInfos {
			if info.CurrentValue != "" {
				domain := info.Domain
				if info.SubDomain != "" && info.SubDomain != "@" {
					domain = info.SubDomain + "." + info.Domain
				}
				usedValueMap[info.CurrentValue] = domain
			}
		}
	}

	// 构建响应列表，标记已使用的资源
	resp := make([]ResourceResponse, 0, len(resources))
	for _, r := range resources {
		rr := resourceToResponse(r)
		if domain, ok := usedValueMap[r.Value]; ok {
			rr.InUse = true
			rr.InUseBy = domain
		}
		resp = append(resp, rr)
	}

	c.JSON(http.StatusOK, resp)
}

// UpdatePool 更新解析池
// PUT /api/pools/:id
func (h *PoolHandler) UpdatePool(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	var req CreatePoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 查询现有解析池
	existingPool, err := h.PoolManager.GetPool(c.Request.Context(), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询解析池失败"})
		return
	}

	// 验证探测协议
	if req.ProbeProtocol != "" && !prober.IsValidProtocol(prober.ProbeProtocol(req.ProbeProtocol)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "探测协议无效"})
		return
	}

	// 更新字段（名称和资源类型不允许修改）
	existingPool.Description = req.Description
	if req.ProbeProtocol != "" {
		existingPool.ProbeProtocol = req.ProbeProtocol
	}
	existingPool.ProbePort = req.ProbePort
	if req.ProbeIntervalSec > 0 {
		existingPool.ProbeIntervalSec = req.ProbeIntervalSec
	}
	if req.TimeoutMs > 0 {
		existingPool.TimeoutMs = req.TimeoutMs
	}
	if req.FailThreshold > 0 {
		existingPool.FailThreshold = req.FailThreshold
	}
	if req.RecoverThreshold > 0 {
		existingPool.RecoverThreshold = req.RecoverThreshold
	}

	if err := h.PoolManager.UpdatePool(c.Request.Context(), existingPool); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新解析池失败"})
		return
	}

	// 通知探测调度器重启该池的探测（使用新配置）
	if h.PoolProber != nil {
		h.PoolProber.StopPoolProbing(poolID)
		_ = h.PoolProber.StartPoolProbing(c.Request.Context(), poolID)
	}

	c.JSON(http.StatusOK, poolToResponse(*existingPool))
}

// ResolveDomainIPs 解析域名资源的IP列表并探测每个IP的状态
// GET /api/pools/:id/resources/:resource_id/resolve?refresh=true
// 带缓存机制：首次查询执行完整解析+探测，结果缓存24小时，支持 refresh=true 强制刷新
func (h *PoolHandler) ResolveDomainIPs(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	resourceID, err := parseResourceID(c)
	if err != nil {
		return
	}

	// 查询解析池，确认是域名类型
	p, err := h.PoolManager.GetPool(c.Request.Context(), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询解析池失败"})
		return
	}

	if p.ResourceType != "domain" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅域名类型的解析池支持此操作"})
		return
	}

	// 查询资源
	resources, err := h.PoolManager.GetPoolResources(c.Request.Context(), poolID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询资源失败"})
		return
	}

	var targetResource *model.PoolResource
	for i := range resources {
		if resources[i].ID == resourceID {
			targetResource = &resources[i]
			break
		}
	}

	if targetResource == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
		return
	}

	// 检查是否强制刷新
	forceRefresh := c.Query("refresh") == "true"
	cacheKey := fmt.Sprintf("%d:%d", poolID, resourceID)

	// 尝试读取缓存（非强制刷新且缓存未过期时使用）
	if !forceRefresh {
		h.resolveCacheMu.RLock()
		entry, exists := h.resolveCache[cacheKey]
		h.resolveCacheMu.RUnlock()

		if exists && time.Since(entry.CachedAt) < 24*time.Hour {
			c.JSON(http.StatusOK, gin.H{
				"domain":    entry.Domain,
				"ips":       entry.IPs,
				"error":     entry.Error,
				"cached_at": entry.CachedAt.Format("2006-01-02 15:04:05"),
				"cached":    true,
			})
			return
		}
	}

	// 执行完整的多DNS解析
	uniqueIPs := h.resolveFromMultipleDNS(c.Request.Context(), targetResource.Value)

	if len(uniqueIPs) == 0 {
		entry := &resolveCacheEntry{
			Domain:   targetResource.Value,
			IPs:      []ipStatus{},
			Error:    "DNS解析失败: 所有DNS服务器均未解析到IP地址",
			CachedAt: time.Now(),
		}
		h.resolveCacheMu.Lock()
		h.resolveCache[cacheKey] = entry
		h.resolveCacheMu.Unlock()

		c.JSON(http.StatusOK, gin.H{
			"domain":    entry.Domain,
			"ips":       entry.IPs,
			"error":     entry.Error,
			"cached_at": entry.CachedAt.Format("2006-01-02 15:04:05"),
			"cached":    false,
		})
		return
	}

	// 对每个IP进行实时探测
	prob := prober.NewProber(prober.ProbeProtocol(p.ProbeProtocol))
	timeout := time.Duration(p.TimeoutMs) * time.Millisecond

	results := make([]ipStatus, 0, len(uniqueIPs))
	for _, ip := range uniqueIPs {
		item := ipStatus{IP: ip}
		if prob != nil {
			result := prob.Probe(c.Request.Context(), ip, p.ProbePort, timeout)
			item.Success = result.Success
			item.LatencyMs = result.Latency.Milliseconds()
			item.Error = result.Error
		} else {
			item.Error = "不支持的探测协议"
		}
		results = append(results, item)
	}

	// 写入缓存
	entry := &resolveCacheEntry{
		Domain:   targetResource.Value,
		IPs:      results,
		CachedAt: time.Now(),
	}
	h.resolveCacheMu.Lock()
	h.resolveCache[cacheKey] = entry
	h.resolveCacheMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"domain":    entry.Domain,
		"ips":       results,
		"cached_at": entry.CachedAt.Format("2006-01-02 15:04:05"),
		"cached":    false,
	})
}

// resolveFromMultipleDNS 使用多个公共DNS服务器并发解析域名，汇总去重
func (h *PoolHandler) resolveFromMultipleDNS(ctx context.Context, domain string) []string {
	dnsServers := []string{
		"8.8.8.8:53",         // Google
		"8.8.4.4:53",         // Google
		"1.1.1.1:53",         // Cloudflare
		"1.0.0.1:53",         // Cloudflare
		"223.5.5.5:53",       // 阿里DNS
		"223.6.6.6:53",       // 阿里DNS
		"119.29.29.29:53",    // 腾讯DNS
		"114.114.114.114:53", // 114DNS
	}

	type dnsResult struct {
		ips []string
	}

	resultCh := make(chan dnsResult, len(dnsServers))
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	defer queryCancel()

	for _, server := range dnsServers {
		go func(srv string) {
			r := &net.Resolver{
				PreferGo: true,
				Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: 3 * time.Second}
					return d.DialContext(dialCtx, "udp", srv)
				},
			}
			var ips []string
			addrs4, _ := r.LookupIP(queryCtx, "ip4", domain)
			for _, addr := range addrs4 {
				ips = append(ips, addr.String())
			}
			addrs6, _ := r.LookupIP(queryCtx, "ip6", domain)
			for _, addr := range addrs6 {
				ips = append(ips, addr.String())
			}
			resultCh <- dnsResult{ips: ips}
		}(server)
	}

	seen := make(map[string]bool)
	uniqueIPs := make([]string, 0)
dnsLoop:
	for i := 0; i < len(dnsServers); i++ {
		select {
		case res := <-resultCh:
			for _, ip := range res.ips {
				if !seen[ip] {
					seen[ip] = true
					uniqueIPs = append(uniqueIPs, ip)
				}
			}
		case <-queryCtx.Done():
			break dnsLoop
		}
	}

	return uniqueIPs
}

// GetPoolHealth 获取解析池健康状态概览
// GET /api/pools/:id/health
// 返回池中资源的健康统计信息，包括健康/不健康/未知数量和平均延迟
func (h *PoolHandler) GetPoolHealth(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return // parsePoolID 已经返回了错误响应
	}

	// 获取池中所有资源
	resources, err := h.PoolManager.GetPoolResources(c.Request.Context(), poolID)
	if err != nil {
		if errors.Is(err, pool.ErrPoolNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "解析池不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询资源列表失败"})
		return
	}

	// 统计各状态资源数量和计算健康资源的平均延迟
	var healthyCount, unhealthyCount, unknownCount int
	var totalLatency int
	for _, r := range resources {
		switch model.HealthStatus(r.HealthStatus) {
		case model.HealthStatusHealthy:
			healthyCount++
			totalLatency += r.AvgLatencyMs
		case model.HealthStatusUnhealthy:
			unhealthyCount++
		default:
			unknownCount++
		}
	}

	// 计算健康资源的平均延迟
	avgLatency := 0
	if healthyCount > 0 {
		avgLatency = totalLatency / healthyCount
	}

	resp := PoolHealthResponse{
		TotalCount:     len(resources),
		HealthyCount:   healthyCount,
		UnhealthyCount: unhealthyCount,
		UnknownCount:   unknownCount,
		AvgLatencyMs:   avgLatency,
	}

	c.JSON(http.StatusOK, resp)
}

// EnableResource 启用资源探测
// PUT /api/pools/:id/resources/:resource_id/enable
func (h *PoolHandler) EnableResource(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	resourceID, err := parseResourceID(c)
	if err != nil {
		return
	}

	if err := h.PoolManager.EnableResource(c.Request.Context(), poolID, resourceID); err != nil {
		if errors.Is(err, pool.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "启用资源失败"})
		return
	}

	// 通知探测调度器启动该资源的探测
	if h.PoolProber != nil {
		_ = h.PoolProber.StartResourceProbing(c.Request.Context(), poolID, resourceID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "资源已启用"})
}

// DisableResource 禁用资源探测
// PUT /api/pools/:id/resources/:resource_id/disable
func (h *PoolHandler) DisableResource(c *gin.Context) {
	poolID, err := parsePoolID(c)
	if err != nil {
		return
	}

	resourceID, err := parseResourceID(c)
	if err != nil {
		return
	}

	if err := h.PoolManager.DisableResource(c.Request.Context(), poolID, resourceID); err != nil {
		if errors.Is(err, pool.ErrResourceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "禁用资源失败"})
		return
	}

	// 通知探测调度器停止该资源的探测
	if h.PoolProber != nil {
		h.PoolProber.StopResourceProbing(poolID, resourceID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "资源已禁用"})
}

// ========== 辅助函数 ==========

// parsePoolID 从URL参数中解析解析池ID
// 如果解析失败，会直接返回400错误响应
func parsePoolID(c *gin.Context) (uint, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的解析池 ID"})
		return 0, err
	}
	return uint(id), nil
}

// parseResourceID 从URL参数中解析资源ID
// 如果解析失败，会直接返回400错误响应
func parseResourceID(c *gin.Context) (uint, error) {
	idStr := c.Param("resource_id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的资源 ID"})
		return 0, err
	}
	return uint(id), nil
}

// isDuplicateKeyError 检查错误是否为唯一约束冲突
// 支持 SQLite 和 MySQL 的唯一约束错误检测
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	// SQLite: UNIQUE constraint failed
	// MySQL: Duplicate entry
	return contains(errMsg, "UNIQUE constraint failed") ||
		contains(errMsg, "Duplicate entry") ||
		contains(errMsg, "duplicate key")
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

// searchSubstring 在字符串中搜索子串
func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchAt(s, substr, i) {
			return true
		}
	}
	return false
}

// matchAt 检查在指定位置是否匹配子串
func matchAt(s, substr string, pos int) bool {
	for j := 0; j < len(substr); j++ {
		sc := s[pos+j]
		pc := substr[j]
		// 简单的大小写不敏感比较（仅ASCII）
		if sc != pc && sc != pc+32 && sc != pc-32 {
			return false
		}
	}
	return true
}
