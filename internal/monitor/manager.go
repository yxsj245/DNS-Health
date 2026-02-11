// Package monitor 健康监控管理器
// 提供健康监控任务的CRUD操作和生命周期管理
package monitor

import (
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/prober"
	"fmt"

	"gorm.io/gorm"
)

// HealthMonitorManager 健康监控管理器接口
// 定义健康监控任务的所有管理操作
type HealthMonitorManager interface {
	// CreateTask 创建监控任务
	CreateTask(task *model.HealthMonitorTask) error

	// UpdateTask 更新监控任务配置
	UpdateTask(id uint, updates map[string]interface{}) error

	// DeleteTask 删除监控任务及其所有关联数据
	DeleteTask(id uint) error

	// PauseTask 暂停监控任务
	PauseTask(id uint) error

	// ResumeTask 恢复监控任务
	ResumeTask(id uint) error

	// GetTask 获取任务详情
	GetTask(id uint) (*model.HealthMonitorTask, error)

	// ListTasks 获取所有任务列表
	ListTasks() ([]*model.HealthMonitorTask, error)

	// GetTaskTargets 获取任务的所有监控目标
	GetTaskTargets(taskID uint) ([]*model.HealthMonitorTarget, error)
}

// healthMonitorManagerImpl 健康监控管理器的GORM实现
type healthMonitorManagerImpl struct {
	db *gorm.DB
}

// NewHealthMonitorManager 创建健康监控管理器实例
func NewHealthMonitorManager(db *gorm.DB) HealthMonitorManager {
	return &healthMonitorManagerImpl{db: db}
}

// validateTask 验证监控任务的必填字段和配置有效性
// 需求 1.1: 验证所有必填字段是否完整
func validateTask(task *model.HealthMonitorTask) error {
	if task.Domain == "" {
		return fmt.Errorf("域名不能为空")
	}
	if task.SubDomain == "" {
		return fmt.Errorf("主机记录不能为空")
	}

	// 凭证为可选项，不选择凭证时直接DNS解析

	// 需求 1.2: 验证记录类型
	if !model.IsValidRecordType(task.RecordType) {
		return fmt.Errorf("记录类型无效，必须是 A、AAAA、A_AAAA 或 CNAME")
	}

	// 需求 1.4: 验证探测协议
	if !prober.IsValidProtocol(prober.ProbeProtocol(task.ProbeProtocol)) {
		return fmt.Errorf("探测协议无效，必须是 ICMP、TCP、UDP、HTTP 或 HTTPS")
	}

	// 需求 1.5: 探测间隔必须大于0
	if task.ProbeIntervalSec <= 0 {
		return fmt.Errorf("探测间隔必须为正整数")
	}

	// 需求 1.6: 超时时间必须大于0
	if task.TimeoutMs <= 0 {
		return fmt.Errorf("超时时间必须为正整数")
	}

	// 需求 1.7: 失败阈值必须大于0
	if task.FailThreshold <= 0 {
		return fmt.Errorf("失败阈值必须为正整数")
	}

	// 需求 1.8: 恢复阈值必须大于0
	if task.RecoverThreshold <= 0 {
		return fmt.Errorf("恢复阈值必须为正整数")
	}

	// CNAME专用字段验证
	if task.FailThresholdType != "" && !model.IsValidFailThresholdType(task.FailThresholdType) {
		return fmt.Errorf("失败阈值类型无效，必须是 count 或 percent")
	}

	return nil
}

// validateUpdates 验证更新字段的有效性
// 需求 7.4: 验证新配置并应用更改
func validateUpdates(updates map[string]interface{}) error {
	// 验证记录类型
	if v, ok := updates["record_type"]; ok {
		if s, ok := v.(string); ok && !model.IsValidRecordType(s) {
			return fmt.Errorf("记录类型无效，必须是 A、AAAA、A_AAAA 或 CNAME")
		}
	}

	// 验证探测协议
	if v, ok := updates["probe_protocol"]; ok {
		if s, ok := v.(string); ok && !prober.IsValidProtocol(prober.ProbeProtocol(s)) {
			return fmt.Errorf("探测协议无效，必须是 ICMP、TCP、UDP、HTTP 或 HTTPS")
		}
	}

	// 验证数值字段为正整数
	intFields := map[string]string{
		"probe_interval_sec": "探测间隔",
		"timeout_ms":         "超时时间",
		"fail_threshold":     "失败阈值",
		"recover_threshold":  "恢复阈值",
	}
	for field, name := range intFields {
		if v, ok := updates[field]; ok {
			if num, ok := toInt(v); ok && num <= 0 {
				return fmt.Errorf("%s必须为正整数", name)
			}
		}
	}

	// 验证失败阈值类型
	if v, ok := updates["fail_threshold_type"]; ok {
		if s, ok := v.(string); ok && !model.IsValidFailThresholdType(s) {
			return fmt.Errorf("失败阈值类型无效，必须是 count 或 percent")
		}
	}

	return nil
}

// toInt 将interface{}转换为int，支持多种数值类型
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// CreateTask 创建监控任务
// 需求 1.1: 验证必填字段; 需求 1.9: 创建任务并返回ID
func (m *healthMonitorManagerImpl) CreateTask(task *model.HealthMonitorTask) error {
	// 验证必填字段
	if err := validateTask(task); err != nil {
		return err
	}

	// 设置CNAME字段默认值
	if task.FailThresholdType == "" {
		task.FailThresholdType = string(model.FailThresholdCount)
	}
	if task.FailThresholdValue <= 0 {
		task.FailThresholdValue = 1
	}

	// 需求 1.10: 创建时自动启用
	task.Enabled = true

	// 保存到数据库
	if err := m.db.Create(task).Error; err != nil {
		return fmt.Errorf("创建监控任务失败: %w", err)
	}

	return nil
}

// UpdateTask 更新监控任务配置
// 需求 7.4: 验证新配置并应用更改
func (m *healthMonitorManagerImpl) UpdateTask(id uint, updates map[string]interface{}) error {
	// 先检查任务是否存在
	var task model.HealthMonitorTask
	if err := m.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("任务不存在")
		}
		return fmt.Errorf("查询任务失败: %w", err)
	}

	// 验证更新字段
	if err := validateUpdates(updates); err != nil {
		return err
	}

	// 应用更新
	if err := m.db.Model(&task).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新任务失败: %w", err)
	}

	return nil
}

// DeleteTask 删除监控任务及其所有关联数据
// 需求 7.3: 停止任务并删除所有相关数据（targets和results）
func (m *healthMonitorManagerImpl) DeleteTask(id uint) error {
	// 先检查任务是否存在
	var task model.HealthMonitorTask
	if err := m.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("任务不存在")
		}
		return fmt.Errorf("查询任务失败: %w", err)
	}

	// 使用事务删除任务及关联数据
	return m.db.Transaction(func(tx *gorm.DB) error {
		// 删除关联的监控目标
		if err := tx.Where("task_id = ?", id).Delete(&model.HealthMonitorTarget{}).Error; err != nil {
			return fmt.Errorf("删除监控目标失败: %w", err)
		}

		// 删除关联的探测结果
		if err := tx.Where("task_id = ?", id).Delete(&model.HealthMonitorResult{}).Error; err != nil {
			return fmt.Errorf("删除探测结果失败: %w", err)
		}

		// 删除任务本身
		if err := tx.Delete(&task).Error; err != nil {
			return fmt.Errorf("删除任务失败: %w", err)
		}

		return nil
	})
}

// PauseTask 暂停监控任务
// 需求 7.1: 停止该任务的DNS解析和探测
func (m *healthMonitorManagerImpl) PauseTask(id uint) error {
	var task model.HealthMonitorTask
	if err := m.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("任务不存在")
		}
		return fmt.Errorf("查询任务失败: %w", err)
	}

	if !task.Enabled {
		return fmt.Errorf("任务已经处于暂停状态")
	}

	if err := m.db.Model(&task).Update("enabled", false).Error; err != nil {
		return fmt.Errorf("暂停任务失败: %w", err)
	}

	return nil
}

// ResumeTask 恢复监控任务
// 需求 7.2: 重新启动该任务的DNS解析和探测
func (m *healthMonitorManagerImpl) ResumeTask(id uint) error {
	var task model.HealthMonitorTask
	if err := m.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("任务不存在")
		}
		return fmt.Errorf("查询任务失败: %w", err)
	}

	if task.Enabled {
		return fmt.Errorf("任务已经在运行中")
	}

	if err := m.db.Model(&task).Update("enabled", true).Error; err != nil {
		return fmt.Errorf("恢复任务失败: %w", err)
	}

	return nil
}

// GetTask 获取任务详情
// 需求 7.7: 返回任务配置和所有监控IP的健康状态
func (m *healthMonitorManagerImpl) GetTask(id uint) (*model.HealthMonitorTask, error) {
	var task model.HealthMonitorTask
	if err := m.db.First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("任务不存在")
		}
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}

	return &task, nil
}

// ListTasks 获取所有任务列表
// 需求 7.6: 返回所有监控任务及其当前状态
func (m *healthMonitorManagerImpl) ListTasks() ([]*model.HealthMonitorTask, error) {
	var tasks []*model.HealthMonitorTask
	if err := m.db.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("查询任务列表失败: %w", err)
	}

	return tasks, nil
}

// GetTaskTargets 获取任务的所有监控目标
// 需求 7.7: 返回所有监控IP的健康状态
func (m *healthMonitorManagerImpl) GetTaskTargets(taskID uint) ([]*model.HealthMonitorTarget, error) {
	// 先检查任务是否存在
	var task model.HealthMonitorTask
	if err := m.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("任务不存在")
		}
		return nil, fmt.Errorf("查询任务失败: %w", err)
	}

	var targets []*model.HealthMonitorTarget
	if err := m.db.Where("task_id = ?", taskID).Find(&targets).Error; err != nil {
		return nil, fmt.Errorf("查询监控目标失败: %w", err)
	}

	return targets, nil
}
