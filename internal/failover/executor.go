// Package failover 故障转移模块
// 实现DNS记录的故障转移执行器，负责切换到备用资源、切换回原始资源以及判断回切条件。
// 集成ResourceSelector选择最优资源，调用DNS Provider更新记录，并记录操作日志。
package failover

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/pool"
	"dns-health-monitor/internal/provider"

	"gorm.io/gorm"
)

// ProviderFactory 创建 DNSProvider 实例的工厂函数类型
type ProviderFactory func(credential model.Credential) (provider.DNSProvider, error)

// ========== 接口定义 ==========

// FailoverExecutor 故障转移执行器接口
// 负责执行DNS记录的切换操作，包括切换到备用资源和切换回原始资源
type FailoverExecutor interface {
	// SwitchToBackup 切换到备用资源（整体切换，兼容CNAME等场景）
	// 1. 调用DNS Provider的UpdateRecordValue更新DNS记录
	// 2. 更新ProbeTask的OriginalValue（保存当前值）、CurrentValue（设为备用值）、IsSwitched = true
	// 3. 记录操作日志到OperationLog表
	SwitchToBackup(ctx context.Context, task *model.ProbeTask, backupValue string) error

	// SwitchBack 切换回原始资源（整体回切，兼容CNAME等场景）
	// 1. 调用DNS Provider的UpdateRecordValue恢复原始值
	// 2. 更新ProbeTask的CurrentValue = OriginalValue、IsSwitched = false
	// 3. 记录操作日志
	SwitchBack(ctx context.Context, task *model.ProbeTask) error

	// ShouldSwitchBack 判断是否应该回切
	// 检查条件：task.IsSwitched && task.SwitchBackPolicy == "auto"
	ShouldSwitchBack(task *model.ProbeTask) bool

	// SwitchRecordToBackup 按单条DNS记录切换到备用资源
	// 用于A/AAAA多条记录场景，只替换异常的那一条记录
	// recordID: 要切换的DNS记录ID
	// originalIP: 该记录的原始IP
	// backupValue: 备用资源值
	SwitchRecordToBackup(ctx context.Context, task *model.ProbeTask, recordID, originalIP, backupValue string) error

	// SwitchRecordBack 按单条DNS记录回切到原始值
	// 用于A/AAAA多条记录场景，只恢复已切换的那一条记录
	SwitchRecordBack(ctx context.Context, task *model.ProbeTask, state *model.RecordSwitchState) error

	// GetRecordSwitchStates 获取任务的所有记录切换状态
	GetRecordSwitchStates(ctx context.Context, taskID uint) ([]model.RecordSwitchState, error)

	// HasAnySwitchedRecord 检查任务是否有任何已切换的记录
	HasAnySwitchedRecord(ctx context.Context, taskID uint) (bool, error)
}

// ========== ShouldSwitchBack 纯函数（导出，便于测试） ==========

// ShouldSwitchBack 判断任务是否应该自动回切
// 纯函数，不依赖任何外部状态，仅根据任务的切换状态和回切策略判断
// 条件：任务已切换到备用资源 且 回切策略为自动回切
func ShouldSwitchBack(task *model.ProbeTask) bool {
	if task == nil {
		return false
	}
	return task.IsSwitched && task.SwitchBackPolicy == string(model.SwitchBackAuto)
}

// ========== 实现 ==========

// failoverExecutorImpl 故障转移执行器的具体实现
type failoverExecutorImpl struct {
	db              *gorm.DB                      // 数据库连接，用于更新任务状态和记录日志
	providerFactory ProviderFactory               // DNS服务商工厂函数，按需创建provider
	selector        pool.ResourceSelector         // 资源选择器，用于从解析池选择最优资源
	providers       map[uint]provider.DNSProvider // credentialID -> provider 缓存
	mu              sync.RWMutex                  // 保护 providers 缓存
}

// NewFailoverExecutor 创建故障转移执行器实例
// db: 数据库连接
// factory: DNS服务商工厂函数，根据凭证按需创建provider
// selector: 资源选择器实例
func NewFailoverExecutor(db *gorm.DB, factory ProviderFactory, selector pool.ResourceSelector) FailoverExecutor {
	return &failoverExecutorImpl{
		db:              db,
		providerFactory: factory,
		selector:        selector,
		providers:       make(map[uint]provider.DNSProvider),
	}
}

// SwitchToBackup 切换到备用资源
// 执行步骤：
// 1. 根据任务凭证ID获取对应的DNS Provider
// 2. 调用DNS Provider的UpdateRecordValue将DNS记录更新为备用值
// 3. 保存当前值到OriginalValue（仅在首次切换时），设置CurrentValue为备用值，标记IsSwitched为true
// 4. 将任务状态更新持久化到数据库
// 5. 记录操作日志（无论成功或失败）
func (e *failoverExecutorImpl) SwitchToBackup(ctx context.Context, task *model.ProbeTask, backupValue string) error {
	if task == nil {
		return fmt.Errorf("任务不能为空")
	}
	if backupValue == "" {
		return fmt.Errorf("备用资源值不能为空")
	}

	// 获取任务对应的DNS Provider
	prov, err := e.getProvider(task.CredentialID)
	if err != nil {
		e.saveOperationLog(task.ID, "switch_to_backup", "", backupValue, task.RecordType, false,
			fmt.Sprintf("获取DNS Provider失败: %v", err))
		return fmt.Errorf("获取DNS Provider失败: %w", err)
	}

	// 获取当前DNS记录，用于确定recordID
	// A_AAAA 类型需要同时获取 A 和 AAAA 记录
	var records []provider.DNSRecord
	if model.RecordType(task.RecordType) == model.RecordTypeA_AAAA {
		aRecords, _ := prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
		aaaaRecords, _ := prov.ListRecords(ctx, task.Domain, task.SubDomain, "AAAA")
		records = append(aRecords, aaaaRecords...)
	} else {
		var err error
		records, err = prov.ListRecords(ctx, task.Domain, task.SubDomain, task.RecordType)
		if err != nil {
			e.saveOperationLog(task.ID, "switch_to_backup", "", backupValue, task.RecordType, false,
				fmt.Sprintf("获取DNS记录失败: %v", err))
			return fmt.Errorf("获取DNS记录失败: %w", err)
		}
	}

	if len(records) == 0 {
		e.saveOperationLog(task.ID, "switch_to_backup", "", backupValue, task.RecordType, false,
			"未找到匹配的DNS记录")
		return fmt.Errorf("未找到域名 %s.%s 的DNS记录", task.SubDomain, task.Domain)
	}

	// 使用第一条匹配的记录
	record := records[0]

	// 调用DNS Provider更新记录值
	if err := prov.UpdateRecordValue(ctx, record.RecordID, backupValue); err != nil {
		e.saveOperationLog(task.ID, "switch_to_backup", record.RecordID, backupValue, task.RecordType, false,
			fmt.Sprintf("更新DNS记录失败: %v", err))
		return fmt.Errorf("更新DNS记录失败: %w", err)
	}

	// 仅在首次切换时保存原始值（避免多次切换覆盖原始值）
	if !task.IsSwitched {
		task.OriginalValue = record.Value
	}
	task.CurrentValue = backupValue
	task.IsSwitched = true

	// 持久化任务状态到数据库
	if err := e.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"original_value": task.OriginalValue,
		"current_value":  task.CurrentValue,
		"is_switched":    task.IsSwitched,
	}).Error; err != nil {
		log.Printf("更新任务状态失败（任务ID=%d）: %v", task.ID, err)
		// 即使数据库更新失败，DNS记录已经切换，仍然记录日志
		e.saveOperationLog(task.ID, "switch_to_backup", record.RecordID, backupValue, task.RecordType, true,
			fmt.Sprintf("DNS记录已切换，但任务状态更新失败: %v", err))
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	// 记录成功的操作日志
	detail := fmt.Sprintf("从 %s 切换到备用资源 %s", record.Value, backupValue)
	e.saveOperationLog(task.ID, "switch_to_backup", record.RecordID, backupValue, task.RecordType, true, detail)

	log.Printf("任务 %d: 成功切换到备用资源 %s（原始值: %s）", task.ID, backupValue, record.Value)
	return nil
}

// SwitchBack 切换回原始资源
// 执行步骤：
// 1. 验证任务确实处于已切换状态
// 2. 根据任务凭证ID获取对应的DNS Provider
// 3. 调用DNS Provider的UpdateRecordValue将DNS记录恢复为原始值
// 4. 更新CurrentValue为OriginalValue，标记IsSwitched为false
// 5. 将任务状态更新持久化到数据库
// 6. 记录操作日志
func (e *failoverExecutorImpl) SwitchBack(ctx context.Context, task *model.ProbeTask) error {
	if task == nil {
		return fmt.Errorf("任务不能为空")
	}
	if !task.IsSwitched {
		return fmt.Errorf("任务 %d 未处于切换状态，无需回切", task.ID)
	}
	if task.OriginalValue == "" {
		return fmt.Errorf("任务 %d 的原始值为空，无法回切", task.ID)
	}

	// 获取任务对应的DNS Provider
	prov, err := e.getProvider(task.CredentialID)
	if err != nil {
		e.saveOperationLog(task.ID, "switch_back", "", task.OriginalValue, task.RecordType, false,
			fmt.Sprintf("获取DNS Provider失败: %v", err))
		return fmt.Errorf("获取DNS Provider失败: %w", err)
	}

	// 获取当前DNS记录，用于确定recordID
	// A_AAAA 类型需要同时获取 A 和 AAAA 记录
	var records []provider.DNSRecord
	if model.RecordType(task.RecordType) == model.RecordTypeA_AAAA {
		aRecords, _ := prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
		aaaaRecords, _ := prov.ListRecords(ctx, task.Domain, task.SubDomain, "AAAA")
		records = append(aRecords, aaaaRecords...)
	} else {
		var err error
		records, err = prov.ListRecords(ctx, task.Domain, task.SubDomain, task.RecordType)
		if err != nil {
			e.saveOperationLog(task.ID, "switch_back", "", task.OriginalValue, task.RecordType, false,
				fmt.Sprintf("获取DNS记录失败: %v", err))
			return fmt.Errorf("获取DNS记录失败: %w", err)
		}
	}

	if len(records) == 0 {
		e.saveOperationLog(task.ID, "switch_back", "", task.OriginalValue, task.RecordType, false,
			"未找到匹配的DNS记录")
		return fmt.Errorf("未找到域名 %s.%s 的DNS记录", task.SubDomain, task.Domain)
	}

	// 使用第一条匹配的记录
	record := records[0]

	// 调用DNS Provider恢复原始值
	if err := prov.UpdateRecordValue(ctx, record.RecordID, task.OriginalValue); err != nil {
		e.saveOperationLog(task.ID, "switch_back", record.RecordID, task.OriginalValue, task.RecordType, false,
			fmt.Sprintf("恢复DNS记录失败: %v", err))
		return fmt.Errorf("恢复DNS记录失败: %w", err)
	}

	// 保存切换前的备用值，用于日志记录
	previousValue := task.CurrentValue

	// 更新任务状态
	task.CurrentValue = task.OriginalValue
	task.IsSwitched = false

	// 持久化任务状态到数据库
	if err := e.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"current_value": task.CurrentValue,
		"is_switched":   task.IsSwitched,
	}).Error; err != nil {
		log.Printf("更新任务状态失败（任务ID=%d）: %v", task.ID, err)
		e.saveOperationLog(task.ID, "switch_back", record.RecordID, task.OriginalValue, task.RecordType, true,
			fmt.Sprintf("DNS记录已恢复，但任务状态更新失败: %v", err))
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	// 记录成功的操作日志
	detail := fmt.Sprintf("从备用资源 %s 切换回原始值 %s", previousValue, task.OriginalValue)
	e.saveOperationLog(task.ID, "switch_back", record.RecordID, task.OriginalValue, task.RecordType, true, detail)

	log.Printf("任务 %d: 成功切换回原始资源 %s（备用值: %s）", task.ID, task.OriginalValue, previousValue)
	return nil
}

// ShouldSwitchBack 判断是否应该回切
// 委托给导出的纯函数 ShouldSwitchBack
func (e *failoverExecutorImpl) ShouldSwitchBack(task *model.ProbeTask) bool {
	return ShouldSwitchBack(task)
}

// SwitchRecordToBackup 按单条DNS记录切换到备用资源
// 用于A/AAAA多条记录场景，只替换异常的那一条记录，其他记录不受影响
func (e *failoverExecutorImpl) SwitchRecordToBackup(ctx context.Context, task *model.ProbeTask, recordID, originalIP, backupValue string) error {
	if task == nil {
		return fmt.Errorf("任务不能为空")
	}
	if recordID == "" || backupValue == "" {
		return fmt.Errorf("记录ID和备用资源值不能为空")
	}

	// 获取DNS Provider
	prov, err := e.getProvider(task.CredentialID)
	if err != nil {
		e.saveOperationLog(task.ID, "switch_to_backup", recordID, backupValue, task.RecordType, false,
			fmt.Sprintf("获取DNS Provider失败: %v", err))
		return fmt.Errorf("获取DNS Provider失败: %w", err)
	}

	// 调用DNS Provider更新该条记录的值
	if err := prov.UpdateRecordValue(ctx, recordID, backupValue); err != nil {
		e.saveOperationLog(task.ID, "switch_to_backup", recordID, backupValue, task.RecordType, false,
			fmt.Sprintf("更新DNS记录失败: %v", err))
		return fmt.Errorf("更新DNS记录 %s 失败: %w", recordID, err)
	}

	// 创建或更新该记录的切换状态
	var state model.RecordSwitchState
	result := e.db.WithContext(ctx).Where("task_id = ? AND record_id = ?", task.ID, recordID).First(&state)
	if result.Error != nil {
		// 不存在，创建新记录
		state = model.RecordSwitchState{
			TaskID:        task.ID,
			RecordID:      recordID,
			RecordType:    task.RecordType,
			RecordIP:      originalIP,
			IsSwitched:    true,
			OriginalValue: originalIP,
			CurrentValue:  backupValue,
		}
		if err := e.db.WithContext(ctx).Create(&state).Error; err != nil {
			log.Printf("创建记录切换状态失败（任务ID=%d, 记录ID=%s）: %v", task.ID, recordID, err)
		}
	} else {
		// 已存在，更新状态
		if err := e.db.WithContext(ctx).Model(&state).Updates(map[string]interface{}{
			"is_switched":    true,
			"original_value": originalIP,
			"current_value":  backupValue,
		}).Error; err != nil {
			log.Printf("更新记录切换状态失败（任务ID=%d, 记录ID=%s）: %v", task.ID, recordID, err)
		}
	}

	// 同步更新任务级别的切换标记（只要有任何一条记录被切换，任务就标记为已切换）
	if err := e.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"is_switched": true,
	}).Error; err != nil {
		log.Printf("更新任务切换标记失败（任务ID=%d）: %v", task.ID, err)
	}
	task.IsSwitched = true

	// 记录操作日志
	detail := fmt.Sprintf("记录 %s: 从 %s 切换到备用资源 %s", recordID, originalIP, backupValue)
	e.saveOperationLog(task.ID, "switch_to_backup", recordID, backupValue, task.RecordType, true, detail)

	log.Printf("任务 %d: 记录 %s 成功切换到备用资源 %s（原始值: %s）", task.ID, recordID, backupValue, originalIP)
	return nil
}

// SwitchRecordBack 按单条DNS记录回切到原始值
// 用于A/AAAA多条记录场景，只恢复已切换的那一条记录
func (e *failoverExecutorImpl) SwitchRecordBack(ctx context.Context, task *model.ProbeTask, state *model.RecordSwitchState) error {
	if task == nil || state == nil {
		return fmt.Errorf("任务和切换状态不能为空")
	}
	if !state.IsSwitched {
		return fmt.Errorf("记录 %s 未处于切换状态，无需回切", state.RecordID)
	}
	if state.OriginalValue == "" {
		return fmt.Errorf("记录 %s 的原始值为空，无法回切", state.RecordID)
	}

	// 获取DNS Provider
	prov, err := e.getProvider(task.CredentialID)
	if err != nil {
		e.saveOperationLog(task.ID, "switch_back", state.RecordID, state.OriginalValue, state.RecordType, false,
			fmt.Sprintf("获取DNS Provider失败: %v", err))
		return fmt.Errorf("获取DNS Provider失败: %w", err)
	}

	// 需要找到当前记录的实际 RecordID（因为切换后记录ID可能变化）
	// 先尝试用原始 RecordID 更新
	if err := prov.UpdateRecordValue(ctx, state.RecordID, state.OriginalValue); err != nil {
		e.saveOperationLog(task.ID, "switch_back", state.RecordID, state.OriginalValue, state.RecordType, false,
			fmt.Sprintf("恢复DNS记录失败: %v", err))
		return fmt.Errorf("恢复DNS记录 %s 失败: %w", state.RecordID, err)
	}

	previousValue := state.CurrentValue

	// 更新切换状态
	if err := e.db.WithContext(ctx).Model(state).Updates(map[string]interface{}{
		"is_switched":   false,
		"current_value": state.OriginalValue,
	}).Error; err != nil {
		log.Printf("更新记录切换状态失败（任务ID=%d, 记录ID=%s）: %v", task.ID, state.RecordID, err)
	}
	state.IsSwitched = false
	state.CurrentValue = state.OriginalValue

	// 检查是否还有其他已切换的记录，如果没有则清除任务级别的切换标记
	hasSwitched, _ := e.HasAnySwitchedRecord(ctx, task.ID)
	if !hasSwitched {
		if err := e.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
			"is_switched": false,
		}).Error; err != nil {
			log.Printf("更新任务切换标记失败（任务ID=%d）: %v", task.ID, err)
		}
		task.IsSwitched = false
	}

	// 记录操作日志
	detail := fmt.Sprintf("记录 %s: 从备用资源 %s 切换回原始值 %s", state.RecordID, previousValue, state.OriginalValue)
	e.saveOperationLog(task.ID, "switch_back", state.RecordID, state.OriginalValue, state.RecordType, true, detail)

	log.Printf("任务 %d: 记录 %s 成功回切到原始值 %s（备用值: %s）", task.ID, state.RecordID, state.OriginalValue, previousValue)
	return nil
}

// GetRecordSwitchStates 获取任务的所有记录切换状态
func (e *failoverExecutorImpl) GetRecordSwitchStates(ctx context.Context, taskID uint) ([]model.RecordSwitchState, error) {
	var states []model.RecordSwitchState
	if err := e.db.WithContext(ctx).Where("task_id = ?", taskID).Find(&states).Error; err != nil {
		return nil, fmt.Errorf("查询记录切换状态失败: %w", err)
	}
	return states, nil
}

// HasAnySwitchedRecord 检查任务是否有任何已切换的记录
func (e *failoverExecutorImpl) HasAnySwitchedRecord(ctx context.Context, taskID uint) (bool, error) {
	var count int64
	if err := e.db.WithContext(ctx).Model(&model.RecordSwitchState{}).
		Where("task_id = ? AND is_switched = ?", taskID, true).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ========== 内部辅助方法 ==========

// getProvider 根据凭证ID获取或创建DNS Provider实例
// 使用缓存避免重复创建，线程安全
func (e *failoverExecutorImpl) getProvider(credentialID uint) (provider.DNSProvider, error) {
	// 先尝试从缓存读取
	e.mu.RLock()
	if prov, exists := e.providers[credentialID]; exists {
		e.mu.RUnlock()
		return prov, nil
	}
	e.mu.RUnlock()

	// 从数据库加载凭证
	var credential model.Credential
	if err := e.db.First(&credential, credentialID).Error; err != nil {
		return nil, fmt.Errorf("查询凭证 %d 失败: %w", credentialID, err)
	}

	// 使用工厂函数创建 provider
	prov, err := e.providerFactory(credential)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}

	// 缓存 provider
	e.mu.Lock()
	e.providers[credentialID] = prov
	e.mu.Unlock()

	return prov, nil
}

// saveOperationLog 记录操作日志到OperationLog表
// 参数说明：
// - taskID: 任务ID
// - opType: 操作类型（switch_to_backup / switch_back）
// - recordID: DNS记录ID
// - ip: 目标IP或域名
// - recordType: 记录类型（A / AAAA / CNAME）
// - success: 操作是否成功
// - detail: 操作详情
func (e *failoverExecutorImpl) saveOperationLog(taskID uint, opType, recordID, ip, recordType string, success bool, detail string) {
	logEntry := model.OperationLog{
		TaskID:        taskID,
		OperationType: opType,
		RecordID:      recordID,
		IP:            ip,
		RecordType:    recordType,
		Success:       success,
		Detail:        detail,
		OperatedAt:    time.Now(),
	}

	if err := e.db.Create(&logEntry).Error; err != nil {
		log.Printf("保存操作日志失败（任务ID=%d, 操作=%s）: %v", taskID, opType, err)
	}
}
