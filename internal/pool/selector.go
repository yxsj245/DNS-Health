// Package pool 解析池管理模块
// selector.go 实现资源选择器，用于从解析池中智能选择最优的备用资源。
// 支持最低延迟优先策略和轮询策略，确保故障转移时选择健康且性能最佳的资源。
package pool

import (
	"context"
	"errors"
	"sync"

	"dns-health-monitor/internal/model"

	"gorm.io/gorm"
)

// ========== 错误定义 ==========

var (
	// ErrNoHealthyResource 没有可用的健康资源
	ErrNoHealthyResource = errors.New("解析池中没有可用的健康资源")
	// ErrEmptyResourceList 资源列表为空
	ErrEmptyResourceList = errors.New("资源列表为空")
)

// ========== 接口定义 ==========

// ResourceSelector 资源选择器接口
// 从解析池中选择最优的备用资源，用于故障转移
type ResourceSelector interface {
	// SelectBestResource 从解析池中选择最优资源
	// 仅考虑健康状态的资源，优先选择延迟最低的
	// 返回资源值（IP或域名），如果没有健康资源则返回错误
	SelectBestResource(ctx context.Context, poolID uint) (string, error)

	// SelectBestResourceExcluding 从解析池中选择最优资源，排除已使用的资源
	// excludeValues: 需要排除的资源值列表（已被其他记录使用的备用IP）
	// 确保同一解析池中的资源不会被重复分配给不同的异常记录
	SelectBestResourceExcluding(ctx context.Context, poolID uint, excludeValues []string) (string, error)
}

// SelectionStrategy 选择策略接口
// 定义从健康资源列表中选择一个资源的策略
type SelectionStrategy interface {
	// Select 从健康资源列表中选择一个资源
	// resources 必须是非空的健康资源列表
	Select(resources []model.PoolResource) (*model.PoolResource, error)
}

// ========== 策略实现 ==========

// LowestLatencyStrategy 最低延迟优先策略
// 从健康资源中选择平均延迟最低的资源
type LowestLatencyStrategy struct{}

// Select 选择延迟最低的资源
// 遍历所有健康资源，返回AvgLatencyMs最小的资源
func (s *LowestLatencyStrategy) Select(resources []model.PoolResource) (*model.PoolResource, error) {
	if len(resources) == 0 {
		return nil, ErrEmptyResourceList
	}

	best := &resources[0]
	for i := 1; i < len(resources); i++ {
		if resources[i].AvgLatencyMs < best.AvgLatencyMs {
			best = &resources[i]
		}
	}

	return best, nil
}

// RoundRobinStrategy 轮询策略
// 按顺序循环选择健康资源，确保负载均匀分散
// 使用互斥锁保证线程安全
type RoundRobinStrategy struct {
	lastIndex int
	mu        sync.Mutex
}

// Select 轮询选择下一个资源
// 每次调用返回列表中的下一个资源，到达末尾后从头开始
func (s *RoundRobinStrategy) Select(resources []model.PoolResource) (*model.PoolResource, error) {
	if len(resources) == 0 {
		return nil, ErrEmptyResourceList
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 计算下一个索引，使用取模确保在有效范围内
	index := s.lastIndex % len(resources)
	s.lastIndex = index + 1

	return &resources[index], nil
}

// ========== ResourceSelector 实现 ==========

// resourceSelectorImpl 资源选择器的具体实现
// 组合数据库访问和选择策略，从解析池中选择最优资源
type resourceSelectorImpl struct {
	db       *gorm.DB
	strategy SelectionStrategy
}

// NewResourceSelector 创建资源选择器实例
// 使用最低延迟优先策略作为默认策略
func NewResourceSelector(db *gorm.DB) ResourceSelector {
	return &resourceSelectorImpl{
		db:       db,
		strategy: &LowestLatencyStrategy{},
	}
}

// NewResourceSelectorWithStrategy 创建使用指定策略的资源选择器实例
func NewResourceSelectorWithStrategy(db *gorm.DB, strategy SelectionStrategy) ResourceSelector {
	return &resourceSelectorImpl{
		db:       db,
		strategy: strategy,
	}
}

// SelectBestResource 从解析池中选择最优资源
// 1. 查询解析池中所有健康状态的资源
// 2. 如果没有健康资源，返回 ErrNoHealthyResource
// 3. 使用配置的选择策略从健康资源中选择一个
func (r *resourceSelectorImpl) SelectBestResource(ctx context.Context, poolID uint) (string, error) {
	return r.SelectBestResourceExcluding(ctx, poolID, nil)
}

// SelectBestResourceExcluding 从解析池中选择最优资源，排除已使用的资源
// excludeValues 为需要排除的资源值列表，确保不会重复分配
func (r *resourceSelectorImpl) SelectBestResourceExcluding(ctx context.Context, poolID uint, excludeValues []string) (string, error) {
	// 构建查询：健康状态的资源
	query := r.db.WithContext(ctx).
		Where("pool_id = ? AND health_status = ?", poolID, string(model.HealthStatusHealthy))

	// 如果有需要排除的值，添加 NOT IN 条件
	if len(excludeValues) > 0 {
		query = query.Where("value NOT IN ?", excludeValues)
	}

	var healthyResources []model.PoolResource
	if err := query.Order("avg_latency_ms ASC").Find(&healthyResources).Error; err != nil {
		return "", err
	}

	// 如果没有健康资源，返回错误
	if len(healthyResources) == 0 {
		return "", ErrNoHealthyResource
	}

	// 使用策略选择最优资源
	selected, err := r.strategy.Select(healthyResources)
	if err != nil {
		return "", err
	}

	return selected.Value, nil
}
