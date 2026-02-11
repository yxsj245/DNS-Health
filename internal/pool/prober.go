// Package pool 解析池管理模块
// prober.go 实现解析池资源的探测调度功能。
// 为每个解析池启动独立的探测goroutine，持续监控池中资源的健康状态，
// 并将探测结果记录到PoolProbeResult表中。
package pool

import (
	"context"
	"log"
	"sync"
	"time"

	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/prober"

	"gorm.io/gorm"
)

// PoolProber 解析池资源探测调度器
// 为每个解析池维护独立的探测goroutine，管理资源的健康状态
type PoolProber struct {
	db  *gorm.DB
	mu  sync.RWMutex
	ctx context.Context // 长生命周期的上下文，由 Start 方法设置
	// poolRunners 每个解析池的探测运行器，key为池ID
	poolRunners map[uint]*poolRunner
}

// poolRunner 单个解析池的探测运行器
type poolRunner struct {
	poolID uint
	cancel context.CancelFunc
	// resourceCancels 每个资源的取消函数，key为资源ID
	// 用于在资源被移除时停止对该资源的探测
	resourceCancels map[uint]context.CancelFunc
	mu              sync.Mutex
}

// NewPoolProber 创建解析池探测调度器实例
func NewPoolProber(db *gorm.DB) *PoolProber {
	return &PoolProber{
		db:          db,
		poolRunners: make(map[uint]*poolRunner),
	}
}

// Start 启动探测调度器，加载所有解析池并启动探测
// 在系统启动时调用，恢复所有解析池的探测活动
func (p *PoolProber) Start(ctx context.Context) error {
	// 保存长生命周期的上下文，后续所有探测 goroutine 都从此 context 派生
	p.ctx = ctx

	var pools []model.ResolutionPool
	if err := p.db.Find(&pools).Error; err != nil {
		return err
	}

	log.Printf("解析池探测调度器启动，加载了 %d 个解析池", len(pools))

	for _, pool := range pools {
		if err := p.StartPoolProbing(ctx, pool.ID); err != nil {
			log.Printf("启动解析池 %d (%s) 探测失败: %v", pool.ID, pool.Name, err)
			continue
		}
	}

	return nil
}

// Stop 停止所有解析池的探测
func (p *PoolProber) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, runner := range p.poolRunners {
		runner.cancel()
		log.Printf("已停止解析池 %d 的探测", id)
	}
	p.poolRunners = make(map[uint]*poolRunner)
	log.Println("解析池探测调度器已停止")
}

// StartPoolProbing 启动指定解析池的探测
// 为该池中的每个资源启动独立的探测goroutine
// 注意：传入的 ctx 仅用于数据库查询，探测 goroutine 使用内部长生命周期 context
func (p *PoolProber) StartPoolProbing(_ context.Context, poolID uint) error {
	// 查询解析池配置
	var pool model.ResolutionPool
	if err := p.db.First(&pool, poolID).Error; err != nil {
		return err
	}

	// 查询池中的所有资源
	var resources []model.PoolResource
	if err := p.db.Where("pool_id = ?", poolID).Find(&resources).Error; err != nil {
		return err
	}

	// 使用内部长生命周期 context 派生池级别的上下文
	poolCtx, poolCancel := context.WithCancel(p.ctx)

	runner := &poolRunner{
		poolID:          poolID,
		cancel:          poolCancel,
		resourceCancels: make(map[uint]context.CancelFunc),
	}

	p.mu.Lock()
	// 如果已有运行器，先停止旧的
	if oldRunner, exists := p.poolRunners[poolID]; exists {
		oldRunner.cancel()
	}
	p.poolRunners[poolID] = runner
	p.mu.Unlock()

	// 为每个资源启动探测goroutine
	for _, resource := range resources {
		p.startResourceProbing(poolCtx, runner, &pool, resource)
	}

	log.Printf("已启动解析池 %d (%s) 的探测，共 %d 个资源", poolID, pool.Name, len(resources))
	return nil
}

// StopPoolProbing 停止指定解析池的探测
func (p *PoolProber) StopPoolProbing(poolID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if runner, exists := p.poolRunners[poolID]; exists {
		runner.cancel()
		delete(p.poolRunners, poolID)
		log.Printf("已停止解析池 %d 的探测", poolID)
	}
}

// StartResourceProbing 启动单个资源的探测
// 当新资源被添加到解析池时调用
func (p *PoolProber) StartResourceProbing(_ context.Context, poolID uint, resourceID uint) error {
	// 查询解析池配置
	var pool model.ResolutionPool
	if err := p.db.First(&pool, poolID).Error; err != nil {
		return err
	}

	// 查询资源信息
	var resource model.PoolResource
	if err := p.db.First(&resource, resourceID).Error; err != nil {
		return err
	}

	p.mu.RLock()
	runner, exists := p.poolRunners[poolID]
	p.mu.RUnlock()

	if !exists {
		// 如果池还没有运行器，先启动池的探测
		if err := p.StartPoolProbing(p.ctx, poolID); err != nil {
			return err
		}
		return nil
	}

	// 在现有运行器中启动资源探测，使用内部长生命周期 context
	p.startResourceProbing(p.ctx, runner, &pool, resource)
	return nil
}

// StopResourceProbing 停止单个资源的探测
// 当资源从解析池中移除时调用
func (p *PoolProber) StopResourceProbing(poolID uint, resourceID uint) {
	p.mu.RLock()
	runner, exists := p.poolRunners[poolID]
	p.mu.RUnlock()

	if !exists {
		return
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()

	if cancel, ok := runner.resourceCancels[resourceID]; ok {
		cancel()
		delete(runner.resourceCancels, resourceID)
		log.Printf("已停止解析池 %d 中资源 %d 的探测", poolID, resourceID)
	}
}

// startResourceProbing 启动单个资源的探测goroutine（内部方法）
func (p *PoolProber) startResourceProbing(ctx context.Context, runner *poolRunner, pool *model.ResolutionPool, resource model.PoolResource) {
	// 创建资源级别的上下文
	resourceCtx, resourceCancel := context.WithCancel(ctx)

	runner.mu.Lock()
	// 如果该资源已有探测，先停止旧的
	if oldCancel, exists := runner.resourceCancels[resource.ID]; exists {
		oldCancel()
	}
	runner.resourceCancels[resource.ID] = resourceCancel
	runner.mu.Unlock()

	// 启动探测goroutine
	go p.runResourceProbeLoop(resourceCtx, pool, resource)
}

// runResourceProbeLoop 单个资源的探测循环
func (p *PoolProber) runResourceProbeLoop(ctx context.Context, pool *model.ResolutionPool, resource model.PoolResource) {
	interval := time.Duration(pool.ProbeIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即执行一次探测
	p.probeResource(ctx, pool, resource.ID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("解析池 %d 资源 %d (%s) 探测循环已停止", pool.ID, resource.ID, resource.Value)
			return
		case <-ticker.C:
			p.probeResource(ctx, pool, resource.ID)
		}
	}
}

// probeResource 执行一次资源探测
func (p *PoolProber) probeResource(ctx context.Context, pool *model.ResolutionPool, resourceID uint) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return
	default:
	}

	// 从数据库重新加载资源信息（可能已被更新）
	var resource model.PoolResource
	if err := p.db.First(&resource, resourceID).Error; err != nil {
		log.Printf("解析池 %d: 加载资源 %d 失败: %v", pool.ID, resourceID, err)
		return
	}

	// 创建探测器
	prob := prober.NewProber(prober.ProbeProtocol(pool.ProbeProtocol))
	if prob == nil {
		log.Printf("解析池 %d: 不支持的探测协议 %s", pool.ID, pool.ProbeProtocol)
		return
	}

	timeout := time.Duration(pool.TimeoutMs) * time.Millisecond

	// 执行探测
	result := prob.Probe(ctx, resource.Value, pool.ProbePort, timeout)

	// 记录探测结果到PoolProbeResult表
	probeResult := model.PoolProbeResult{
		ResourceID: resource.ID,
		Success:    result.Success,
		LatencyMs:  int(result.Latency.Milliseconds()),
		ErrorMsg:   result.Error,
		ProbedAt:   result.Time,
	}
	if err := p.db.Create(&probeResult).Error; err != nil {
		log.Printf("解析池 %d: 保存资源 %d 探测结果失败: %v", pool.ID, resource.ID, err)
	}

	// 更新资源健康状态
	p.updateResourceHealth(pool, &resource, result.Success)

	// 计算并更新平均延迟（基于最近10次成功探测）
	p.updateAvgLatency(&resource)

	// 更新最后探测时间
	now := result.Time
	if err := p.db.Model(&resource).Update("last_probe_at", &now).Error; err != nil {
		log.Printf("解析池 %d: 更新资源 %d 最后探测时间失败: %v", pool.ID, resource.ID, err)
	}
}

// UpdateResourceHealth 更新资源健康状态（导出以便测试使用）
// 根据连续失败/成功次数和池的阈值配置，更新资源的健康状态
func UpdateResourceHealth(
	currentStatus string,
	consecutiveFails int,
	consecutiveSuccesses int,
	probeSuccess bool,
	failThreshold int,
	recoverThreshold int,
) (newStatus string, newFails int, newSuccesses int) {
	if probeSuccess {
		newFails = 0
		newSuccesses = consecutiveSuccesses + 1
	} else {
		newFails = consecutiveFails + 1
		newSuccesses = 0
	}

	newStatus = currentStatus

	// 判断是否需要转换健康状态
	switch currentStatus {
	case string(model.HealthStatusHealthy), string(model.HealthStatusUnknown):
		// 健康或未知状态 -> 如果连续失败达到阈值，标记为不健康
		if newFails >= failThreshold {
			newStatus = string(model.HealthStatusUnhealthy)
			log.Printf("资源健康状态变更: %s -> unhealthy (连续失败 %d 次，阈值 %d)",
				currentStatus, newFails, failThreshold)
		}
		// 未知状态下首次成功，标记为健康
		if currentStatus == string(model.HealthStatusUnknown) && probeSuccess {
			newStatus = string(model.HealthStatusHealthy)
		}
	case string(model.HealthStatusUnhealthy):
		// 不健康状态 -> 如果连续成功达到恢复阈值，标记为健康
		if newSuccesses >= recoverThreshold {
			newStatus = string(model.HealthStatusHealthy)
			log.Printf("资源健康状态变更: unhealthy -> healthy (连续成功 %d 次，恢复阈值 %d)",
				newSuccesses, recoverThreshold)
		}
	}

	return newStatus, newFails, newSuccesses
}

// updateResourceHealth 更新资源健康状态（内部方法，操作数据库）
func (p *PoolProber) updateResourceHealth(pool *model.ResolutionPool, resource *model.PoolResource, success bool) {
	newStatus, newFails, newSuccesses := UpdateResourceHealth(
		resource.HealthStatus,
		resource.ConsecutiveFails,
		resource.ConsecutiveSuccesses,
		success,
		pool.FailThreshold,
		pool.RecoverThreshold,
	)

	// 更新数据库
	updates := map[string]interface{}{
		"health_status":         newStatus,
		"consecutive_fails":     newFails,
		"consecutive_successes": newSuccesses,
	}

	if err := p.db.Model(resource).Updates(updates).Error; err != nil {
		log.Printf("解析池 %d: 更新资源 %d 健康状态失败: %v", pool.ID, resource.ID, err)
	}
}

// CalculateAvgLatency 计算资源的平均延迟（导出以便测试使用）
// 基于最近10次成功探测的延迟数据计算平均值
// 如果成功次数少于10次，则使用所有成功探测的平均值
func CalculateAvgLatency(latencies []int) int {
	if len(latencies) == 0 {
		return 0
	}

	sum := 0
	for _, l := range latencies {
		sum += l
	}
	return sum / len(latencies)
}

// updateAvgLatency 计算并更新资源的平均延迟
func (p *PoolProber) updateAvgLatency(resource *model.PoolResource) {
	// 查询最近10次成功探测的延迟
	var latencies []int
	if err := p.db.Model(&model.PoolProbeResult{}).
		Where("resource_id = ? AND success = ?", resource.ID, true).
		Order("probed_at DESC").
		Limit(10).
		Pluck("latency_ms", &latencies).Error; err != nil {
		log.Printf("查询资源 %d 延迟数据失败: %v", resource.ID, err)
		return
	}

	avgLatency := CalculateAvgLatency(latencies)

	// 更新平均延迟
	if err := p.db.Model(resource).Update("avg_latency_ms", avgLatency).Error; err != nil {
		log.Printf("更新资源 %d 平均延迟失败: %v", resource.ID, err)
	}
}
