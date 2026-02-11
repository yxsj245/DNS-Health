// Package database 数据库初始化与访问
package database

import (
	"fmt"
	"log"

	"dns-health-monitor/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// InitDB 初始化 SQLite 数据库，执行自动迁移。
// 不再创建默认管理员账户，首次使用时由用户自行注册。
// dbPath 为 SQLite 数据库文件路径，使用 ":memory:" 可创建内存数据库（用于测试）。
func InitDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 自动迁移所有数据模型（包括新增的解析池相关表）
	err = db.AutoMigrate(
		&model.User{},
		&model.Credential{},
		&model.ProbeTask{},
		&model.DeletedRecord{},
		&model.ProbeResult{},
		&model.OperationLog{},
		&model.ExcludedIP{},
		// 新增：解析池相关表
		&model.ResolutionPool{},
		&model.PoolResource{},
		&model.PoolProbeResult{},
		&model.CNAMETarget{},
		// 新增：通知模块相关表
		&model.SMTPConfig{},
		&model.NotificationSetting{},
		&model.NotificationLog{},
	)
	if err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 向后兼容：为现有任务设置默认的任务类型和记录类型
	if migrateErr := migrateExistingTasks(db); migrateErr != nil {
		log.Printf("警告：迁移现有任务数据失败: %v", migrateErr)
	}

	// 清理旧的CNAME目标数据（cname_value为空的记录，下次探测会重新创建带关联的记录）
	if migrateErr := cleanOrphanCNAMETargets(db); migrateErr != nil {
		log.Printf("警告：清理旧CNAME目标数据失败: %v", migrateErr)
	}

	return db, nil
}

// migrateExistingTasks 向后兼容迁移：为现有任务设置默认值
// 确保所有没有设置 TaskType 的任务自动标记为 'pause_delete' 类型
func migrateExistingTasks(db *gorm.DB) error {
	// 将所有 task_type 为空的任务设置为 'pause_delete'
	result := db.Model(&model.ProbeTask{}).
		Where("task_type = '' OR task_type IS NULL").
		Updates(map[string]interface{}{
			"task_type":   string(model.TaskTypePauseDelete),
			"record_type": string(model.RecordTypeA),
		})
	if result.Error != nil {
		return fmt.Errorf("更新现有任务类型失败: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Printf("已将 %d 个现有任务标记为 pause_delete 类型", result.RowsAffected)
	}
	return nil
}

// cleanOrphanCNAMETargets 清理旧的CNAME目标数据
// 删除 cname_value 为空的记录，下次探测时会重新创建带 cname_value 关联的记录
func cleanOrphanCNAMETargets(db *gorm.DB) error {
	result := db.Where("cname_value = '' OR cname_value IS NULL").Delete(&model.CNAMETarget{})
	if result.Error != nil {
		return fmt.Errorf("清理旧CNAME目标数据失败: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Printf("已清理 %d 条未关联CNAME记录的旧目标数据", result.RowsAffected)
	}
	return nil
}
