// Package pool 解析池管理模块
// 提供解析池的CRUD操作、资源管理和格式验证功能。
// 解析池用于存储备用IP或域名资源，在故障转移时提供健康的备用资源。
package pool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"dns-health-monitor/internal/model"

	"gorm.io/gorm"
)

// ========== 错误定义 ==========

var (
	// ErrPoolNotFound 解析池不存在
	ErrPoolNotFound = errors.New("解析池不存在")
	// ErrPoolReferenced 解析池被任务引用，无法删除
	ErrPoolReferenced = errors.New("解析池正在被任务引用，无法删除")
	// ErrResourceNotFound 资源不存在
	ErrResourceNotFound = errors.New("资源不存在")
	// ErrInvalidResourceFormat 资源格式无效
	ErrInvalidResourceFormat = errors.New("资源格式无效")
	// ErrDuplicateResource 资源已存在
	ErrDuplicateResource = errors.New("该资源已存在于解析池中")
	// ErrInvalidResourceType 无效的资源类型
	ErrInvalidResourceType = errors.New("无效的资源类型，必须是 ip 或 domain")
)

// ========== 接口定义 ==========

// PoolManager 解析池管理器接口
// 提供解析池的创建、删除、资源添加和移除等核心管理功能
type PoolManager interface {
	// CreatePool 创建解析池
	// 返回新创建的解析池ID和可能的错误
	CreatePool(ctx context.Context, pool model.ResolutionPool) (uint, error)

	// DeletePool 删除解析池（会检查是否有任务引用）
	// 如果有任务引用该池，返回 ErrPoolReferenced 错误
	DeletePool(ctx context.Context, poolID uint) error

	// AddResource 向解析池添加资源
	// 会根据池的资源类型验证资源格式（IP或域名）
	AddResource(ctx context.Context, poolID uint, value string) error

	// RemoveResource 从解析池移除资源
	RemoveResource(ctx context.Context, resourceID uint) error

	// GetPool 获取解析池详情
	GetPool(ctx context.Context, poolID uint) (*model.ResolutionPool, error)

	// ListPools 获取所有解析池列表
	ListPools(ctx context.Context) ([]model.ResolutionPool, error)

	// GetPoolResources 获取解析池中的所有资源及健康状态
	GetPoolResources(ctx context.Context, poolID uint) ([]model.PoolResource, error)

	// UpdatePool 更新解析池配置
	UpdatePool(ctx context.Context, pool *model.ResolutionPool) error

	// EnableResource 启用资源探测
	EnableResource(ctx context.Context, poolID, resourceID uint) error

	// DisableResource 禁用资源探测
	DisableResource(ctx context.Context, poolID, resourceID uint) error
}

// ========== 实现 ==========

// poolManagerImpl 解析池管理器的具体实现
type poolManagerImpl struct {
	db *gorm.DB
}

// NewPoolManager 创建解析池管理器实例
func NewPoolManager(db *gorm.DB) PoolManager {
	return &poolManagerImpl{db: db}
}

// CreatePool 创建解析池
func (m *poolManagerImpl) CreatePool(ctx context.Context, pool model.ResolutionPool) (uint, error) {
	// 验证资源类型
	if !isValidResourceType(pool.ResourceType) {
		return 0, ErrInvalidResourceType
	}

	// 创建解析池记录
	if err := m.db.WithContext(ctx).Create(&pool).Error; err != nil {
		return 0, fmt.Errorf("创建解析池失败: %w", err)
	}

	return pool.ID, nil
}

// DeletePool 删除解析池（检查引用）
func (m *poolManagerImpl) DeletePool(ctx context.Context, poolID uint) error {
	// 检查解析池是否存在
	var pool model.ResolutionPool
	if err := m.db.WithContext(ctx).First(&pool, poolID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPoolNotFound
		}
		return fmt.Errorf("查询解析池失败: %w", err)
	}

	// 检查是否有任务引用该解析池
	var taskCount int64
	if err := m.db.WithContext(ctx).Model(&model.ProbeTask{}).
		Where("pool_id = ?", poolID).
		Count(&taskCount).Error; err != nil {
		return fmt.Errorf("检查解析池引用失败: %w", err)
	}

	if taskCount > 0 {
		return ErrPoolReferenced
	}

	// 使用事务删除解析池及其关联资源
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先删除关联的探测结果
		var resourceIDs []uint
		if err := tx.Model(&model.PoolResource{}).
			Where("pool_id = ?", poolID).
			Pluck("id", &resourceIDs).Error; err != nil {
			return fmt.Errorf("查询池资源ID失败: %w", err)
		}

		if len(resourceIDs) > 0 {
			// 删除资源的探测结果
			if err := tx.Where("resource_id IN ?", resourceIDs).
				Delete(&model.PoolProbeResult{}).Error; err != nil {
				return fmt.Errorf("删除池资源探测结果失败: %w", err)
			}
		}

		// 删除池中的所有资源
		if err := tx.Where("pool_id = ?", poolID).
			Delete(&model.PoolResource{}).Error; err != nil {
			return fmt.Errorf("删除池资源失败: %w", err)
		}

		// 删除解析池本身
		if err := tx.Delete(&pool).Error; err != nil {
			return fmt.Errorf("删除解析池失败: %w", err)
		}

		return nil
	})
}

// AddResource 向解析池添加资源
func (m *poolManagerImpl) AddResource(ctx context.Context, poolID uint, value string) error {
	// 查询解析池，获取资源类型
	var pool model.ResolutionPool
	if err := m.db.WithContext(ctx).First(&pool, poolID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPoolNotFound
		}
		return fmt.Errorf("查询解析池失败: %w", err)
	}

	// 去除首尾空格
	value = strings.TrimSpace(value)

	// 根据资源类型验证格式
	if err := ValidateResourceFormat(pool.ResourceType, value); err != nil {
		return err
	}

	// 检查资源是否已存在
	var existCount int64
	if err := m.db.WithContext(ctx).Model(&model.PoolResource{}).
		Where("pool_id = ? AND value = ?", poolID, value).
		Count(&existCount).Error; err != nil {
		return fmt.Errorf("检查资源是否存在失败: %w", err)
	}

	if existCount > 0 {
		return ErrDuplicateResource
	}

	// 创建资源记录
	resource := model.PoolResource{
		PoolID:       poolID,
		Value:        value,
		HealthStatus: string(model.HealthStatusUnknown),
	}

	if err := m.db.WithContext(ctx).Create(&resource).Error; err != nil {
		return fmt.Errorf("添加资源失败: %w", err)
	}

	return nil
}

// RemoveResource 从解析池移除资源
func (m *poolManagerImpl) RemoveResource(ctx context.Context, resourceID uint) error {
	// 检查资源是否存在
	var resource model.PoolResource
	if err := m.db.WithContext(ctx).First(&resource, resourceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrResourceNotFound
		}
		return fmt.Errorf("查询资源失败: %w", err)
	}

	// 使用事务删除资源及其探测结果
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 删除资源的探测结果
		if err := tx.Where("resource_id = ?", resourceID).
			Delete(&model.PoolProbeResult{}).Error; err != nil {
			return fmt.Errorf("删除资源探测结果失败: %w", err)
		}

		// 删除资源本身
		if err := tx.Delete(&resource).Error; err != nil {
			return fmt.Errorf("删除资源失败: %w", err)
		}

		return nil
	})
}

// GetPool 获取解析池详情
func (m *poolManagerImpl) GetPool(ctx context.Context, poolID uint) (*model.ResolutionPool, error) {
	var pool model.ResolutionPool
	if err := m.db.WithContext(ctx).First(&pool, poolID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, fmt.Errorf("查询解析池失败: %w", err)
	}
	return &pool, nil
}

// ListPools 获取所有解析池列表
func (m *poolManagerImpl) ListPools(ctx context.Context) ([]model.ResolutionPool, error) {
	var pools []model.ResolutionPool
	if err := m.db.WithContext(ctx).Order("created_at DESC").Find(&pools).Error; err != nil {
		return nil, fmt.Errorf("查询解析池列表失败: %w", err)
	}
	return pools, nil
}

// GetPoolResources 获取解析池中的所有资源及健康状态
func (m *poolManagerImpl) GetPoolResources(ctx context.Context, poolID uint) ([]model.PoolResource, error) {
	// 先检查解析池是否存在
	var pool model.ResolutionPool
	if err := m.db.WithContext(ctx).First(&pool, poolID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPoolNotFound
		}
		return nil, fmt.Errorf("查询解析池失败: %w", err)
	}

	var resources []model.PoolResource
	if err := m.db.WithContext(ctx).
		Where("pool_id = ?", poolID).
		Order("created_at ASC").
		Find(&resources).Error; err != nil {
		return nil, fmt.Errorf("查询池资源失败: %w", err)
	}
	return resources, nil
}

// UpdatePool 更新解析池配置
func (m *poolManagerImpl) UpdatePool(ctx context.Context, p *model.ResolutionPool) error {
	if err := m.db.WithContext(ctx).Save(p).Error; err != nil {
		return fmt.Errorf("更新解析池失败: %w", err)
	}
	return nil
}

// EnableResource 启用资源探测
func (m *poolManagerImpl) EnableResource(ctx context.Context, poolID, resourceID uint) error {
	// 查询资源
	var resource model.PoolResource
	if err := m.db.WithContext(ctx).
		Where("id = ? AND pool_id = ?", resourceID, poolID).
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrResourceNotFound
		}
		return fmt.Errorf("查询资源失败: %w", err)
	}

	// 如果已经启用，直接返回
	if resource.Enabled {
		return nil
	}

	// 更新启用状态
	if err := m.db.WithContext(ctx).Model(&resource).Update("enabled", true).Error; err != nil {
		return fmt.Errorf("启用资源失败: %w", err)
	}

	return nil
}

// DisableResource 禁用资源探测
func (m *poolManagerImpl) DisableResource(ctx context.Context, poolID, resourceID uint) error {
	// 查询资源
	var resource model.PoolResource
	if err := m.db.WithContext(ctx).
		Where("id = ? AND pool_id = ?", resourceID, poolID).
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrResourceNotFound
		}
		return fmt.Errorf("查询资源失败: %w", err)
	}

	// 如果已经禁用，直接返回
	if !resource.Enabled {
		return nil
	}

	// 更新启用状态
	if err := m.db.WithContext(ctx).Model(&resource).Update("enabled", false).Error; err != nil {
		return fmt.Errorf("禁用资源失败: %w", err)
	}

	return nil
}

// ========== 资源格式验证 ==========

// domainRegex 域名格式正则表达式
// 支持标准域名格式，如 example.com、sub.example.com
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// ValidateResourceFormat 验证资源格式
// resourceType: 资源类型（"ip" 或 "domain"）
// value: 资源值（IP地址或域名）
// 返回 nil 表示格式有效，否则返回 ErrInvalidResourceFormat
func ValidateResourceFormat(resourceType, value string) error {
	if value == "" {
		return ErrInvalidResourceFormat
	}

	switch resourceType {
	case "ip":
		return validateIPFormat(value)
	case "domain":
		return validateDomainFormat(value)
	default:
		return ErrInvalidResourceType
	}
}

// validateIPFormat 验证IP地址格式（支持IPv4和IPv6）
func validateIPFormat(value string) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("%w: '%s' 不是有效的IPv4或IPv6地址", ErrInvalidResourceFormat, value)
	}
	return nil
}

// validateDomainFormat 验证域名格式
func validateDomainFormat(value string) error {
	// 去除末尾的点（FQDN格式）
	value = strings.TrimSuffix(value, ".")

	// 域名长度限制
	if len(value) > 253 {
		return fmt.Errorf("%w: 域名长度超过253个字符", ErrInvalidResourceFormat)
	}

	// 使用正则验证域名格式
	if !domainRegex.MatchString(value) {
		return fmt.Errorf("%w: '%s' 不是有效的域名格式", ErrInvalidResourceFormat, value)
	}

	return nil
}

// isValidResourceType 验证资源类型是否有效
func isValidResourceType(t string) bool {
	return t == "ip" || t == "domain"
}
