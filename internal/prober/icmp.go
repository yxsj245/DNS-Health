// ICMP 探测器实现
// 使用 pro-bing 库发送 ICMP Echo 请求检测目标 IP 的可达性
package prober

import (
	"context"
	"fmt"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// ICMPProber ICMP 协议探测器
// 使用 ICMP Echo 请求检测目标 IP 的可达性
type ICMPProber struct{}

// Probe 执行 ICMP 健康探测
// target: 目标 IP 地址
// port: 端口参数，ICMP 协议忽略此参数
// timeout: 超时时间，超时未收到响应则标记为失败（需求 1.6）
func (p *ICMPProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult {
	now := time.Now()

	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   fmt.Sprintf("上下文已取消: %v", ctx.Err()),
		}
	default:
	}

	// 创建 pinger 实例
	pinger, err := probing.NewPinger(target)
	if err != nil {
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   fmt.Sprintf("创建 pinger 失败: %v", err),
		}
	}

	// 配置 pinger
	pinger.Count = 1           // 只发送一个 ICMP Echo 请求
	pinger.Timeout = timeout   // 设置超时时间（需求 1.6）
	pinger.SetPrivileged(true) // 使用原始 ICMP socket（需要 root 权限）

	// 监听上下文取消，及时停止 pinger
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			pinger.Stop()
		case <-done:
		}
	}()

	// 执行 ping 操作
	err = pinger.Run()
	close(done)

	// 检查上下文是否在 ping 过程中被取消
	if ctx.Err() != nil {
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   fmt.Sprintf("上下文已取消: %v", ctx.Err()),
		}
	}

	if err != nil {
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   fmt.Sprintf("ICMP 探测失败: %v", err),
		}
	}

	// 获取统计信息
	stats := pinger.Statistics()

	// 判断是否收到回复
	if stats.PacketsRecv == 0 {
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   "ICMP 探测超时: 未收到回复",
		}
	}

	// 探测成功，返回 RTT 作为延迟
	return ProbeResult{
		Success: true,
		Latency: stats.AvgRtt,
		Time:    now,
		Error:   "",
	}
}
