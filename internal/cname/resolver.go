// Package cname CNAME解析器模块
// 提供CNAME记录的IP解析、目标列表管理、失败IP统计和阈值计算功能。
// 当任务类型为CNAME时，系统会解析CNAME记录指向的所有IP地址，
// 并根据配置的阈值（个数或百分比）判断是否需要触发故障转移。
package cname

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"

	"dns-health-monitor/internal/model"

	"gorm.io/gorm"
)

// ========== 错误定义 ==========

var (
	// ErrResolveFailed DNS解析失败
	ErrResolveFailed = errors.New("CNAME记录DNS解析失败")
	// ErrTaskNotFound 任务不存在
	ErrTaskNotFound = errors.New("任务不存在")
	// ErrInvalidThresholdType 无效的阈值类型
	ErrInvalidThresholdType = errors.New("无效的阈值类型，必须是 count 或 percent")
)

// ========== 接口定义 ==========

// CNAMEResolver CNAME解析器接口
// 负责解析CNAME记录指向的所有IP地址，管理探测目标列表，
// 统计失败IP数量，并根据配置计算实际失败阈值。
type CNAMEResolver interface {
	// ResolveIPs 解析CNAME记录指向的所有IP
	// domain: 需要解析的域名（如 www.example.com）
	// 返回去重且排序后的IP地址列表
	ResolveIPs(ctx context.Context, domain string) ([]string, error)

	// UpdateTargets 更新任务的CNAME目标列表
	// 对比当前数据库中的IP列表和新解析的IP列表，
	// 添加新出现的IP，删除已消失的IP，保留未变化的IP及其健康状态。
	UpdateTargets(ctx context.Context, taskID uint, ips []string) error

	// GetFailedIPCount 获取失败IP数量
	// 统计指定任务下健康状态为 unhealthy 的CNAME目标数量
	GetFailedIPCount(ctx context.Context, taskID uint) (int, error)

	// CalculateThreshold 计算实际失败阈值
	// 根据任务配置的阈值类型（个数或百分比）和当前IP总数，
	// 计算出实际的失败个数阈值。
	// 对于百分比类型：threshold = ceil(totalIPs * percent / 100)，最小为1
	CalculateThreshold(task *model.ProbeTask, totalIPs int) int
}

// ========== DNS解析函数类型 ==========

// LookupHostFunc DNS解析函数类型，用于依赖注入（方便测试）
type LookupHostFunc func(ctx context.Context, host string) ([]string, error)

// ========== 实现 ==========

// cnameResolverImpl CNAME解析器的具体实现
type cnameResolverImpl struct {
	db         *gorm.DB
	lookupHost LookupHostFunc // DNS解析函数（可注入mock）
}

// NewCNAMEResolver 创建CNAME解析器实例
// db: 数据库连接
func NewCNAMEResolver(db *gorm.DB) CNAMEResolver {
	resolver := &net.Resolver{}
	return &cnameResolverImpl{
		db: db,
		lookupHost: func(ctx context.Context, host string) ([]string, error) {
			return resolver.LookupHost(ctx, host)
		},
	}
}

// NewCNAMEResolverWithLookup 创建CNAME解析器实例（自定义DNS解析函数）
// 主要用于测试场景，可以注入mock的DNS解析函数
func NewCNAMEResolverWithLookup(db *gorm.DB, lookupHost LookupHostFunc) CNAMEResolver {
	return &cnameResolverImpl{
		db:         db,
		lookupHost: lookupHost,
	}
}

// ResolveIPs 解析CNAME记录指向的所有IP
// 使用net.LookupHost解析域名，返回去重且排序后的IP列表。
// 验证需求：3.1 - 解析CNAME记录指向的所有IP地址
func (r *cnameResolverImpl) ResolveIPs(ctx context.Context, domain string) ([]string, error) {
	if domain == "" {
		return nil, fmt.Errorf("%w: 域名不能为空", ErrResolveFailed)
	}

	// 使用DNS解析函数查询域名对应的所有IP
	addrs, err := r.lookupHost(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResolveFailed, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: 域名 '%s' 未解析到任何IP地址", ErrResolveFailed, domain)
	}

	// 去重并排序，确保结果稳定
	ips := deduplicateAndSort(addrs)

	return ips, nil
}

// UpdateTargets 更新任务的CNAME目标列表
// 通过对比当前数据库中的IP列表和新解析的IP列表，实现增量更新：
// - 新出现的IP：添加到CNAMETarget表，初始状态为unknown
// - 已消失的IP：从CNAMETarget表中删除
// - 未变化的IP：保留原有记录及其健康状态
// 验证需求：3.2 - 自动更新探测目标列表
func (r *cnameResolverImpl) UpdateTargets(ctx context.Context, taskID uint, ips []string) error {
	// 查询当前数据库中该任务的所有CNAME目标
	var currentTargets []model.CNAMETarget
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Find(&currentTargets).Error; err != nil {
		return fmt.Errorf("查询当前CNAME目标失败: %w", err)
	}

	// 构建当前IP集合（用于快速查找）
	currentIPSet := make(map[string]bool, len(currentTargets))
	for _, target := range currentTargets {
		currentIPSet[target.IP] = true
	}

	// 构建新IP集合
	newIPSet := make(map[string]bool, len(ips))
	for _, ip := range ips {
		newIPSet[ip] = true
	}

	// 使用事务执行增量更新
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 添加新出现的IP
		for _, ip := range ips {
			if !currentIPSet[ip] {
				target := model.CNAMETarget{
					TaskID:       taskID,
					IP:           ip,
					HealthStatus: string(model.HealthStatusUnknown),
				}
				if err := tx.Create(&target).Error; err != nil {
					return fmt.Errorf("添加CNAME目标 '%s' 失败: %w", ip, err)
				}
			}
		}

		// 2. 删除已消失的IP
		for _, target := range currentTargets {
			if !newIPSet[target.IP] {
				if err := tx.Delete(&target).Error; err != nil {
					return fmt.Errorf("删除CNAME目标 '%s' 失败: %w", target.IP, err)
				}
			}
		}

		return nil
	})
}

// GetFailedIPCount 获取失败IP数量
// 统计指定任务下健康状态为 unhealthy 的CNAME目标数量。
// 验证需求：7.5 - 失败IP数量达到或超过阈值时触发故障转移
func (r *cnameResolverImpl) GetFailedIPCount(ctx context.Context, taskID uint) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.CNAMETarget{}).
		Where("task_id = ? AND health_status = ?", taskID, string(model.HealthStatusUnhealthy)).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("查询失败IP数量失败: %w", err)
	}

	return int(count), nil
}

// CalculateThreshold 计算实际失败阈值
// 根据任务配置的阈值类型和当前IP总数，计算出实际的失败个数阈值。
// - count类型：直接返回配置的阈值数值
// - percent类型：threshold = ceil(totalIPs * percent / 100)，最小为1
// 这是一个纯函数，不依赖外部状态，便于属性测试。
// 验证需求：7.1-7.4
func (r *cnameResolverImpl) CalculateThreshold(task *model.ProbeTask, totalIPs int) int {
	return CalculateThreshold(task.FailThresholdType, task.FailThresholdValue, totalIPs)
}

// ========== 导出的纯函数（便于属性测试） ==========

// CalculateThreshold 计算实际失败阈值（纯函数版本）
// thresholdType: 阈值类型（"count" 或 "percent"）
// thresholdValue: 阈值数值（个数或百分比值）
// totalIPs: 当前IP总数
// 返回计算后的实际失败个数阈值
//
// 计算规则：
// - count类型：直接返回 thresholdValue
// - percent类型：返回 ceil(totalIPs * thresholdValue / 100)，最小为1
// - 如果totalIPs为0，返回0（没有IP则无需阈值判断）
//
// 验证需求：7.1, 7.2, 7.3, 7.4
func CalculateThreshold(thresholdType string, thresholdValue int, totalIPs int) int {
	switch model.FailThresholdType(thresholdType) {
	case model.FailThresholdPercent:
		// 百分比类型：向上取整，确保至少为1
		if totalIPs <= 0 {
			return 0
		}
		threshold := int(math.Ceil(float64(totalIPs) * float64(thresholdValue) / 100.0))
		if threshold < 1 {
			threshold = 1
		}
		return threshold

	case model.FailThresholdCount:
		// 个数类型：直接返回配置值
		return thresholdValue

	default:
		// 未知类型，默认按个数处理
		return thresholdValue
	}
}

// ========== 内部辅助函数 ==========

// deduplicateAndSort 对IP列表去重并排序
// 确保返回结果的稳定性和一致性
func deduplicateAndSort(ips []string) []string {
	seen := make(map[string]bool, len(ips))
	result := make([]string, 0, len(ips))

	for _, ip := range ips {
		if !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}

	// 排序确保结果稳定
	sort.Strings(result)

	return result
}
