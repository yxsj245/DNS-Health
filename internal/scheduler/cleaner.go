// Package scheduler 监控调度器模块
// cleaner.go 实现日志清理定时任务，自动清理过期的探测结果和操作日志
package scheduler

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// CleanerConfig 日志清理器配置
type CleanerConfig struct {
	// RetentionDays 数据保留天数，超过此天数的记录将被清理（默认30天）
	RetentionDays int
	// CleanInterval 清理任务执行间隔（默认24小时）
	CleanInterval time.Duration
}

// DefaultCleanerConfig 返回默认的清理器配置
func DefaultCleanerConfig() CleanerConfig {
	return CleanerConfig{
		RetentionDays: 30,
		CleanInterval: 24 * time.Hour,
	}
}

// Cleaner 日志清理器
// 定期清理过期的ProbeResult、PoolProbeResult和OperationLog记录
// 验证需求：10.6（自动清理过期日志）
type Cleaner struct {
	db     *gorm.DB
	config CleanerConfig
}

// NewCleaner 创建日志清理器实例
func NewCleaner(db *gorm.DB, config CleanerConfig) *Cleaner {
	return &Cleaner{
		db:     db,
		config: config,
	}
}

// Start 启动日志清理定时任务
// 在独立的goroutine中运行，按配置的间隔定期执行清理
// 通过context控制生命周期，context取消时自动停止
func (c *Cleaner) Start(ctx context.Context) {
	go c.runCleanLoop(ctx)
	log.Printf("日志清理器已启动（保留期限: %d天，清理间隔: %v）", c.config.RetentionDays, c.config.CleanInterval)
}

// runCleanLoop 清理循环主逻辑
func (c *Cleaner) runCleanLoop(ctx context.Context) {
	// 启动时立即执行一次清理
	c.cleanAll()

	ticker := time.NewTicker(c.config.CleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("日志清理器已停止")
			return
		case <-ticker.C:
			c.cleanAll()
		}
	}
}

// cleanAll 执行所有清理任务
func (c *Cleaner) cleanAll() {
	cutoff := time.Now().AddDate(0, 0, -c.config.RetentionDays)
	log.Printf("开始清理过期日志数据（截止时间: %s）", cutoff.Format("2006-01-02 15:04:05"))

	// 清理过期的ProbeResult记录
	probeCount := c.cleanProbeResults(cutoff)

	// 清理过期的PoolProbeResult记录
	poolProbeCount := c.cleanPoolProbeResults(cutoff)

	// 清理过期的OperationLog记录
	opLogCount := c.cleanOperationLogs(cutoff)

	log.Printf("日志清理完成：ProbeResult=%d条, PoolProbeResult=%d条, OperationLog=%d条",
		probeCount, poolProbeCount, opLogCount)
}

// cleanProbeResults 清理过期的探测结果记录
// 删除probed_at早于截止时间的ProbeResult记录
func (c *Cleaner) cleanProbeResults(cutoff time.Time) int64 {
	result := c.db.Where("probed_at < ?", cutoff).Delete(&probeResultModel{})
	if result.Error != nil {
		log.Printf("清理ProbeResult记录失败: %v", result.Error)
		return 0
	}
	return result.RowsAffected
}

// cleanPoolProbeResults 清理过期的解析池资源探测结果记录
// 删除probed_at早于截止时间的PoolProbeResult记录
func (c *Cleaner) cleanPoolProbeResults(cutoff time.Time) int64 {
	result := c.db.Where("probed_at < ?", cutoff).Delete(&poolProbeResultModel{})
	if result.Error != nil {
		log.Printf("清理PoolProbeResult记录失败: %v", result.Error)
		return 0
	}
	return result.RowsAffected
}

// cleanOperationLogs 清理过期的操作日志记录
// 删除operated_at早于截止时间的OperationLog记录
func (c *Cleaner) cleanOperationLogs(cutoff time.Time) int64 {
	result := c.db.Where("operated_at < ?", cutoff).Delete(&operationLogModel{})
	if result.Error != nil {
		log.Printf("清理OperationLog记录失败: %v", result.Error)
		return 0
	}
	return result.RowsAffected
}

// 以下为GORM模型引用，用于Delete操作时指定表名
// 直接引用model包的类型，避免循环依赖

// probeResultModel 探测结果模型（用于GORM删除操作）
type probeResultModel struct{}

func (probeResultModel) TableName() string { return "probe_results" }

// poolProbeResultModel 解析池探测结果模型（用于GORM删除操作）
type poolProbeResultModel struct{}

func (poolProbeResultModel) TableName() string { return "pool_probe_results" }

// operationLogModel 操作日志模型（用于GORM删除操作）
type operationLogModel struct{}

func (operationLogModel) TableName() string { return "operation_logs" }
