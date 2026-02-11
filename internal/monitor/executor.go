// Package monitor 健康监控执行器
// 负责DNS解析和监控目标列表的管理
package monitor

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"time"

	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/notification"
	"dns-health-monitor/internal/prober"
	"dns-health-monitor/internal/provider"

	"gorm.io/gorm"
)

// ========== DNS解析函数类型 ==========

// LookupIPFunc DNS IP解析函数类型，支持按网络类型解析（ip4/ip6）
type LookupIPFunc func(ctx context.Context, network, host string) ([]net.IP, error)

// LookupCNAMEFunc DNS CNAME解析函数类型
type LookupCNAMEFunc func(ctx context.Context, host string) (string, error)

// ProviderFactory 创建 DNSProvider 实例的工厂函数类型
type ProviderFactory func(credential model.Credential) (provider.DNSProvider, error)

// ========== 监控执行器 ==========

// MonitorExecutor 监控执行器
// 负责执行DNS解析、更新监控目标列表、探测执行和状态管理
type MonitorExecutor struct {
	db              *gorm.DB                            // 数据库连接
	prober          func(protocol string) prober.Prober // 探测器工厂函数
	notifier        *notification.NotificationManager   // 通知管理器（可选）
	lookupIP        LookupIPFunc                        // DNS IP解析函数（可注入mock）
	lookupCNAME     LookupCNAMEFunc                     // DNS CNAME解析函数（可注入mock）
	providerFactory ProviderFactory                     // DNS服务商工厂函数（可选，用于凭证模式）
}

// NewMonitorExecutor 创建监控执行器实例
func NewMonitorExecutor(db *gorm.DB, notifier *notification.NotificationManager) *MonitorExecutor {
	resolver := &net.Resolver{}
	return &MonitorExecutor{
		db: db,
		prober: func(protocol string) prober.Prober {
			return prober.NewProber(prober.ProbeProtocol(protocol))
		},
		notifier: notifier,
		lookupIP: func(ctx context.Context, network, host string) ([]net.IP, error) {
			return resolver.LookupIP(ctx, network, host)
		},
		lookupCNAME: func(ctx context.Context, host string) (string, error) {
			return resolver.LookupCNAME(ctx, host)
		},
	}
}

// SetProviderFactory 设置DNS服务商工厂函数
// 设置后，有凭证的任务将通过云服务商API查询解析记录
func (e *MonitorExecutor) SetProviderFactory(factory ProviderFactory) {
	e.providerFactory = factory
}

// NewMonitorExecutorWithLookup 创建监控执行器实例（自定义DNS解析函数）
// 主要用于测试场景，可以注入mock的DNS解析函数
func NewMonitorExecutorWithLookup(
	db *gorm.DB,
	notifier *notification.NotificationManager,
	lookupIP LookupIPFunc,
	lookupCNAME LookupCNAMEFunc,
) *MonitorExecutor {
	return &MonitorExecutor{
		db: db,
		prober: func(protocol string) prober.Prober {
			return prober.NewProber(prober.ProbeProtocol(protocol))
		},
		notifier:    notifier,
		lookupIP:    lookupIP,
		lookupCNAME: lookupCNAME,
	}
}

// ========== DNS解析逻辑 ==========

// DNSResolveResult DNS解析结果
type DNSResolveResult struct {
	IPs        []string // 解析到的IP地址列表
	CNAMEValue string   // CNAME记录值（仅CNAME类型有值）
}

// resolveDNS 根据记录类型解析域名，返回IP地址列表
// 需求 2.1: 根据记录类型解析域名
// 需求 2.2: A记录返回所有IPv4地址
// 需求 2.3: AAAA记录返回所有IPv6地址
// 需求 2.4: A_AAAA记录返回所有IPv4和IPv6地址
// 需求 2.5: CNAME记录先解析CNAME记录，再解析CNAME指向的IP地址
func (e *MonitorExecutor) resolveDNS(ctx context.Context, task *model.HealthMonitorTask) (*DNSResolveResult, error) {
	// 如果任务配置了凭证且工厂函数可用，通过云服务商API查询解析记录
	if task.CredentialID != nil && *task.CredentialID > 0 && e.providerFactory != nil {
		return e.resolveViaProvider(ctx, task)
	}

	// 无凭证时，通过系统DNS解析器直接解析域名
	return e.resolveViaDNS(ctx, task)
}

// resolveViaProvider 通过云服务商API查询解析记录
func (e *MonitorExecutor) resolveViaProvider(ctx context.Context, task *model.HealthMonitorTask) (*DNSResolveResult, error) {
	// 从数据库加载凭证
	var credential model.Credential
	if err := e.db.First(&credential, *task.CredentialID).Error; err != nil {
		log.Printf("任务 %d: 加载凭证 %d 失败: %v，回退到DNS解析", task.ID, *task.CredentialID, err)
		return e.resolveViaDNS(ctx, task)
	}

	// 创建DNS Provider实例
	prov, err := e.providerFactory(credential)
	if err != nil {
		log.Printf("任务 %d: 创建DNS Provider失败: %v，回退到DNS解析", task.ID, err)
		return e.resolveViaDNS(ctx, task)
	}

	// 根据记录类型查询解析记录
	switch model.RecordType(task.RecordType) {
	case model.RecordTypeA:
		return e.listProviderRecords(ctx, prov, task, "A")
	case model.RecordTypeAAAA:
		return e.listProviderRecords(ctx, prov, task, "AAAA")
	case model.RecordTypeA_AAAA:
		return e.listProviderRecordsAll(ctx, prov, task)
	case model.RecordTypeCNAME:
		return e.listProviderCNAME(ctx, prov, task)
	default:
		return nil, fmt.Errorf("不支持的记录类型: %s", task.RecordType)
	}
}

// listProviderRecords 通过Provider查询指定类型的解析记录
func (e *MonitorExecutor) listProviderRecords(ctx context.Context, prov provider.DNSProvider, task *model.HealthMonitorTask, recordType string) (*DNSResolveResult, error) {
	records, err := prov.ListRecords(ctx, task.Domain, task.SubDomain, recordType)
	if err != nil {
		return nil, fmt.Errorf("通过API查询%s记录失败: %w", recordType, err)
	}

	ips := make([]string, 0, len(records))
	for _, r := range records {
		// 只取启用状态的记录
		if r.Status == "ENABLE" && r.Value != "" {
			ips = append(ips, r.Value)
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("通过API未查询到启用的%s记录", recordType)
	}

	log.Printf("任务 %d: 通过API查询到 %d 条%s记录", task.ID, len(ips), recordType)
	return &DNSResolveResult{IPs: deduplicateAndSort(ips)}, nil
}

// listProviderRecordsAll 通过Provider查询A和AAAA记录
func (e *MonitorExecutor) listProviderRecordsAll(ctx context.Context, prov provider.DNSProvider, task *model.HealthMonitorTask) (*DNSResolveResult, error) {
	var allIPs []string

	// 查询A记录
	aRecords, errA := prov.ListRecords(ctx, task.Domain, task.SubDomain, "A")
	if errA == nil {
		for _, r := range aRecords {
			if r.Status == "ENABLE" && r.Value != "" {
				allIPs = append(allIPs, r.Value)
			}
		}
	}

	// 查询AAAA记录
	aaaaRecords, errAAAA := prov.ListRecords(ctx, task.Domain, task.SubDomain, "AAAA")
	if errAAAA == nil {
		for _, r := range aaaaRecords {
			if r.Status == "ENABLE" && r.Value != "" {
				allIPs = append(allIPs, r.Value)
			}
		}
	}

	if errA != nil && errAAAA != nil {
		return nil, fmt.Errorf("通过API查询A记录失败: %v; AAAA记录失败: %v", errA, errAAAA)
	}

	if len(allIPs) == 0 {
		return nil, fmt.Errorf("通过API未查询到启用的A/AAAA记录")
	}

	log.Printf("任务 %d: 通过API查询到 %d 条A/AAAA记录", task.ID, len(allIPs))
	return &DNSResolveResult{IPs: deduplicateAndSort(allIPs)}, nil
}

// listProviderCNAME 通过Provider查询CNAME记录，再解析CNAME指向的IP
func (e *MonitorExecutor) listProviderCNAME(ctx context.Context, prov provider.DNSProvider, task *model.HealthMonitorTask) (*DNSResolveResult, error) {
	records, err := prov.ListRecords(ctx, task.Domain, task.SubDomain, "CNAME")
	if err != nil {
		return nil, fmt.Errorf("通过API查询CNAME记录失败: %w", err)
	}

	// 取第一条启用的CNAME记录
	var cnameValue string
	for _, r := range records {
		if r.Status == "ENABLE" && r.Value != "" {
			cnameValue = trimTrailingDot(r.Value)
			break
		}
	}

	if cnameValue == "" {
		return nil, fmt.Errorf("通过API未查询到启用的CNAME记录")
	}

	// CNAME指向的IP仍然通过系统DNS解析（因为CNAME目标可能不在同一服务商）
	ips, err := e.resolveAllIPs(ctx, cnameValue)
	if err != nil {
		return nil, fmt.Errorf("解析CNAME目标 '%s' 的IP失败: %w", cnameValue, err)
	}

	log.Printf("任务 %d: 通过API查询到CNAME=%s，解析到 %d 个IP", task.ID, cnameValue, len(ips))
	return &DNSResolveResult{
		IPs:        ips,
		CNAMEValue: cnameValue,
	}, nil
}

// resolveViaDNS 通过系统DNS解析器解析域名（无凭证模式）
func (e *MonitorExecutor) resolveViaDNS(ctx context.Context, task *model.HealthMonitorTask) (*DNSResolveResult, error) {
	// 构建完整域名
	fullDomain := buildFullDomain(task.Domain, task.SubDomain)

	switch model.RecordType(task.RecordType) {
	case model.RecordTypeA:
		// 需求 2.2: A记录返回所有IPv4地址
		ips, err := e.resolveIPv4(ctx, fullDomain)
		if err != nil {
			return nil, fmt.Errorf("解析A记录失败: %w", err)
		}
		return &DNSResolveResult{IPs: ips}, nil

	case model.RecordTypeAAAA:
		// 需求 2.3: AAAA记录返回所有IPv6地址
		ips, err := e.resolveIPv6(ctx, fullDomain)
		if err != nil {
			return nil, fmt.Errorf("解析AAAA记录失败: %w", err)
		}
		return &DNSResolveResult{IPs: ips}, nil

	case model.RecordTypeA_AAAA:
		// 需求 2.4: A_AAAA记录返回所有IPv4和IPv6地址
		ips, err := e.resolveAllIPs(ctx, fullDomain)
		if err != nil {
			return nil, fmt.Errorf("解析A_AAAA记录失败: %w", err)
		}
		return &DNSResolveResult{IPs: ips}, nil

	case model.RecordTypeCNAME:
		// 需求 2.5: CNAME记录先解析CNAME记录，再解析CNAME指向的IP地址
		result, err := e.resolveCNAME(ctx, fullDomain)
		if err != nil {
			return nil, fmt.Errorf("解析CNAME记录失败: %w", err)
		}
		return result, nil

	default:
		return nil, fmt.Errorf("不支持的记录类型: %s", task.RecordType)
	}
}

// resolveIPv4 解析域名的所有IPv4地址
// 需求 2.2: A记录返回所有IPv4地址
func (e *MonitorExecutor) resolveIPv4(ctx context.Context, domain string) ([]string, error) {
	addrs, err := e.lookupIP(ctx, "ip4", domain)
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.String())
	}

	return deduplicateAndSort(ips), nil
}

// resolveIPv6 解析域名的所有IPv6地址
// 需求 2.3: AAAA记录返回所有IPv6地址
func (e *MonitorExecutor) resolveIPv6(ctx context.Context, domain string) ([]string, error) {
	addrs, err := e.lookupIP(ctx, "ip6", domain)
	if err != nil {
		return nil, err
	}

	ips := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.String())
	}

	return deduplicateAndSort(ips), nil
}

// resolveAllIPs 解析域名的所有IPv4和IPv6地址
// 需求 2.4: A_AAAA记录返回所有IPv4和IPv6地址
func (e *MonitorExecutor) resolveAllIPs(ctx context.Context, domain string) ([]string, error) {
	var allIPs []string

	// 解析IPv4地址
	ipv4Addrs, err4 := e.lookupIP(ctx, "ip4", domain)
	if err4 == nil {
		for _, addr := range ipv4Addrs {
			allIPs = append(allIPs, addr.String())
		}
	}

	// 解析IPv6地址
	ipv6Addrs, err6 := e.lookupIP(ctx, "ip6", domain)
	if err6 == nil {
		for _, addr := range ipv6Addrs {
			allIPs = append(allIPs, addr.String())
		}
	}

	// 如果两种解析都失败，返回错误
	if err4 != nil && err6 != nil {
		return nil, fmt.Errorf("IPv4解析失败: %v; IPv6解析失败: %v", err4, err6)
	}

	if len(allIPs) == 0 {
		return nil, fmt.Errorf("域名 '%s' 未解析到任何IP地址", domain)
	}

	return deduplicateAndSort(allIPs), nil
}

// resolveCNAME 解析CNAME记录，先获取CNAME值，再解析CNAME指向的IP地址
// 需求 2.5: CNAME记录先解析CNAME记录，再解析CNAME指向的IP地址
func (e *MonitorExecutor) resolveCNAME(ctx context.Context, domain string) (*DNSResolveResult, error) {
	// 第一步：解析CNAME记录值
	cnameValue, err := e.lookupCNAME(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("查询CNAME记录失败: %w", err)
	}

	// 去除末尾的点号（DNS标准格式）
	cnameValue = trimTrailingDot(cnameValue)

	if cnameValue == "" || cnameValue == domain {
		return nil, fmt.Errorf("域名 '%s' 没有CNAME记录", domain)
	}

	// 第二步：解析CNAME指向的所有IP地址（IPv4 + IPv6）
	ips, err := e.resolveAllIPs(ctx, cnameValue)
	if err != nil {
		return nil, fmt.Errorf("解析CNAME目标 '%s' 的IP失败: %w", cnameValue, err)
	}

	return &DNSResolveResult{
		IPs:        ips,
		CNAMEValue: cnameValue,
	}, nil
}

// ========== 监控目标列表更新逻辑 ==========

// updateTargets 更新监控目标列表
// 对比当前数据库中的IP列表和新解析的IP列表，实现增量更新：
// - 需求 2.7: 解析到新的IP地址时加入监控列表
// - 需求 2.8: IP不再出现在解析结果中时停止监控
// - 需求 8.5: CNAME记录值发生变化时更新监控的IP列表
func (e *MonitorExecutor) updateTargets(ctx context.Context, taskID uint, result *DNSResolveResult) error {
	// 查询当前数据库中该任务的所有监控目标
	var currentTargets []model.HealthMonitorTarget
	if err := e.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Find(&currentTargets).Error; err != nil {
		return fmt.Errorf("查询当前监控目标失败: %w", err)
	}

	// 构建当前IP集合（用于快速查找）
	currentIPSet := make(map[string]bool, len(currentTargets))
	for _, target := range currentTargets {
		currentIPSet[target.IP] = true
	}

	// 构建新IP集合
	newIPSet := make(map[string]bool, len(result.IPs))
	for _, ip := range result.IPs {
		newIPSet[ip] = true
	}

	// 使用事务执行增量更新
	return e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 需求 8.5: 检测CNAME记录值是否发生变化
		if result.CNAMEValue != "" {
			for i := range currentTargets {
				if currentTargets[i].CNAMEValue != "" && currentTargets[i].CNAMEValue != result.CNAMEValue {
					// CNAME值已变化，更新所有现有目标的CNAMEValue
					log.Printf("任务 %d: CNAME记录值变化: %s → %s", taskID, currentTargets[i].CNAMEValue, result.CNAMEValue)
					if err := tx.Model(&model.HealthMonitorTarget{}).
						Where("task_id = ? AND cname_value = ?", taskID, currentTargets[i].CNAMEValue).
						Update("cname_value", result.CNAMEValue).Error; err != nil {
						return fmt.Errorf("更新CNAME值失败: %w", err)
					}
					break // 只需要检测一次，所有旧CNAME值相同
				}
			}
		}

		// 需求 2.7: 添加新出现的IP到监控列表
		for _, ip := range result.IPs {
			if !currentIPSet[ip] {
				target := model.HealthMonitorTarget{
					TaskID:       taskID,
					IP:           ip,
					CNAMEValue:   result.CNAMEValue,
					HealthStatus: string(model.HealthStatusUnknown), // 需求 4.3: 初始状态为未知
				}
				if err := tx.Create(&target).Error; err != nil {
					return fmt.Errorf("添加监控目标 '%s' 失败: %w", ip, err)
				}
				log.Printf("任务 %d: 新增监控目标 IP=%s CNAMEValue=%s", taskID, ip, result.CNAMEValue)
			}
		}

		// 需求 2.8: 删除不再出现在解析结果中的IP
		for _, target := range currentTargets {
			if !newIPSet[target.IP] {
				if err := tx.Delete(&target).Error; err != nil {
					return fmt.Errorf("删除监控目标 '%s' 失败: %w", target.IP, err)
				}
				log.Printf("任务 %d: 移除监控目标 IP=%s", taskID, target.IP)
			}
		}

		return nil
	})
}

// getActiveTargets 获取任务的所有活跃监控目标
func (e *MonitorExecutor) getActiveTargets(ctx context.Context, taskID uint) ([]model.HealthMonitorTarget, error) {
	var targets []model.HealthMonitorTarget
	if err := e.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Find(&targets).Error; err != nil {
		return nil, fmt.Errorf("查询监控目标失败: %w", err)
	}
	return targets, nil
}

// ========== 探测执行逻辑 ==========

// probeTarget 对单个监控目标执行探测
// 需求 3.1: 对所有监控中的IP地址执行探测
// 需求 3.2-3.6: 支持ICMP/TCP/UDP/HTTP/HTTPS探测（复用现有prober）
// 需求 3.7: 探测成功时记录成功状态和响应延迟
// 需求 3.8: 探测失败时记录失败状态和错误信息
func (e *MonitorExecutor) probeTarget(ctx context.Context, task *model.HealthMonitorTask, target *model.HealthMonitorTarget) *prober.ProbeResult {
	// 根据探测协议创建对应的探测器
	p := e.prober(task.ProbeProtocol)
	if p == nil {
		errMsg := fmt.Sprintf("不支持的探测协议: %s", task.ProbeProtocol)
		log.Printf("任务 %d: %s", task.ID, errMsg)
		return &prober.ProbeResult{
			Success: false,
			Latency: 0,
			Time:    time.Now(),
			Error:   errMsg,
		}
	}

	// 将超时时间从毫秒转换为Duration
	timeout := time.Duration(task.TimeoutMs) * time.Millisecond

	// 执行探测
	result := p.Probe(ctx, target.IP, task.ProbePort, timeout)

	// 记录探测日志
	if result.Success {
		log.Printf("任务 %d: 探测 IP=%s 成功, 延迟=%v", task.ID, target.IP, result.Latency)
	} else {
		log.Printf("任务 %d: 探测 IP=%s 失败, 错误=%s", task.ID, target.IP, result.Error)
	}

	return &result
}

// saveResult 保存探测结果到数据库
// 需求 5.1: 将探测结果存储到数据库
// 需求 5.2: 记录任务ID、IP地址、成功状态、延迟和时间戳
// 需求 5.3: 探测失败时同时记录错误信息
func (e *MonitorExecutor) saveResult(ctx context.Context, taskID uint, ip string, result *prober.ProbeResult) error {
	record := model.HealthMonitorResult{
		TaskID:    taskID,
		IP:        ip,
		Success:   result.Success,
		LatencyMs: int(result.Latency.Milliseconds()),
		ErrorMsg:  result.Error,
		ProbedAt:  result.Time,
	}

	if err := e.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("保存探测结果失败: %w", err)
	}

	return nil
}

// Execute 执行一次完整的监控周期
// 流程: DNS解析 → 更新目标 → 探测每个IP → 保存结果
// 健康状态更新和通知触发将在后续任务中实现
func (e *MonitorExecutor) Execute(ctx context.Context, task *model.HealthMonitorTask) error {
	// 1. DNS解析获取IP列表
	resolveResult, err := e.resolveDNS(ctx, task)
	if err != nil {
		// 需求 2.6: DNS解析失败时记录错误信息，在下个周期重试
		log.Printf("任务 %d: DNS解析失败: %v", task.ID, err)
		return fmt.Errorf("DNS解析失败: %w", err)
	}

	// 2. 更新监控目标列表
	if err := e.updateTargets(ctx, task.ID, resolveResult); err != nil {
		log.Printf("任务 %d: 更新监控目标失败: %v", task.ID, err)
		return fmt.Errorf("更新监控目标失败: %w", err)
	}

	// 3. 获取所有活跃监控目标
	targets, err := e.getActiveTargets(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("获取监控目标失败: %w", err)
	}

	// 4. 对每个活跃目标执行探测并保存结果
	for i := range targets {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 执行探测
		result := e.probeTarget(ctx, task, &targets[i])

		// 保存探测结果
		if err := e.saveResult(ctx, task.ID, targets[i].IP, result); err != nil {
			log.Printf("任务 %d: 保存探测结果失败 IP=%s: %v", task.ID, targets[i].IP, err)
			// 保存失败不中断整个探测流程，继续处理下一个目标
		}

		// 5. 更新健康状态
		oldStatus, newStatus := e.updateHealthStatus(ctx, task, &targets[i], result)

		// 6. 检查是否需要发送通知
		e.checkAndNotify(ctx, task, &targets[i], oldStatus, newStatus)
	}

	// 7. 需求 8.6, 8.7: 检查CNAME类型任务的失败阈值
	if model.RecordType(task.RecordType) == model.RecordTypeCNAME {
		// 重新获取最新的目标状态（探测后状态已更新）
		updatedTargets, err := e.getActiveTargets(ctx, task.ID)
		if err != nil {
			log.Printf("任务 %d: 获取更新后的监控目标失败: %v", task.ID, err)
		} else {
			thresholdReached := e.checkCNAMEThreshold(task, updatedTargets)
			if thresholdReached {
				log.Printf("任务 %d: CNAME失败阈值已触发", task.ID)
				// CNAME阈值触发时，对所有不健康的IP发送连续失败告警
				if e.notifier != nil {
					failedIPs := make([]string, 0)
					for _, t := range updatedTargets {
						if t.HealthStatus == string(model.HealthStatusUnhealthy) {
							failedIPs = append(failedIPs, t.IP)
						}
					}
					if len(failedIPs) > 0 {
						e.notifier.Notify(notification.NotificationEvent{
							Type:          model.EventTypeConsecutiveFail,
							TaskID:        task.ID,
							Domain:        task.Domain,
							SubDomain:     task.SubDomain,
							OccurredAt:    time.Now(),
							FailCount:     len(failedIPs),
							FailedIPs:     failedIPs,
							ProbeProtocol: task.ProbeProtocol,
							ProbePort:     task.ProbePort,
							HealthStatus:  string(model.HealthStatusUnhealthy),
						})
					}
				}
			}
		}
	}

	return nil
}

// ========== 健康状态管理 ==========

// updateHealthStatus 更新监控目标的健康状态
// 根据探测结果更新连续失败/成功计数、健康状态和平均延迟
// 返回旧状态和新状态，以便后续通知逻辑使用
//
// 需求 3.10: 每次探测后更新IP的连续成功次数和连续失败次数
// 需求 4.1: 连续失败次数达到失败阈值时，标记为不健康(unhealthy)
// 需求 4.2: 连续成功次数达到恢复阈值时，标记为健康(healthy)
// 需求 4.4: 状态从健康变为不健康时记录状态变更事件
// 需求 4.5: 状态从不健康变为健康时记录状态恢复事件
// 需求 4.6: 维护最近10次成功探测的平均延迟
func (e *MonitorExecutor) updateHealthStatus(ctx context.Context, task *model.HealthMonitorTask, target *model.HealthMonitorTarget, result *prober.ProbeResult) (oldStatus, newStatus string) {
	oldStatus = target.HealthStatus
	now := result.Time

	if result.Success {
		// 探测成功: 连续成功次数+1，连续失败次数归零
		target.ConsecutiveSuccesses++
		target.ConsecutiveFails = 0
	} else {
		// 探测失败: 连续失败次数+1，连续成功次数归零
		target.ConsecutiveFails++
		target.ConsecutiveSuccesses = 0
	}

	// 更新最后探测时间
	target.LastProbeAt = &now

	// 状态转换逻辑
	// 需求 4.1: 连续失败次数达到失败阈值时，标记为不健康
	if target.ConsecutiveFails >= task.FailThreshold {
		target.HealthStatus = string(model.HealthStatusUnhealthy)
	}
	// 需求 4.2: 连续成功次数达到恢复阈值时，标记为健康
	if target.ConsecutiveSuccesses >= task.RecoverThreshold {
		target.HealthStatus = string(model.HealthStatusHealthy)
	}

	newStatus = target.HealthStatus

	// 需求 4.6: 计算最近10次成功探测的平均延迟
	target.AvgLatencyMs = e.calculateAvgLatency(ctx, task.ID, target.IP)

	// 保存更新到数据库
	if err := e.db.WithContext(ctx).Save(target).Error; err != nil {
		log.Printf("任务 %d: 更新健康状态失败 IP=%s: %v", task.ID, target.IP, err)
		return oldStatus, newStatus
	}

	// 记录状态变更日志
	// 需求 4.4: 状态从健康变为不健康时记录状态变更事件
	if oldStatus == string(model.HealthStatusHealthy) && newStatus == string(model.HealthStatusUnhealthy) {
		log.Printf("任务 %d: IP=%s 状态变更: 健康 → 不健康 (连续失败 %d 次)", task.ID, target.IP, target.ConsecutiveFails)
	}
	// 需求 4.5: 状态从不健康变为健康时记录状态恢复事件
	if oldStatus == string(model.HealthStatusUnhealthy) && newStatus == string(model.HealthStatusHealthy) {
		log.Printf("任务 %d: IP=%s 状态恢复: 不健康 → 健康 (连续成功 %d 次)", task.ID, target.IP, target.ConsecutiveSuccesses)
	}

	return oldStatus, newStatus
}

// calculateAvgLatency 计算最近10次成功探测的平均延迟
// 需求 4.6: 为每个IP维护最近的平均延迟(基于最近10次成功探测)
func (e *MonitorExecutor) calculateAvgLatency(ctx context.Context, taskID uint, ip string) int {
	var results []model.HealthMonitorResult
	if err := e.db.WithContext(ctx).
		Where("task_id = ? AND ip = ? AND success = ?", taskID, ip, true).
		Order("probed_at DESC").
		Limit(10).
		Find(&results).Error; err != nil {
		log.Printf("任务 %d: 查询探测结果失败 IP=%s: %v", taskID, ip, err)
		return 0
	}

	if len(results) == 0 {
		return 0
	}

	// 计算平均延迟
	var totalLatency int
	for _, r := range results {
		totalLatency += r.LatencyMs
	}
	return totalLatency / len(results)
}

// ========== 通知触发逻辑 ==========

// checkAndNotify 检查是否需要发送通知并触发通知
// 根据状态变化和连续失败情况，判断是否需要发送通知
//
// 需求 6.1: IP状态变为不健康且通知已启用时，发送故障通知
// 需求 6.2: IP状态恢复为健康且通知已启用时，发送恢复通知
// 需求 6.3: IP连续失败达到阈值且通知已启用时，发送连续失败告警
// 需求 6.4: 通知未启用时不发送任何通知（由NotificationManager内部处理）
// 需求 6.5: 通知包含任务名称、域名、IP地址、状态和时间信息
// 需求 6.6: 记录通知日志（由NotificationManager内部处理）
func (e *MonitorExecutor) checkAndNotify(ctx context.Context, task *model.HealthMonitorTask, target *model.HealthMonitorTarget, oldStatus, newStatus string) {
	// 如果通知管理器未配置，直接返回
	if e.notifier == nil {
		return
	}

	now := time.Now()

	// 需求 6.1: 状态从健康/未知变为不健康时，发送故障通知
	if newStatus == string(model.HealthStatusUnhealthy) && oldStatus != string(model.HealthStatusUnhealthy) {
		log.Printf("任务 %d: IP=%s 触发故障通知 (状态: %s → %s)", task.ID, target.IP, oldStatus, newStatus)
		e.notifier.Notify(notification.NotificationEvent{
			Type:          model.EventTypeFailover,
			TaskID:        task.ID,
			Domain:        task.Domain,
			SubDomain:     task.SubDomain,
			OccurredAt:    now,
			OriginalValue: target.IP,
			HealthStatus:  newStatus,
		})
	}

	// 需求 6.2: 状态从不健康变为健康时，发送恢复通知
	if newStatus == string(model.HealthStatusHealthy) && oldStatus == string(model.HealthStatusUnhealthy) {
		log.Printf("任务 %d: IP=%s 触发恢复通知 (状态: %s → %s)", task.ID, target.IP, oldStatus, newStatus)
		e.notifier.Notify(notification.NotificationEvent{
			Type:           model.EventTypeRecovery,
			TaskID:         task.ID,
			Domain:         task.Domain,
			SubDomain:      task.SubDomain,
			OccurredAt:     now,
			RecoveredValue: target.IP,
		})
	}

	// 需求 6.3: 连续失败次数刚好达到失败阈值时，发送连续失败告警
	// 仅在刚好达到阈值时发送，避免重复通知
	if target.ConsecutiveFails == task.FailThreshold && target.ConsecutiveFails > 0 {
		log.Printf("任务 %d: IP=%s 触发连续失败告警 (连续失败 %d 次)", task.ID, target.IP, target.ConsecutiveFails)
		e.notifier.Notify(notification.NotificationEvent{
			Type:          model.EventTypeConsecutiveFail,
			TaskID:        task.ID,
			Domain:        task.Domain,
			SubDomain:     task.SubDomain,
			OccurredAt:    now,
			FailCount:     target.ConsecutiveFails,
			FailedIPs:     []string{target.IP},
			ProbeProtocol: task.ProbeProtocol,
			ProbePort:     task.ProbePort,
			HealthStatus:  newStatus,
		})
	}
}

// ========== CNAME失败阈值判断 ==========

// checkCNAMEThreshold 检查CNAME类型任务是否达到失败阈值
// 需求 8.6: 当FailThresholdType为"count"时，按失败IP个数判断
// 需求 8.7: 当FailThresholdType为"percent"时，按失败IP占比判断
// 返回: 是否达到失败阈值
func (e *MonitorExecutor) checkCNAMEThreshold(task *model.HealthMonitorTask, targets []model.HealthMonitorTarget) bool {
	// 仅对CNAME类型任务进行阈值判断
	if model.RecordType(task.RecordType) != model.RecordTypeCNAME {
		return false
	}

	// 没有目标时不触发阈值
	totalCount := len(targets)
	if totalCount == 0 {
		return false
	}

	// 统计不健康的IP数量
	unhealthyCount := 0
	for _, target := range targets {
		if target.HealthStatus == string(model.HealthStatusUnhealthy) {
			unhealthyCount++
		}
	}

	// 根据阈值类型判断
	switch model.FailThresholdType(task.FailThresholdType) {
	case model.FailThresholdCount:
		// 需求 8.6: 按失败IP个数判断
		reached := unhealthyCount >= task.FailThresholdValue
		if reached {
			log.Printf("任务 %d: CNAME失败阈值(count)已达到: 不健康IP数=%d >= 阈值=%d",
				task.ID, unhealthyCount, task.FailThresholdValue)
		}
		return reached

	case model.FailThresholdPercent:
		// 需求 8.7: 按失败IP占比判断（百分比）
		unhealthyPercent := float64(unhealthyCount) / float64(totalCount) * 100
		reached := unhealthyPercent >= float64(task.FailThresholdValue)
		if reached {
			log.Printf("任务 %d: CNAME失败阈值(percent)已达到: 不健康IP占比=%.1f%% >= 阈值=%d%%",
				task.ID, unhealthyPercent, task.FailThresholdValue)
		}
		return reached

	default:
		log.Printf("任务 %d: 未知的阈值类型: %s，默认使用count模式", task.ID, task.FailThresholdType)
		return unhealthyCount >= task.FailThresholdValue
	}
}

// ========== 辅助函数 ==========

// buildFullDomain 构建完整域名
// 如果子域名为"@"或空，则返回主域名；否则返回"子域名.主域名"
func buildFullDomain(domain, subDomain string) string {
	if subDomain == "" || subDomain == "@" {
		return domain
	}
	return subDomain + "." + domain
}

// trimTrailingDot 去除域名末尾的点号
func trimTrailingDot(domain string) string {
	if len(domain) > 0 && domain[len(domain)-1] == '.' {
		return domain[:len(domain)-1]
	}
	return domain
}

// deduplicateAndSort 对IP列表去重并排序，确保结果稳定
func deduplicateAndSort(ips []string) []string {
	seen := make(map[string]bool, len(ips))
	result := make([]string, 0, len(ips))

	for _, ip := range ips {
		if !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}

	sort.Strings(result)
	return result
}
