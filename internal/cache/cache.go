// Package cache 已删除记录缓存模块
// 提供对因探测失败而被删除的 DNS 记录的持久化缓存管理
package cache

import (
	"fmt"
	"time"

	"dns-health-monitor/internal/model"

	"gorm.io/gorm"
)

// DeletedRecordCache 已删除记录缓存
// 基于 GORM 操作 DeletedRecord 表，持久化保存因探测失败被删除的 DNS 记录信息
type DeletedRecordCache struct {
	db *gorm.DB
}

// NewDeletedRecordCache 创建已删除记录缓存实例
func NewDeletedRecordCache(db *gorm.DB) *DeletedRecordCache {
	return &DeletedRecordCache{db: db}
}

// Add 添加已删除记录到缓存
// 如果 DeletedAt 为零值，则自动设置为当前时间
func (c *DeletedRecordCache) Add(record model.DeletedRecord) error {
	if record.DeletedAt.IsZero() {
		record.DeletedAt = time.Now()
	}
	if err := c.db.Create(&record).Error; err != nil {
		return fmt.Errorf("添加已删除记录失败: %w", err)
	}
	return nil
}

// Remove 移除已恢复的记录
// 根据任务 ID 和 IP 地址删除匹配的缓存记录
func (c *DeletedRecordCache) Remove(taskID uint, ip string) error {
	result := c.db.Where("task_id = ? AND ip = ?", taskID, ip).Delete(&model.DeletedRecord{})
	if result.Error != nil {
		return fmt.Errorf("移除已删除记录失败: %w", result.Error)
	}
	return nil
}

// ListByTask 获取某任务下所有已删除记录
func (c *DeletedRecordCache) ListByTask(taskID uint) ([]model.DeletedRecord, error) {
	var records []model.DeletedRecord
	if err := c.db.Where("task_id = ?", taskID).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("查询已删除记录失败: %w", err)
	}
	return records, nil
}

// CleanByTask 清理某任务的所有已删除记录
func (c *DeletedRecordCache) CleanByTask(taskID uint) error {
	result := c.db.Where("task_id = ?", taskID).Delete(&model.DeletedRecord{})
	if result.Error != nil {
		return fmt.Errorf("清理已删除记录失败: %w", result.Error)
	}
	return nil
}
