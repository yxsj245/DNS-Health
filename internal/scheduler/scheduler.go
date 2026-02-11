// Package scheduler 监控调度器模块
// 按用户配置的周期调度探测任务，根据探测结果自动暂停/恢复/切换 DNS 记录。
// 支持两种任务类型：
// - pause_delete（暂停/删除）：现有功能，探测失败时暂停或删除DNS记录
// - switch（切换解析）：新功能，探测失败时切换到解析池中的备用资源
package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"dns-health-monitor/internal/cache"
	"dns-health-monitor/internal/cname"
	"dns-health-monitor/internal/failover"
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/notification"
	"dns-health-monitor/internal/pool"
	"dns-health-monitor/internal/prober"
	"dns-health-monitor/internal/provider"
	"dns-health-monitor/internal/retry"

	"gorm.io/gorm"
)

// ProviderFactory 创建 DNSProvider 实例的工厂函数类型
// credentialID 用于查找凭证，db 用于从数据库读取凭证信息
type ProviderFactory func(credential model.Credential) (provider.DNSProvider, error)

// IPCounter IP 探测计数器（导出以便属性测试使用）
type IPCounter struct {
	// ConsecutiveFails 连续失败次数
	ConsecutiveFails int
	// ConsecutiveSuccesses 连续成功次数
	ConsecutiveSuccesses int
	// CurrentStatus 当前状态: "healthy" / "unhealthy" / "paused" / "deleted"
	CurrentStatus string
}

// taskRunner 单个任务的运行器
type taskRunner struct {
	task     model.ProbeTask
	cancel   context.CancelFunc
	counters map[string]*IPCounter // IP -> 连续成功/失败计数
}

// Scheduler 监控调度器
type Scheduler struct {
	db              *gorm.DB
	cache           *cache.DeletedRecordCache
	providerFactory ProviderFactory
	providers       map[uint]provider.DNSProvider // credentialID -> provider
	tasks           map[uint]*taskRunner          // taskID -> runner
	mu              sync.RWMutex

	// 重试配置 - 用于DNS Provider API调用的重试和退避策略
	retryConfig retry.Config

	// 新增依赖 - 用于切换类型任务（可选，nil表示不支持切换功能）
	cnameResolver    cname.CNAMEResolver       // CNAME解析器，用于解析CNAME记录的IP
	failoverExecutor failover.FailoverExecutor // 故障转移执行器，用于切换DNS记录
	poolProber       *pool.PoolProber          // 解析池探测器，用于探测池中资源
	resourceSelector pool.ResourceSelector     // 资源选择器，用于从解析池选择最优资源

	// 通知管理器 - 用于在关键事件发生时发送通知（可选，nil表示不发送通知）
	notificationManager *notification.NotificationManager
}

// SchedulerOption 调度器可选配置函数类型
// 使用函数选项模式，保持NewScheduler向后兼容
type SchedulerOption func(*Scheduler)

// WithCNAMEResolver 设置CNAME解析器
func WithCNAMEResolver(resolver cname.CNAMEResolver) SchedulerOption {
	return func(s *Scheduler) {
		s.cnameResolver = resolver
	}
}

// WithFailoverExecutor 设置故障转移执行器
func WithFailoverExecutor(executor failover.FailoverExecutor) SchedulerOption {
	return func(s *Scheduler) {
		s.failoverExecutor = executor
	}
}

// WithPoolProber 设置解析池探测器
func WithPoolProber(prober *pool.PoolProber) SchedulerOption {
	return func(s *Scheduler) {
		s.poolProber = prober
	}
}

// WithResourceSelector 设置资源选择器
func WithResourceSelector(selector pool.ResourceSelector) SchedulerOption {
	return func(s *Scheduler) {
		s.resourceSelector = selector
	}
}

// WithRetryConfig 设置重试配置
// 用于自定义DNS Provider API调用的重试和退避策略参数
func WithRetryConfig(config retry.Config) SchedulerOption {
	return func(s *Scheduler) {
		s.retryConfig = config
	}
}

// WithNotificationManager 设置通知管理器
// 用于在故障转移、恢复、连续失败等关键事件发生时发送通知
func WithNotificationManager(manager *notification.NotificationManager) SchedulerOption {
	return func(s *Scheduler) {
		s.notificationManager = manager
	}
}

// NewScheduler 创建调度器实例
// opts 为可选配置，用于注入切换类型任务所需的依赖
// 不传入任何选项时，调度器仅支持暂停/删除类型任务（向后兼容）
func NewScheduler(db *gorm.DB, c *cache.DeletedRecordCache, factory ProviderFactory, opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{
		db:              db,
		cache:           c,
		providerFactory: factory,
		providers:       make(map[uint]provider.DNSProvider),
		tasks:           make(map[uint]*taskRunner),
		retryConfig:     retry.DefaultConfig(), // 默认重试配置
	}

	// 应用可选配置
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start 启动调度器，从数据库加载所有已启用的探测任务并启动 goroutine
func (s *Scheduler) Start(ctx context.Context) error {
	var tasks []model.ProbeTask
	if err := s.db.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return fmt.Errorf("加载探测任务失败: %w", err)
	}

	log.Printf("调度器启动，加载了 %d 个探测任务", len(tasks))

	for _, task := range tasks {
		if err := s.startTask(ctx, task); err != nil {
			log.Printf("启动任务 %d (%s) 失败: %v", task.ID, task.Domain, err)
			continue
		}
	}

	return nil
}

// Stop 停止调度器，取消所有运行中的任务
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, runner := range s.tasks {
		runner.cancel()
		log.Printf("已停止任务 %d", id)
	}
	s.tasks = make(map[uint]*taskRunner)
	log.Println("调度器已停止")
}

// AddTask 添加新的探测任务并启动
func (s *Scheduler) AddTask(ctx context.Context, task model.ProbeTask) error {
	s.mu.Lock()
	// 如果任务已存在，先停止旧的
	if runner, exists := s.tasks[task.ID]; exists {
		runner.cancel()
		delete(s.tasks, task.ID)
	}
	s.mu.Unlock()

	return s.startTask(ctx, task)
}

// UpdateTask 更新探测任务配置（停止旧任务，启动新配置的任务）
func (s *Scheduler) UpdateTask(ctx context.Context, task model.ProbeTask) error {
	s.mu.Lock()
	if runner, exists := s.tasks[task.ID]; exists {
		runner.cancel()
		delete(s.tasks, task.ID)
	}
	s.mu.Unlock()

	if !task.Enabled {
		log.Printf("任务 %d 已禁用，不启动", task.ID)
		return nil
	}

	return s.startTask(ctx, task)
}

// RemoveTask 移除探测任务并清理缓存
func (s *Scheduler) RemoveTask(taskID uint) error {
	s.mu.Lock()
	if runner, exists := s.tasks[taskID]; exists {
		runner.cancel()
		delete(s.tasks, taskID)
	}
	s.mu.Unlock()

	// 清理该任务关联的已删除记录缓存
	if err := s.cache.CleanByTask(taskID); err != nil {
		return fmt.Errorf("清理任务 %d 的缓存失败: %w", taskID, err)
	}

	log.Printf("已移除任务 %d 并清理缓存", taskID)
	return nil
}

// startTask 启动单个探测任务的 goroutine
func (s *Scheduler) startTask(ctx context.Context, task model.ProbeTask) error {
	// 获取或创建 provider
	prov, err := s.getOrCreateProvider(task.CredentialID)
	if err != nil {
		return fmt.Errorf("创建 DNS Provider 失败: %w", err)
	}

	// 创建任务上下文
	taskCtx, cancel := context.WithCancel(ctx)

	runner := &taskRunner{
		task:     task,
		cancel:   cancel,
		counters: make(map[string]*IPCounter),
	}

	// 加载该任务已有的已删除记录，初始化计数器
	deletedRecords, err := s.cache.ListByTask(task.ID)
	if err != nil {
		cancel()
		return fmt.Errorf("加载已删除记录失败: %w", err)
	}
	for _, dr := range deletedRecords {
		runner.counters[dr.IP] = &IPCounter{
			CurrentStatus: "deleted",
		}
	}

	s.mu.Lock()
	s.tasks[task.ID] = runner
	s.mu.Unlock()

	// 启动探测循环 goroutine
	go s.runProbeLoop(taskCtx, runner, prov)

	log.Printf("已启动任务 %d: %s.%s (协议=%s, 周期=%ds)",
		task.ID, task.SubDomain, task.Domain, task.ProbeProtocol, task.ProbeIntervalSec)

	return nil
}

// getOrCreateProvider 获取或创建 DNS Provider 实例
func (s *Scheduler) getOrCreateProvider(credentialID uint) (provider.DNSProvider, error) {
	s.mu.RLock()
	if prov, exists := s.providers[credentialID]; exists {
		s.mu.RUnlock()
		return prov, nil
	}
	s.mu.RUnlock()

	// 从数据库加载凭证
	var credential model.Credential
	if err := s.db.First(&credential, credentialID).Error; err != nil {
		return nil, fmt.Errorf("查询凭证 %d 失败: %w", credentialID, err)
	}

	// 使用工厂函数创建 provider
	prov, err := s.providerFactory(credential)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}

	s.mu.Lock()
	s.providers[credentialID] = prov
	s.mu.Unlock()

	return prov, nil
}

// runProbeLoop 单任务探测循环
func (s *Scheduler) runProbeLoop(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task
	ticker := time.NewTicker(time.Duration(task.ProbeIntervalSec) * time.Second)
	defer ticker.Stop()

	// 立即执行一次探测
	s.executeProbe(ctx, runner, prov)

	for {
		select {
		case <-ctx.Done():
			log.Printf("任务 %d 探测循环已停止", task.ID)
			return
		case <-ticker.C:
			s.executeProbe(ctx, runner, prov)
		}
	}
}

// executeProbe 执行一次完整的探测周期
// 根据任务类型分发到不同的处理逻辑：
// - pause_delete（或空值）：使用现有的暂停/删除逻辑
// - switch：使用新的故障转移逻辑
func (s *Scheduler) executeProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 根据任务类型分发处理逻辑
	switch model.TaskType(task.TaskType) {
	case model.TaskTypeSwitch:
		// 切换类型任务：使用故障转移逻辑
		s.executeSwitchProbe(ctx, runner, prov)
	case model.TaskTypeCDNSwitch:
		// CDN故障转移类型任务：通过Cloudflare proxied字段控制CDN代理开关
		s.executeCDNSwitchProbe(ctx, runner, prov)
	default:
		// 暂停/删除类型任务（包括空值和pause_delete）：使用现有逻辑
		s.executePauseDeleteProbe(ctx, runner, prov)
	}
}

// executePauseDeleteProbe 执行暂停/删除类型任务的探测（原有逻辑，增加API重试）
// 对DNS Provider的ListRecords调用增加重试和退避策略
// 根据记录类型分发：CNAME类型走专门的CNAME探测逻辑，A/AAAA类型走原有逻辑
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
func (s *Scheduler) executePauseDeleteProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// CNAME记录类型走专门的探测逻辑：解析CNAME→探测IP→根据阈值暂停/删除CNAME记录
	if model.RecordType(task.RecordType) == model.RecordTypeCNAME {
		s.executeCNAMEPauseDeleteProbe(ctx, runner, prov)
		return
	}

	// 1. 从 DNSProvider 获取当前域名的所有解析记录（A 和 AAAA 类型），带重试
	aRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
		return prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
	})
	if err != nil {
		log.Printf("任务 %d: 获取 A 记录失败（已重试）: %v", task.ID, err)
		// 获取失败时跳过本轮探测
		return
	}

	aaaaRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
		return prov.ListRecords(ctx, task.Domain, task.SubDomain, "AAAA")
	})
	if err != nil {
		log.Printf("任务 %d: 获取 AAAA 记录失败（已重试）: %v", task.ID, err)
		// AAAA 获取失败不影响 A 记录的探测，继续
	}

	// 合并 A 和 AAAA 记录
	allRecords := append(aRecords, aaaaRecords...)

	// 2. 合并 DeletedRecordCache 中的已删除记录
	deletedRecords, err := s.cache.ListByTask(task.ID)
	if err != nil {
		log.Printf("任务 %d: 获取已删除记录缓存失败: %v", task.ID, err)
	}

	// 合并 IP 列表
	mergedList := MergeIPList(allRecords, deletedRecords)

	// 获取被排除的 IP 列表
	var excludedIPs []model.ExcludedIP
	s.db.Where("task_id = ?", task.ID).Find(&excludedIPs)
	excludedSet := make(map[string]bool, len(excludedIPs))
	for _, e := range excludedIPs {
		excludedSet[e.IP] = true
	}

	// 创建探测器
	p := prober.NewProber(prober.ProbeProtocol(task.ProbeProtocol))
	if p == nil {
		log.Printf("任务 %d: 不支持的探测协议 %s", task.ID, task.ProbeProtocol)
		return
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 3. 对每个 IP 执行健康探测
	for _, item := range mergedList {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 跳过被用户手动排除的 IP
		if excludedSet[item.IP] {
			continue
		}

		result := p.Probe(ctx, item.IP, task.ProbePort, timeout)

		// 4. 更新连续成功/失败计数
		counter := s.getOrCreateCounter(runner, item.IP, item.Source)
		s.updateCounter(counter, result.Success)

		// 5. 记录探测结果到数据库
		s.saveProbeResult(task.ID, item.IP, result)

		// 6. 阈值判定和操作执行
		s.evaluateAndAct(ctx, runner, prov, item, counter, allRecords)
	}
}

// executeCNAMEPauseDeleteProbe 执行暂停/删除+CNAME类型任务的探测
// 流程：
// 1. 获取域名下所有CNAME记录
// 2. 分别解析每条CNAME记录指向的IP
// 3. 对每个IP进行健康探测
// 4. 按CNAME记录维度统计失败IP数量，与阈值比较
// 5. 某条CNAME下的失败IP达到阈值时，只暂停/删除那一条CNAME记录
// 6. 某条CNAME下的IP恢复健康时，恢复那一条CNAME记录
func (s *Scheduler) executeCNAMEPauseDeleteProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 检查CNAME解析器是否可用
	if s.cnameResolver == nil {
		log.Printf("任务 %d: CNAME类型任务需要CNAME解析器，但未配置", task.ID)
		return
	}

	// 1. 获取当前域名下所有CNAME记录
	cnameRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
		return prov.ListRecords(ctx, task.Domain, task.SubDomain, "CNAME")
	})
	if err != nil {
		log.Printf("任务 %d: 获取CNAME记录失败: %v", task.ID, err)
		return
	}

	// 同时获取已删除的CNAME记录缓存（用于恢复判断）
	deletedRecords, err := s.cache.ListByTask(task.ID)
	if err != nil {
		log.Printf("任务 %d: 获取已删除CNAME记录缓存失败: %v", task.ID, err)
	}

	// 构建所有需要处理的CNAME值列表（在线 + 已删除）
	// cnameValue -> recordID（在线记录有值，已删除记录为空）
	type cnameInfo struct {
		recordID string // DNS记录ID（在线记录有值）
		status   string // 记录状态（在线记录有值）
		ttl      int    // TTL
		isOnline bool   // 是否在线（未被删除）
	}
	cnameMap := make(map[string]*cnameInfo)

	for _, record := range cnameRecords {
		cnameMap[record.Value] = &cnameInfo{
			recordID: record.RecordID,
			status:   record.Status,
			ttl:      record.TTL,
			isOnline: true,
		}
	}
	for _, dr := range deletedRecords {
		if dr.RecordType == "CNAME" {
			if _, exists := cnameMap[dr.IP]; !exists {
				cnameMap[dr.IP] = &cnameInfo{
					ttl:      dr.TTL,
					isOnline: false,
				}
			}
		}
	}

	if len(cnameMap) == 0 {
		log.Printf("任务 %d: 未找到任何CNAME记录（在线或已删除），跳过探测", task.ID)
		return
	}

	// 创建探测器
	p := prober.NewProber(prober.ProbeProtocol(task.ProbeProtocol))
	if p == nil {
		log.Printf("任务 %d: 不支持的探测协议 %s", task.ID, task.ProbeProtocol)
		return
	}
	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 2. 逐条CNAME记录处理：解析IP → 探测 → 按记录维度判断
	for cnameValue, info := range cnameMap {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 2.1 解析该CNAME记录指向的所有IP
		ips, err := s.cnameResolver.ResolveIPs(ctx, cnameValue)
		if err != nil {
			log.Printf("任务 %d: 解析CNAME %s 失败: %v，保持现有目标", task.ID, cnameValue, err)
			ips = nil
		}

		// 2.2 更新该CNAME记录的目标IP列表
		if ips != nil {
			if err := s.cnameResolver.UpdateTargetsForCNAME(ctx, task.ID, cnameValue, ips); err != nil {
				log.Printf("任务 %d: 更新CNAME %s 的目标列表失败: %v", task.ID, cnameValue, err)
			}
		}

		// 2.3 从数据库加载该CNAME记录的目标列表
		var targets []model.CNAMETarget
		if err := s.db.Where("task_id = ? AND cname_value = ?", task.ID, cnameValue).Find(&targets).Error; err != nil {
			log.Printf("任务 %d: 加载CNAME %s 的目标列表失败: %v", task.ID, cnameValue, err)
			continue
		}

		if len(targets) == 0 {
			log.Printf("任务 %d: CNAME %s 目标列表为空，跳过", task.ID, cnameValue)
			continue
		}

		// 2.4 对每个IP进行探测
		for i := range targets {
			select {
			case <-ctx.Done():
				return
			default:
			}

			target := &targets[i]
			result := p.Probe(ctx, target.IP, task.ProbePort, timeout)
			s.saveProbeResult(task.ID, target.IP, result)
			s.updateCNAMETargetHealth(target, result.Success, task.FailThreshold, task.RecoverThreshold)
		}

		// 2.5 按该CNAME记录维度统计失败IP
		failedCount, err := s.cnameResolver.GetFailedIPCountByCNAME(ctx, task.ID, cnameValue)
		if err != nil {
			log.Printf("任务 %d: 获取CNAME %s 的失败IP数量失败: %v", task.ID, cnameValue, err)
			continue
		}

		totalIPs := len(targets)
		threshold := s.cnameResolver.CalculateThreshold(&runner.task, totalIPs)

		log.Printf("任务 %d: CNAME %s 探测完成，失败IP: %d/%d，阈值: %d",
			task.ID, cnameValue, failedCount, totalIPs, threshold)

		// 2.6 根据失败数量和阈值决定操作（只针对这一条CNAME记录）
		if failedCount >= threshold && threshold > 0 && info.isOnline {
			// 失败达到阈值且记录在线 -> 暂停/删除这条CNAME记录
			s.pauseOrDeleteCNAMERecord(ctx, runner, prov, cnameValue, info.recordID, info.ttl, failedCount, totalIPs, threshold)
		} else if (failedCount < threshold || threshold == 0) && !info.isOnline {
			// 恢复健康且记录已被删除 -> 重新添加这条CNAME记录
			s.restoreCNAMERecord(ctx, runner, prov, cnameValue, info.ttl)
		} else if (failedCount < threshold || threshold == 0) && info.isOnline &&
			(info.status == "paused" || info.status == "DISABLE") {
			// 恢复健康且记录已暂停 -> 恢复这条CNAME记录
			s.resumeCNAMERecord(ctx, runner, prov, cnameValue, info.recordID)
		}
	}
}

// pauseOrDeleteCNAMERecord 暂停或删除一条CNAME记录
func (s *Scheduler) pauseOrDeleteCNAMERecord(ctx context.Context, runner *taskRunner, prov provider.DNSProvider,
	cnameValue, recordID string, ttl, failedCount, totalIPs, threshold int) {
	task := runner.task

	// 尝试暂停
	err := retry.Do(ctx, s.retryConfig, func() error {
		return prov.PauseRecord(ctx, recordID)
	})
	if err != nil {
		// 暂停失败，尝试删除
		log.Printf("任务 %d: 暂停CNAME记录 %s 失败，尝试删除: %v", task.ID, cnameValue, err)
		err = retry.Do(ctx, s.retryConfig, func() error {
			return prov.DeleteRecord(ctx, recordID)
		})
		if err != nil {
			log.Printf("任务 %d: 删除CNAME记录 %s 失败: %v", task.ID, cnameValue, err)
			s.saveOperationLog(task.ID, "delete", recordID, cnameValue, "CNAME", false,
				fmt.Sprintf("删除CNAME记录失败: %v", err))
		} else {
			// 删除成功，缓存以便恢复
			s.cache.Add(model.DeletedRecord{
				TaskID:     task.ID,
				Domain:     task.Domain,
				SubDomain:  task.SubDomain,
				RecordType: "CNAME",
				IP:         cnameValue,
				TTL:        ttl,
			})
			s.saveOperationLog(task.ID, "delete", recordID, cnameValue, "CNAME", true,
				fmt.Sprintf("CNAME %s 失败IP %d/%d 达到阈值 %d，删除记录", cnameValue, failedCount, totalIPs, threshold))
			log.Printf("任务 %d: 已删除CNAME记录 %s", task.ID, cnameValue)

			if s.notificationManager != nil {
				s.notificationManager.Notify(notification.NotificationEvent{
					Type:         model.EventTypeFailover,
					TaskID:       task.ID,
					Domain:       task.Domain,
					SubDomain:    task.SubDomain,
					OccurredAt:   time.Now(),
					HealthStatus: "unhealthy",
				})
			}
		}
	} else {
		s.saveOperationLog(task.ID, "pause", recordID, cnameValue, "CNAME", true,
			fmt.Sprintf("CNAME %s 失败IP %d/%d 达到阈值 %d，暂停记录", cnameValue, failedCount, totalIPs, threshold))
		log.Printf("任务 %d: 已暂停CNAME记录 %s", task.ID, cnameValue)

		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:         model.EventTypeFailover,
				TaskID:       task.ID,
				Domain:       task.Domain,
				SubDomain:    task.SubDomain,
				OccurredAt:   time.Now(),
				HealthStatus: "unhealthy",
			})
		}
	}
}

// restoreCNAMERecord 恢复一条已删除的CNAME记录
func (s *Scheduler) restoreCNAMERecord(ctx context.Context, runner *taskRunner, prov provider.DNSProvider,
	cnameValue string, ttl int) {
	task := runner.task

	_, err := retry.DoWithResult(ctx, s.retryConfig, func() (string, error) {
		return prov.AddRecord(ctx, task.Domain, task.SubDomain, "CNAME", cnameValue, ttl)
	})
	if err != nil {
		log.Printf("任务 %d: 恢复CNAME记录 %s 失败: %v", task.ID, cnameValue, err)
		s.saveOperationLog(task.ID, "add", "", cnameValue, "CNAME", false,
			fmt.Sprintf("恢复CNAME记录失败: %v", err))
	} else {
		s.cache.Remove(task.ID, cnameValue)
		s.saveOperationLog(task.ID, "add", "", cnameValue, "CNAME", true,
			fmt.Sprintf("CNAME %s 下IP恢复健康，重新添加记录", cnameValue))
		log.Printf("任务 %d: 已恢复CNAME记录 %s", task.ID, cnameValue)

		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:           model.EventTypeRecovery,
				TaskID:         task.ID,
				Domain:         task.Domain,
				SubDomain:      task.SubDomain,
				OccurredAt:     time.Now(),
				RecoveredValue: cnameValue,
				HealthStatus:   "healthy",
			})
		}
	}
}

// resumeCNAMERecord 恢复一条已暂停的CNAME记录
func (s *Scheduler) resumeCNAMERecord(ctx context.Context, runner *taskRunner, prov provider.DNSProvider,
	cnameValue, recordID string) {
	task := runner.task

	err := retry.Do(ctx, s.retryConfig, func() error {
		return prov.ResumeRecord(ctx, recordID)
	})
	if err != nil {
		log.Printf("任务 %d: 恢复暂停的CNAME记录 %s 失败: %v", task.ID, cnameValue, err)
		s.saveOperationLog(task.ID, "resume", recordID, cnameValue, "CNAME", false,
			fmt.Sprintf("恢复暂停的CNAME记录失败: %v", err))
	} else {
		s.saveOperationLog(task.ID, "resume", recordID, cnameValue, "CNAME", true,
			fmt.Sprintf("CNAME %s 下IP恢复健康，恢复暂停的记录", cnameValue))
		log.Printf("任务 %d: 已恢复暂停的CNAME记录 %s", task.ID, cnameValue)

		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:           model.EventTypeRecovery,
				TaskID:         task.ID,
				Domain:         task.Domain,
				SubDomain:      task.SubDomain,
				OccurredAt:     time.Now(),
				RecoveredValue: cnameValue,
				HealthStatus:   "healthy",
			})
		}
	}
}

// executeCDNSwitchProbe 执行CDN故障转移类型任务的探测
// 流程：
// 1. 通过类型断言检查provider是否实现了ProxiedController接口
// 2. 解析探测CNAME域名获取IP列表
// 3. 对解析出的IP进行健康探测
// 4. 按阈值（百分比/个数）判断失败是否达标
// 5. 连续达标轮次达到失败阈值 → 启用CDN代理并切换记录值
// 6. 已切换且恢复健康 → 根据回切策略关闭CDN代理并恢复原始记录值
// 验证需求：4.3, 4.4, 4.5, 4.6, 4.8, 6.5, 6.6
func (s *Scheduler) executeCDNSwitchProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 1. 通过类型断言检查provider是否实现了ProxiedController接口
	pc, ok := prov.(provider.ProxiedController)
	if !ok {
		log.Printf("任务 %d: CDN故障转移需要ProxiedController接口支持，当前服务商不支持", task.ID)
		return
	}

	// 2. 从 DNS API 获取当前域名的解析记录，确定探测目标 IP
	// 已切换状态下，如果原始记录是 CNAME，当前实际是 A 记录（故障转移时转换的）
	// 此时需要探测原始 CNAME 指向的 IP 来判断源站是否恢复
	var record *provider.DNSRecord
	var recordType string
	var ips []string
	var err error

	if task.IsSwitched && task.OriginalRecordType == "CNAME" && task.OriginalValue != "" {
		// 已切换且原始是 CNAME：探测原始 CNAME 域名解析出的 IP
		if s.cnameResolver == nil {
			log.Printf("任务 %d: CNAME记录需要CNAME解析器支持", task.ID)
			return
		}
		ips, err = s.cnameResolver.ResolveIPs(ctx, task.OriginalValue)
		if err != nil {
			log.Printf("任务 %d: 解析原始CNAME %s 失败: %v", task.ID, task.OriginalValue, err)
			return
		}
		// 回切时需要用到当前 A 记录，提前获取
		records, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
			return prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
		})
		if err != nil || len(records) == 0 {
			log.Printf("任务 %d: 获取当前A记录失败（已切换状态）: %v", task.ID, err)
			return
		}
		record = &records[0]
		recordType = "A"
	} else {
		// 正常状态或非 CNAME 原始记录：按任务配置的记录类型查找
		record, recordType, err = s.findCDNRecord(ctx, prov, task)
		if err != nil || record == nil {
			log.Printf("任务 %d: 获取当前DNS记录失败: %v", task.ID, err)
			return
		}

		if recordType == "CNAME" {
			// CNAME 记录：需要解析其指向的域名获取 IP 列表
			if s.cnameResolver == nil {
				log.Printf("任务 %d: CNAME记录需要CNAME解析器支持", task.ID)
				return
			}
			ips, err = s.cnameResolver.ResolveIPs(ctx, record.Value)
			if err != nil {
				log.Printf("任务 %d: 解析CNAME %s 失败: %v", task.ID, record.Value, err)
				return
			}
		} else {
			// A/AAAA 记录：值本身就是 IP，直接探测
			allRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
				return prov.ListRecords(ctx, task.Domain, task.SubDomain, recordType)
			})
			if err != nil {
				log.Printf("任务 %d: 获取 %s 记录列表失败: %v", task.ID, recordType, err)
				return
			}
			for _, r := range allRecords {
				ips = append(ips, r.Value)
			}
		}
	}

	if len(ips) == 0 {
		log.Printf("任务 %d: 域名 %s.%s 未解析到任何IP", task.ID, task.SubDomain, task.Domain)
		return
	}

	// 3. 创建探测器
	p := prober.NewProber(prober.ProbeProtocol(task.ProbeProtocol))
	if p == nil {
		log.Printf("任务 %d: 不支持的探测协议 %s", task.ID, task.ProbeProtocol)
		return
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 4. 探测所有解析出的 IP
	failCount := 0
	successCount := 0
	for _, ip := range ips {
		result := p.Probe(ctx, ip, task.ProbePort, timeout)
		s.saveProbeResult(task.ID, ip, result)

		counter := s.getOrCreateCounter(runner, ip, "online")
		s.updateCounter(counter, result.Success)

		if !result.Success {
			failCount++
		} else {
			successCount++
		}

		log.Printf("任务 %d: CDN探测 IP=%s 成功=%v", task.ID, ip, result.Success)
	}

	// 5. 使用阈值计算判断失败是否达标（支持百分比和个数）
	totalIPs := len(ips)
	threshold := cname.CalculateThreshold(task.FailThresholdType, task.FailThresholdValue, totalIPs)
	thresholdReached := failCount >= threshold && threshold > 0

	// 使用全局计数器跟踪连续达标/恢复轮次
	globalCounter := s.getOrCreateCounter(runner, "cdn_global", "online")

	if thresholdReached {
		globalCounter.ConsecutiveFails++
		globalCounter.ConsecutiveSuccesses = 0
	} else if failCount == 0 {
		// 全部成功才算恢复
		globalCounter.ConsecutiveSuccesses++
		globalCounter.ConsecutiveFails = 0
	} else {
		// 有失败但未达阈值：重置连续达标计数，不影响恢复计数
		globalCounter.ConsecutiveFails = 0
	}

	log.Printf("任务 %d: CDN探测汇总 总IP=%d 失败=%d 阈值=%d(%s:%d) 达标=%v 连续达标轮次=%d 连续恢复轮次=%d",
		task.ID, totalIPs, failCount, threshold, task.FailThresholdType, task.FailThresholdValue,
		thresholdReached, globalCounter.ConsecutiveFails, globalCounter.ConsecutiveSuccesses)

	// 6. 已切换状态（CDN已启用），判断是否回切
	if task.IsSwitched {
		if model.SwitchBackPolicy(task.SwitchBackPolicy) == model.SwitchBackAuto &&
			globalCounter.ConsecutiveSuccesses >= task.RecoverThreshold {
			log.Printf("任务 %d: %s.%s 已恢复健康（连续恢复 %d 轮），执行CDN回切",
				task.ID, task.SubDomain, task.Domain, globalCounter.ConsecutiveSuccesses)

			// 步骤2已获取当前DNS记录到外层record变量，直接复用
			// 已切换+CNAME场景：record是当前A记录；其他场景：record是原始记录类型
			origRecordType := task.OriginalRecordType

			if origRecordType == "CNAME" {
				// 原始是 CNAME：删除当前 A 记录，重新创建 CNAME 记录
				log.Printf("任务 %d: 原始记录为CNAME，删除当前A记录后恢复CNAME记录 %s", task.ID, task.OriginalValue)

				// 先关闭 CDN 代理（删除前需要先关闭 proxied）
				if err := pc.SetProxied(ctx, record.RecordID, false); err != nil {
					log.Printf("任务 %d: 回切时关闭CDN代理失败: %v", task.ID, err)
					s.saveOperationLog(task.ID, "cdn_disable", record.RecordID, task.OriginalValue, "A", false,
						fmt.Sprintf("回切时关闭CDN代理失败: %v", err))
					return
				}

				// 删除当前 A 记录
				if err := retry.Do(ctx, s.retryConfig, func() error {
					return prov.DeleteRecord(ctx, record.RecordID)
				}); err != nil {
					log.Printf("任务 %d: 回切时删除A记录失败: %v", task.ID, err)
					s.saveOperationLog(task.ID, "cdn_disable", record.RecordID, task.OriginalValue, "A", false,
						fmt.Sprintf("回切时删除A记录失败: %v", err))
					return
				}

				// 重新创建 CNAME 记录
				_, err := retry.DoWithResult(ctx, s.retryConfig, func() (string, error) {
					return prov.AddRecord(ctx, task.Domain, task.SubDomain, "CNAME", task.OriginalValue, record.TTL)
				})
				if err != nil {
					log.Printf("任务 %d: 回切时创建CNAME记录失败: %v", task.ID, err)
					s.saveOperationLog(task.ID, "cdn_disable", "", task.OriginalValue, "CNAME", false,
						fmt.Sprintf("回切时创建CNAME记录失败: %v", err))
					return
				}
			} else {
				// 原始是 A/AAAA：直接恢复记录值并关闭代理
				if task.OriginalValue != "" {
					if err := prov.UpdateRecordValue(ctx, record.RecordID, task.OriginalValue); err != nil {
						log.Printf("任务 %d: 恢复原始记录值失败: %v", task.ID, err)
						s.saveOperationLog(task.ID, "cdn_disable", record.RecordID, task.OriginalValue, origRecordType, false,
							fmt.Sprintf("恢复原始记录值失败: %v", err))
						return
					}
				}

				if err := pc.SetProxied(ctx, record.RecordID, false); err != nil {
					log.Printf("任务 %d: 关闭CDN代理失败: %v", task.ID, err)
					s.saveOperationLog(task.ID, "cdn_disable", record.RecordID, task.OriginalValue, origRecordType, false,
						fmt.Sprintf("关闭CDN代理失败: %v", err))
					return
				}
			}

			// 更新任务状态
			if err := s.db.Model(&model.ProbeTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"is_switched":   false,
				"current_value": task.OriginalValue,
			}).Error; err != nil {
				log.Printf("任务 %d: 更新任务状态失败: %v", task.ID, err)
			}

			s.saveOperationLog(task.ID, "cdn_disable", "", task.OriginalValue, origRecordType, true,
				"源站恢复健康，关闭CDN代理并恢复原始记录值")

			globalCounter.ConsecutiveFails = 0
			globalCounter.ConsecutiveSuccesses = 0
			runner.task = s.reloadTask(runner.task)
			log.Printf("任务 %d: CDN回切成功", task.ID)

			// 发送恢复通知
			if s.notificationManager != nil {
				s.notificationManager.Notify(notification.NotificationEvent{
					Type:           model.EventTypeRecovery,
					TaskID:         task.ID,
					Domain:         task.Domain,
					SubDomain:      task.SubDomain,
					OccurredAt:     time.Now(),
					RecoveredValue: task.OriginalValue,
					HealthStatus:   "healthy",
				})
			}
		}
		return
	}

	// 7. 未切换状态，判断是否需要启用CDN故障转移
	if globalCounter.ConsecutiveFails >= task.FailThreshold {
		log.Printf("任务 %d: %s.%s 连续 %d 轮失败达阈值（%s:%d），触发CDN故障转移",
			task.ID, task.SubDomain, task.Domain, globalCounter.ConsecutiveFails, task.FailThresholdType, task.FailThresholdValue)

		// 直接复用步骤2获取的record和recordType（步骤7只在未切换时执行，record就是原始记录）
		originalValue := record.Value
		targetIP := task.CDNTarget

		// 根据原始记录类型决定操作方式
		var newRecordID string
		if recordType == "CNAME" {
			// CNAME 记录不能直接改为 IP，需要删除 CNAME 后创建 A 记录
			log.Printf("任务 %d: 原始记录为CNAME，删除后创建A记录指向目标IP %s", task.ID, targetIP)

			// 删除原始 CNAME 记录
			if err := retry.Do(ctx, s.retryConfig, func() error {
				return prov.DeleteRecord(ctx, record.RecordID)
			}); err != nil {
				log.Printf("任务 %d: 删除原始CNAME记录失败: %v", task.ID, err)
				s.saveOperationLog(task.ID, "cdn_enable", record.RecordID, targetIP, recordType, false,
					fmt.Sprintf("删除原始CNAME记录失败: %v", err))
				return
			}

			// 创建 A 记录指向目标 IP
			newID, err := retry.DoWithResult(ctx, s.retryConfig, func() (string, error) {
				return prov.AddRecord(ctx, task.Domain, task.SubDomain, "A", targetIP, record.TTL)
			})
			if err != nil {
				log.Printf("任务 %d: 创建A记录失败: %v，尝试恢复CNAME记录", task.ID, err)
				// 尝试恢复原始 CNAME 记录
				_, _ = prov.AddRecord(ctx, task.Domain, task.SubDomain, "CNAME", originalValue, record.TTL)
				s.saveOperationLog(task.ID, "cdn_enable", "", targetIP, "A", false,
					fmt.Sprintf("创建A记录失败: %v", err))
				return
			}
			newRecordID = newID
		} else {
			// A/AAAA 记录可以直接更新值
			if err := prov.UpdateRecordValue(ctx, record.RecordID, targetIP); err != nil {
				log.Printf("任务 %d: 切换记录值到目标IP失败: %v", task.ID, err)
				s.saveOperationLog(task.ID, "cdn_enable", record.RecordID, targetIP, recordType, false,
					fmt.Sprintf("切换记录值到目标IP失败: %v", err))
				return
			}
			newRecordID = record.RecordID
		}

		// 启用 CDN 代理（橙色云）
		if err := pc.SetProxied(ctx, newRecordID, true); err != nil {
			log.Printf("任务 %d: 启用CDN代理失败: %v", task.ID, err)
			// 尝试回滚
			if recordType == "CNAME" {
				_ = prov.DeleteRecord(ctx, newRecordID)
				_, _ = prov.AddRecord(ctx, task.Domain, task.SubDomain, "CNAME", originalValue, record.TTL)
			} else {
				_ = prov.UpdateRecordValue(ctx, newRecordID, originalValue)
			}
			s.saveOperationLog(task.ID, "cdn_enable", newRecordID, targetIP, "A", false,
				fmt.Sprintf("启用CDN代理失败: %v", err))
			return
		}

		// 更新任务状态（记录原始记录类型，回切时需要）
		if err := s.db.Model(&model.ProbeTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"is_switched":          true,
			"original_value":       originalValue,
			"original_record_type": recordType,
			"current_value":        targetIP,
		}).Error; err != nil {
			log.Printf("任务 %d: 更新任务状态失败: %v", task.ID, err)
		}

		s.saveOperationLog(task.ID, "cdn_enable", newRecordID, targetIP, "A", true,
			fmt.Sprintf("%s.%s 连续 %d 轮失败达阈值（%s:%d），启用CDN代理并切换记录值为 %s",
				task.SubDomain, task.Domain, globalCounter.ConsecutiveFails, task.FailThresholdType, task.FailThresholdValue, targetIP))

		runner.task = s.reloadTask(runner.task)
		log.Printf("任务 %d: CDN故障转移成功，已启用CDN代理并切换记录值为 %s", task.ID, targetIP)

		// 发送故障转移通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:          model.EventTypeFailover,
				TaskID:        task.ID,
				Domain:        task.Domain,
				SubDomain:     task.SubDomain,
				OccurredAt:    time.Now(),
				OriginalValue: originalValue,
				BackupValue:   fmt.Sprintf("%s (CDN proxied=true)", targetIP),
				HealthStatus:  "unhealthy",
			})
		}
	}
}

// findCDNRecord 查找CDN故障转移操作的目标DNS记录
// 根据任务配置的记录类型直接查找对应的DNS记录
// A_AAAA 类型会依次尝试 A 和 AAAA
func (s *Scheduler) findCDNRecord(ctx context.Context, prov provider.DNSProvider, task model.ProbeTask) (*provider.DNSRecord, string, error) {
	// 根据任务配置的记录类型确定要查找的类型列表
	var recordTypes []string
	switch model.RecordType(task.RecordType) {
	case model.RecordTypeCNAME:
		recordTypes = []string{"CNAME"}
	case model.RecordTypeA:
		recordTypes = []string{"A"}
	case model.RecordTypeAAAA:
		recordTypes = []string{"AAAA"}
	default:
		// A_AAAA 或未指定：依次尝试 A、AAAA
		recordTypes = []string{"A", "AAAA"}
	}

	for _, rt := range recordTypes {
		records, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
			return prov.ListRecords(ctx, task.Domain, task.SubDomain, rt)
		})
		if err != nil {
			log.Printf("任务 %d: 获取 %s 记录失败: %v", task.ID, rt, err)
			continue
		}
		if len(records) > 0 {
			log.Printf("任务 %d: 找到 %s 记录 (RecordID=%s, Value=%s)", task.ID, rt, records[0].RecordID, records[0].Value)
			return &records[0], rt, nil
		}
	}

	return nil, "", fmt.Errorf("未找到 %s 类型的DNS记录", task.RecordType)
}

// executeSwitchProbe 执行切换类型任务的探测
// 根据解析记录类型（CNAME / A / AAAA）分发到不同的处理逻辑
func (s *Scheduler) executeSwitchProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 检查故障转移执行器是否可用
	if s.failoverExecutor == nil {
		log.Printf("任务 %d: 切换类型任务需要故障转移执行器，但未配置", task.ID)
		return
	}

	// 根据解析记录类型分发
	switch model.RecordType(task.RecordType) {
	case model.RecordTypeCNAME:
		s.executeCNAMESwitchProbe(ctx, runner, prov)
	case model.RecordTypeA, model.RecordTypeAAAA, model.RecordTypeA_AAAA:
		s.executeDirectSwitchProbe(ctx, runner, prov)
	default:
		log.Printf("任务 %d: 不支持的解析记录类型 %s", task.ID, task.RecordType)
	}
}

// executeCNAMESwitchProbe 执行CNAME切换类型任务的探测
// 流程：
// 1. 使用CNAMEResolver解析CNAME记录指向的所有IP
// 2. 更新CNAMETarget表中的IP列表
// 3. 对每个IP进行健康探测
// 4. 统计失败IP数量，与阈值比较
// 5. 如果失败数量达到阈值，触发故障转移（切换到解析池中的健康域名）
// 6. 如果已切换且原域名恢复健康，根据回切策略决定是否回切
func (s *Scheduler) executeCNAMESwitchProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 检查CNAME解析器是否可用
	if s.cnameResolver == nil {
		log.Printf("任务 %d: CNAME类型任务需要CNAME解析器，但未配置", task.ID)
		return
	}

	// 1. 构建完整域名用于CNAME解析
	fullDomain := task.Domain
	if task.SubDomain != "" && task.SubDomain != "@" {
		fullDomain = task.SubDomain + "." + task.Domain
	}

	// 2. 获取当前CNAME记录值（用于关联CNAMETarget）
	cnameValue := fullDomain // 默认使用完整域名
	cnameRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
		return prov.ListRecords(ctx, task.Domain, task.SubDomain, "CNAME")
	})
	if err == nil && len(cnameRecords) > 0 {
		cnameValue = cnameRecords[0].Value // 使用实际的CNAME记录值
	}

	// 3. 解析CNAME记录指向的所有IP
	ips, err := s.cnameResolver.ResolveIPs(ctx, cnameValue)
	if err != nil {
		log.Printf("任务 %d: 解析CNAME记录失败: %v，保持现有目标列表", task.ID, err)
		// 解析失败时，继续使用现有的目标列表进行探测
		ips = nil
	}

	// 4. 如果解析成功，更新CNAMETarget表中的IP列表（关联到CNAME记录值）
	if ips != nil {
		if err := s.cnameResolver.UpdateTargetsForCNAME(ctx, task.ID, cnameValue, ips); err != nil {
			log.Printf("任务 %d: 更新CNAME目标列表失败: %v", task.ID, err)
		}
	}

	// 5. 从数据库加载当前的CNAME目标列表
	var targets []model.CNAMETarget
	if err := s.db.Where("task_id = ?", task.ID).Find(&targets).Error; err != nil {
		log.Printf("任务 %d: 加载CNAME目标列表失败: %v", task.ID, err)
		return
	}

	if len(targets) == 0 {
		log.Printf("任务 %d: CNAME目标列表为空，跳过探测", task.ID)
		return
	}

	// 5. 创建探测器
	p := prober.NewProber(prober.ProbeProtocol(task.ProbeProtocol))
	if p == nil {
		log.Printf("任务 %d: 不支持的探测协议 %s", task.ID, task.ProbeProtocol)
		return
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 6. 对每个CNAME目标IP进行探测
	for i := range targets {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return
		default:
		}

		target := &targets[i]
		result := p.Probe(ctx, target.IP, task.ProbePort, timeout)

		// 记录探测结果
		s.saveProbeResult(task.ID, target.IP, result)

		// 更新CNAME目标的健康状态
		s.updateCNAMETargetHealth(target, result.Success, task.FailThreshold, task.RecoverThreshold)
	}

	// 7. 统计失败IP数量并判断是否需要故障转移
	failedCount, err := s.cnameResolver.GetFailedIPCount(ctx, task.ID)
	if err != nil {
		log.Printf("任务 %d: 获取失败IP数量失败: %v", task.ID, err)
		return
	}

	totalIPs := len(targets)
	threshold := s.cnameResolver.CalculateThreshold(&runner.task, totalIPs)

	log.Printf("任务 %d: CNAME探测完成，失败IP: %d/%d，阈值: %d", task.ID, failedCount, totalIPs, threshold)

	// 8. 根据失败数量和阈值决定操作
	if !task.IsSwitched && failedCount >= threshold && threshold > 0 {
		// 未切换且失败数量达到阈值 -> 触发故障转移
		s.triggerCNAMEFailover(ctx, runner, prov)
	} else if task.IsSwitched && failedCount < threshold {
		// 已切换且原域名恢复健康（失败数量低于阈值）-> 判断是否回切
		s.evaluateCNAMESwitchBack(ctx, runner, prov)
	}
}

// executeDirectSwitchProbe 执行A/AAAA切换类型任务的探测
// 流程：
// 1. 探测当前IP的健康状态
// 2. 如果连续失败达到阈值，从解析池选择备用IP并切换
// 3. 如果已切换且原IP恢复健康，根据回切策略决定是否回切
func (s *Scheduler) executeDirectSwitchProbe(ctx context.Context, runner *taskRunner, prov provider.DNSProvider) {
	task := runner.task

	// 1. 获取当前DNS记录，确定要探测的IP（带重试）
	// A_AAAA 类型需要同时获取 A 和 AAAA 记录
	var records []provider.DNSRecord
	if model.RecordType(task.RecordType) == model.RecordTypeA_AAAA {
		aRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
			return prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
		})
		if err != nil {
			log.Printf("任务 %d: 获取 A 记录失败（已重试）: %v", task.ID, err)
		} else {
			records = append(records, aRecords...)
		}
		aaaaRecords, err := retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
			return prov.ListRecords(ctx, task.Domain, task.SubDomain, "AAAA")
		})
		if err != nil {
			log.Printf("任务 %d: 获取 AAAA 记录失败（已重试）: %v", task.ID, err)
		} else {
			records = append(records, aaaaRecords...)
		}
	} else {
		var err error
		records, err = retry.DoWithResult(ctx, s.retryConfig, func() ([]provider.DNSRecord, error) {
			return prov.ListRecords(ctx, task.Domain, task.SubDomain, task.RecordType)
		})
		if err != nil {
			log.Printf("任务 %d: 获取 %s 记录失败（已重试）: %v", task.ID, task.RecordType, err)
			return
		}
	}

	if len(records) == 0 {
		log.Printf("任务 %d: 未找到 %s 记录", task.ID, task.RecordType)
		return
	}

	// 2. 创建探测器
	p := prober.NewProber(prober.ProbeProtocol(task.ProbeProtocol))
	if p == nil {
		log.Printf("任务 %d: 不支持的探测协议 %s", task.ID, task.ProbeProtocol)
		return
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 3. 探测当前IP
	currentIP := records[0].Value
	result := p.Probe(ctx, currentIP, task.ProbePort, timeout)

	// 4. 更新计数器
	counter := s.getOrCreateCounter(runner, currentIP, "online")
	s.updateCounter(counter, result.Success)

	// 5. 记录探测结果
	s.saveProbeResult(task.ID, currentIP, result)

	log.Printf("任务 %d: A/AAAA探测 IP=%s 成功=%v 连续失败=%d 连续成功=%d",
		task.ID, currentIP, result.Success, counter.ConsecutiveFails, counter.ConsecutiveSuccesses)

	// 6. 如果已切换到备用IP，还需要探测原始IP以判断是否回切
	if task.IsSwitched && task.OriginalValue != "" && task.OriginalValue != currentIP {
		origResult := p.Probe(ctx, task.OriginalValue, task.ProbePort, timeout)

		// 使用带"original_"前缀的key来跟踪原始IP的计数器
		origCounter := s.getOrCreateCounter(runner, "original_"+task.OriginalValue, "online")
		s.updateCounter(origCounter, origResult.Success)

		// 记录原始IP的探测结果
		s.saveProbeResult(task.ID, task.OriginalValue, origResult)

		// 判断是否应该回切
		if s.failoverExecutor.ShouldSwitchBack(&runner.task) &&
			origCounter.ConsecutiveSuccesses >= task.RecoverThreshold {
			log.Printf("任务 %d: 原始IP %s 已恢复健康（连续成功 %d 次），执行回切",
				task.ID, task.OriginalValue, origCounter.ConsecutiveSuccesses)

			if err := s.failoverExecutor.SwitchBack(ctx, &runner.task); err != nil {
				log.Printf("任务 %d: 回切失败: %v", task.ID, err)
			} else {
				// 回切成功，重置计数器
				origCounter.ConsecutiveFails = 0
				origCounter.ConsecutiveSuccesses = 0
				// 同步更新runner中的task状态
				runner.task = s.reloadTask(runner.task)
				log.Printf("任务 %d: 回切成功，已恢复到原始IP %s", task.ID, task.OriginalValue)

				// 发送恢复通知
				if s.notificationManager != nil {
					s.notificationManager.Notify(notification.NotificationEvent{
						Type:           model.EventTypeRecovery,
						TaskID:         task.ID,
						Domain:         task.Domain,
						SubDomain:      task.SubDomain,
						OccurredAt:     time.Now(),
						RecoveredValue: task.OriginalValue,
						HealthStatus:   "healthy",
					})
				}
			}
		}
		return
	}

	// 7. 未切换状态下，判断是否需要故障转移
	if !task.IsSwitched && counter.ConsecutiveFails >= task.FailThreshold {
		log.Printf("任务 %d: IP %s 连续失败 %d 次，达到阈值 %d，触发故障转移",
			task.ID, currentIP, counter.ConsecutiveFails, task.FailThreshold)

		s.triggerDirectFailover(ctx, runner, prov)
	}
}

// updateCNAMETargetHealth 更新CNAME目标IP的健康状态
func (s *Scheduler) updateCNAMETargetHealth(target *model.CNAMETarget, success bool, failThreshold, recoverThreshold int) {
	// 更新连续计数
	if success {
		target.ConsecutiveFails = 0
		target.ConsecutiveSuccesses++
	} else {
		target.ConsecutiveFails++
		target.ConsecutiveSuccesses = 0
	}

	// 判断健康状态转换
	oldStatus := target.HealthStatus
	switch model.HealthStatus(target.HealthStatus) {
	case model.HealthStatusHealthy, model.HealthStatusUnknown:
		if target.ConsecutiveFails >= failThreshold {
			target.HealthStatus = string(model.HealthStatusUnhealthy)
		}
		// 未知状态下首次成功，标记为健康
		if model.HealthStatus(oldStatus) == model.HealthStatusUnknown && success {
			target.HealthStatus = string(model.HealthStatusHealthy)
		}
	case model.HealthStatusUnhealthy:
		if target.ConsecutiveSuccesses >= recoverThreshold {
			target.HealthStatus = string(model.HealthStatusHealthy)
		}
	}

	// 更新最后探测时间
	now := time.Now()
	target.LastProbeAt = &now

	// 持久化到数据库
	if err := s.db.Model(target).Updates(map[string]any{
		"health_status":         target.HealthStatus,
		"consecutive_fails":     target.ConsecutiveFails,
		"consecutive_successes": target.ConsecutiveSuccesses,
		"last_probe_at":         target.LastProbeAt,
	}).Error; err != nil {
		log.Printf("更新CNAME目标 %s 健康状态失败: %v", target.IP, err)
	}

	if oldStatus != target.HealthStatus {
		log.Printf("CNAME目标 %s 健康状态变更: %s -> %s", target.IP, oldStatus, target.HealthStatus)
	}
}

// triggerCNAMEFailover 触发CNAME故障转移，从解析池选择健康域名并切换
func (s *Scheduler) triggerCNAMEFailover(ctx context.Context, runner *taskRunner, _ provider.DNSProvider) {
	task := runner.task

	// 检查是否关联了解析池
	if task.PoolID == nil {
		log.Printf("任务 %d: 切换类型任务未关联解析池，无法执行故障转移", task.ID)
		return
	}

	// 检查资源选择器是否可用
	if s.resourceSelector == nil {
		log.Printf("任务 %d: 资源选择器未配置，无法执行故障转移", task.ID)
		return
	}

	// 从解析池选择最优的健康域名
	backupValue, err := s.resourceSelector.SelectBestResource(ctx, *task.PoolID)
	if err != nil {
		log.Printf("任务 %d: 从解析池 %d 选择备用资源失败: %v", task.ID, *task.PoolID, err)
		// 记录错误日志
		s.saveOperationLog(task.ID, "switch_to_backup", "", "", task.RecordType, false,
			fmt.Sprintf("从解析池选择备用域名失败: %v", err))
		return
	}

	log.Printf("任务 %d: 从解析池 %d 选择到备用域名: %s", task.ID, *task.PoolID, backupValue)

	// 执行切换
	if err := s.failoverExecutor.SwitchToBackup(ctx, &runner.task, backupValue); err != nil {
		log.Printf("任务 %d: 切换到备用域名 %s 失败: %v", task.ID, backupValue, err)
	} else {
		// 切换成功，同步更新runner中的task状态
		runner.task = s.reloadTask(runner.task)
		log.Printf("任务 %d: 成功切换到备用域名 %s", task.ID, backupValue)

		// 发送故障转移通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:          model.EventTypeFailover,
				TaskID:        task.ID,
				Domain:        task.Domain,
				SubDomain:     task.SubDomain,
				OccurredAt:    time.Now(),
				OriginalValue: task.OriginalValue,
				BackupValue:   backupValue,
				HealthStatus:  "unhealthy",
			})
		}
	}
}

// evaluateCNAMESwitchBack 评估CNAME任务是否应该回切
func (s *Scheduler) evaluateCNAMESwitchBack(ctx context.Context, runner *taskRunner, _ provider.DNSProvider) {
	task := runner.task

	// 检查回切策略
	if !s.failoverExecutor.ShouldSwitchBack(&runner.task) {
		log.Printf("任务 %d: 回切策略为保持当前，不执行回切", task.ID)
		return
	}

	// 执行回切
	if err := s.failoverExecutor.SwitchBack(ctx, &runner.task); err != nil {
		log.Printf("任务 %d: 回切失败: %v", task.ID, err)
	} else {
		// 回切成功，同步更新runner中的task状态
		runner.task = s.reloadTask(runner.task)
		log.Printf("任务 %d: 成功回切到原始域名", task.ID)

		// 发送恢复通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:           model.EventTypeRecovery,
				TaskID:         task.ID,
				Domain:         task.Domain,
				SubDomain:      task.SubDomain,
				OccurredAt:     time.Now(),
				RecoveredValue: task.OriginalValue,
				HealthStatus:   "healthy",
			})
		}
	}
}

// triggerDirectFailover 触发A/AAAA直接切换故障转移
func (s *Scheduler) triggerDirectFailover(ctx context.Context, runner *taskRunner, _ provider.DNSProvider) {
	task := runner.task

	// 检查是否关联了解析池
	if task.PoolID == nil {
		log.Printf("任务 %d: 切换类型任务未关联解析池，无法执行故障转移", task.ID)
		return
	}

	// 检查资源选择器是否可用
	if s.resourceSelector == nil {
		log.Printf("任务 %d: 资源选择器未配置，无法执行故障转移", task.ID)
		return
	}

	// 从解析池选择最优的健康IP
	backupValue, err := s.resourceSelector.SelectBestResource(ctx, *task.PoolID)
	if err != nil {
		log.Printf("任务 %d: 从解析池 %d 选择备用资源失败: %v", task.ID, *task.PoolID, err)
		// 记录错误日志
		s.saveOperationLog(task.ID, "switch_to_backup", "", "", task.RecordType, false,
			fmt.Sprintf("从解析池选择备用IP失败: %v", err))
		return
	}

	log.Printf("任务 %d: 从解析池 %d 选择到备用IP: %s", task.ID, *task.PoolID, backupValue)

	// 执行切换
	if err := s.failoverExecutor.SwitchToBackup(ctx, &runner.task, backupValue); err != nil {
		log.Printf("任务 %d: 切换到备用IP %s 失败: %v", task.ID, backupValue, err)
	} else {
		// 切换成功，同步更新runner中的task状态
		runner.task = s.reloadTask(runner.task)
		log.Printf("任务 %d: 成功切换到备用IP %s", task.ID, backupValue)

		// 发送故障转移通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:          model.EventTypeFailover,
				TaskID:        task.ID,
				Domain:        task.Domain,
				SubDomain:     task.SubDomain,
				OccurredAt:    time.Now(),
				OriginalValue: task.OriginalValue,
				BackupValue:   backupValue,
				HealthStatus:  "unhealthy",
			})
		}
	}
}

// reloadTask 从数据库重新加载任务状态
// 在故障转移或回切操作后调用，确保runner中的task状态与数据库一致
func (s *Scheduler) reloadTask(task model.ProbeTask) model.ProbeTask {
	var updated model.ProbeTask
	if err := s.db.First(&updated, task.ID).Error; err != nil {
		log.Printf("重新加载任务 %d 失败: %v，使用旧状态", task.ID, err)
		return task
	}
	return updated
}

// MergedIP 合并后的 IP 信息
type MergedIP struct {
	IP         string
	RecordID   string // DNS 记录 ID（在线记录有值）
	RecordType string // "A" 或 "AAAA"
	Source     string // "online" 或 "deleted"
	Status     string // DNS 记录状态 "ENABLE" / "DISABLE"
	SubDomain  string
	Domain     string
	TTL        int
}

// MergeIPList 合并在线 DNS 记录和已删除记录缓存中的 IP 列表（导出以便属性测试使用）
// 返回去重后的完整待探测 IP 列表
func MergeIPList(onlineRecords []provider.DNSRecord, deletedRecords []model.DeletedRecord) []MergedIP {
	result := make([]MergedIP, 0)
	seen := make(map[string]bool)

	// 先添加在线记录
	for _, r := range onlineRecords {
		if !seen[r.Value] {
			seen[r.Value] = true
			result = append(result, MergedIP{
				IP:         r.Value,
				RecordID:   r.RecordID,
				RecordType: r.Type,
				Source:     "online",
				Status:     r.Status,
				SubDomain:  r.SubDomain,
				Domain:     r.DomainName,
				TTL:        r.TTL,
			})
		}
	}

	// 再添加已删除记录中不在在线列表中的 IP
	for _, dr := range deletedRecords {
		if !seen[dr.IP] {
			seen[dr.IP] = true
			result = append(result, MergedIP{
				IP:         dr.IP,
				RecordID:   "",
				RecordType: dr.RecordType,
				Source:     "deleted",
				SubDomain:  dr.SubDomain,
				Domain:     dr.Domain,
				TTL:        dr.TTL,
			})
		}
	}

	return result
}

// getOrCreateCounter 获取或创建 IP 计数器
func (s *Scheduler) getOrCreateCounter(runner *taskRunner, ip string, source string) *IPCounter {
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, exists := runner.counters[ip]
	if !exists {
		status := "healthy"
		if source == "deleted" {
			status = "deleted"
		}
		counter = &IPCounter{
			CurrentStatus: status,
		}
		runner.counters[ip] = counter
	}
	return counter
}

// updateCounter 根据探测结果更新计数器
func (s *Scheduler) updateCounter(counter *IPCounter, success bool) {
	if success {
		counter.ConsecutiveSuccesses++
		counter.ConsecutiveFails = 0
	} else {
		counter.ConsecutiveFails++
		counter.ConsecutiveSuccesses = 0
	}
}

// EvaluateFailureAction 评估失败阈值触发的操作（导出以便属性测试使用）
// 返回值: "pause" / "delete" / "skip" / "none"
// - "pause": 应暂停记录
// - "delete": 应删除记录并存入缓存
// - "skip": 因最后一条记录保护而跳过
// - "none": 未达到阈值，无需操作
func EvaluateFailureAction(counter *IPCounter, failThreshold int, supportsPause bool, isLastActive bool) string {
	// 未达到失败阈值
	if counter.ConsecutiveFails < failThreshold {
		return "none"
	}

	// 已经处于暂停或删除状态，不重复操作
	if counter.CurrentStatus == "paused" || counter.CurrentStatus == "deleted" {
		return "none"
	}

	// 最后一条记录保护
	if isLastActive {
		return "skip"
	}

	// 根据 provider 是否支持暂停决定操作
	if supportsPause {
		return "pause"
	}
	return "delete"
}

// EvaluateRecoverAction 评估恢复阈值触发的操作（导出以便属性测试使用）
// 返回值: "resume" / "add" / "none"
// - "resume": 应启用已暂停的记录
// - "add": 应重新添加已删除的记录
// - "none": 未达到恢复阈值或无需恢复
func EvaluateRecoverAction(counter *IPCounter, recoverThreshold int) string {
	// 未达到恢复阈值
	if counter.ConsecutiveSuccesses < recoverThreshold {
		return "none"
	}

	switch counter.CurrentStatus {
	case "paused":
		return "resume"
	case "deleted":
		return "add"
	default:
		return "none"
	}
}

// IsLastActiveRecord 判断是否为最后一条活跃记录（导出以便属性测试使用）
// activeRecords: 当前在线且状态为 ENABLE 的记录列表
func IsLastActiveRecord(activeRecords []provider.DNSRecord, targetIP string) bool {
	activeCount := 0
	for _, r := range activeRecords {
		if r.Status == "ENABLE" {
			activeCount++
		}
	}
	// 如果只有一条活跃记录，且目标 IP 就是这条记录，则为最后一条
	return activeCount <= 1
}

// evaluateAndAct 评估阈值并执行相应操作
func (s *Scheduler) evaluateAndAct(
	ctx context.Context,
	runner *taskRunner,
	prov provider.DNSProvider,
	item MergedIP,
	counter *IPCounter,
	allRecords []provider.DNSRecord,
) {
	task := runner.task

	// 评估失败阈值
	isLastActive := IsLastActiveRecord(allRecords, item.IP)
	failAction := EvaluateFailureAction(counter, task.FailThreshold, prov.SupportsPause(), isLastActive)

	// 连续失败达到阈值时发送通知（无论后续操作是 pause、delete 还是 skip）
	if failAction != "none" && s.notificationManager != nil {
		s.notificationManager.Notify(notification.NotificationEvent{
			Type:          model.EventTypeConsecutiveFail,
			TaskID:        task.ID,
			Domain:        task.Domain,
			SubDomain:     task.SubDomain,
			OccurredAt:    time.Now(),
			FailCount:     counter.ConsecutiveFails,
			FailedIPs:     []string{item.IP},
			ProbeProtocol: task.ProbeProtocol,
			ProbePort:     task.ProbePort,
			HealthStatus:  "unhealthy",
		})
	}

	switch failAction {
	case "pause":
		s.executePause(ctx, runner, prov, item, counter)
	case "delete":
		s.executeDelete(ctx, runner, prov, item, counter)
	case "skip":
		log.Printf("任务 %d: IP %s 达到失败阈值但为最后一条活跃记录，跳过操作", task.ID, item.IP)
	}

	// 评估恢复阈值
	recoverAction := EvaluateRecoverAction(counter, task.RecoverThreshold)

	switch recoverAction {
	case "resume":
		s.executeResume(ctx, runner, prov, item, counter)
	case "add":
		s.executeAdd(ctx, runner, prov, item, counter)
	}
}

// executePause 执行暂停操作（带重试和退避策略）
// 对DNS Provider API调用进行重试，限流错误使用指数退避
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
func (s *Scheduler) executePause(
	ctx context.Context,
	runner *taskRunner,
	prov provider.DNSProvider,
	item MergedIP,
	counter *IPCounter,
) {
	task := runner.task
	err := retry.Do(ctx, s.retryConfig, func() error {
		return prov.PauseRecord(ctx, item.RecordID)
	})
	success := err == nil

	if success {
		counter.CurrentStatus = "paused"
		counter.ConsecutiveFails = 0
		log.Printf("任务 %d: 已暂停 IP %s 的 DNS 记录 (RecordID=%s)", task.ID, item.IP, item.RecordID)
	} else {
		log.Printf("任务 %d: 暂停 IP %s 失败（已重试）: %v", task.ID, item.IP, err)
	}

	// 记录操作日志
	s.saveOperationLog(task.ID, "pause", item.RecordID, item.IP, item.RecordType, success, fmt.Sprintf("暂停 DNS 记录, err=%v", err))
}

// executeDelete 执行删除操作（带重试和退避策略）
// 对DNS Provider API调用进行重试，限流错误使用指数退避
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
func (s *Scheduler) executeDelete(
	ctx context.Context,
	runner *taskRunner,
	prov provider.DNSProvider,
	item MergedIP,
	counter *IPCounter,
) {
	task := runner.task
	err := retry.Do(ctx, s.retryConfig, func() error {
		return prov.DeleteRecord(ctx, item.RecordID)
	})
	success := err == nil

	if success {
		counter.CurrentStatus = "deleted"
		counter.ConsecutiveFails = 0

		// 将记录存入已删除记录缓存
		deletedRecord := model.DeletedRecord{
			TaskID:     task.ID,
			Domain:     item.Domain,
			SubDomain:  item.SubDomain,
			RecordType: item.RecordType,
			IP:         item.IP,
			TTL:        item.TTL,
			DeletedAt:  time.Now(),
		}
		if cacheErr := s.cache.Add(deletedRecord); cacheErr != nil {
			log.Printf("任务 %d: 存入已删除记录缓存失败: %v", task.ID, cacheErr)
		}

		log.Printf("任务 %d: 已删除 IP %s 的 DNS 记录 (RecordID=%s) 并存入缓存", task.ID, item.IP, item.RecordID)
	} else {
		log.Printf("任务 %d: 删除 IP %s 失败（已重试）: %v", task.ID, item.IP, err)
	}

	// 记录操作日志
	s.saveOperationLog(task.ID, "delete", item.RecordID, item.IP, item.RecordType, success, fmt.Sprintf("删除 DNS 记录, err=%v", err))
}

// executeResume 执行恢复（启用）操作（带重试和退避策略）
// 对DNS Provider API调用进行重试，限流错误使用指数退避
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
func (s *Scheduler) executeResume(
	ctx context.Context,
	runner *taskRunner,
	prov provider.DNSProvider,
	item MergedIP,
	counter *IPCounter,
) {
	task := runner.task
	err := retry.Do(ctx, s.retryConfig, func() error {
		return prov.ResumeRecord(ctx, item.RecordID)
	})
	success := err == nil

	if success {
		counter.CurrentStatus = "healthy"
		counter.ConsecutiveSuccesses = 0
		log.Printf("任务 %d: 已恢复 IP %s 的 DNS 记录 (RecordID=%s)", task.ID, item.IP, item.RecordID)

		// 发送恢复通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:           model.EventTypeRecovery,
				TaskID:         task.ID,
				Domain:         task.Domain,
				SubDomain:      task.SubDomain,
				OccurredAt:     time.Now(),
				RecoveredValue: item.IP,
				HealthStatus:   "healthy",
			})
		}
	} else {
		log.Printf("任务 %d: 恢复 IP %s 失败（已重试）: %v", task.ID, item.IP, err)
	}

	// 记录操作日志
	s.saveOperationLog(task.ID, "resume", item.RecordID, item.IP, item.RecordType, success, fmt.Sprintf("启用 DNS 记录, err=%v", err))
}

// executeAdd 执行重新添加操作（从已删除状态恢复，带重试和退避策略）
// 对DNS Provider API调用进行重试，限流错误使用指数退避
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
func (s *Scheduler) executeAdd(
	ctx context.Context,
	runner *taskRunner,
	prov provider.DNSProvider,
	item MergedIP,
	counter *IPCounter,
) {
	task := runner.task
	recordID, err := retry.DoWithResult(ctx, s.retryConfig, func() (string, error) {
		return prov.AddRecord(ctx, item.Domain, item.SubDomain, item.RecordType, item.IP, item.TTL)
	})
	success := err == nil

	if success {
		counter.CurrentStatus = "healthy"
		counter.ConsecutiveSuccesses = 0

		// 从缓存中移除已恢复的记录
		if cacheErr := s.cache.Remove(task.ID, item.IP); cacheErr != nil {
			log.Printf("任务 %d: 从缓存移除已恢复记录失败: %v", task.ID, cacheErr)
		}

		log.Printf("任务 %d: 已重新添加 IP %s 的 DNS 记录 (新RecordID=%s)", task.ID, item.IP, recordID)

		// 发送恢复通知
		if s.notificationManager != nil {
			s.notificationManager.Notify(notification.NotificationEvent{
				Type:           model.EventTypeRecovery,
				TaskID:         task.ID,
				Domain:         task.Domain,
				SubDomain:      task.SubDomain,
				OccurredAt:     time.Now(),
				RecoveredValue: item.IP,
				HealthStatus:   "healthy",
			})
		}
	} else {
		log.Printf("任务 %d: 重新添加 IP %s 失败（已重试）: %v", task.ID, item.IP, err)
	}

	// 记录操作日志
	s.saveOperationLog(task.ID, "add", recordID, item.IP, item.RecordType, success, fmt.Sprintf("重新添加 DNS 记录, err=%v", err))
}

// saveProbeResult 保存探测结果到数据库
func (s *Scheduler) saveProbeResult(taskID uint, ip string, result prober.ProbeResult) {
	record := model.ProbeResult{
		TaskID:    taskID,
		IP:        ip,
		Success:   result.Success,
		LatencyMs: int(result.Latency.Milliseconds()),
		ErrorMsg:  result.Error,
		ProbedAt:  result.Time,
	}

	if err := s.db.Create(&record).Error; err != nil {
		log.Printf("保存探测结果失败: %v", err)
	}
}

// saveOperationLog 保存操作日志到数据库
func (s *Scheduler) saveOperationLog(taskID uint, opType, recordID, ip, recordType string, success bool, detail string) {
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

	if err := s.db.Create(&logEntry).Error; err != nil {
		log.Printf("保存操作日志失败: %v", err)
	}
}
