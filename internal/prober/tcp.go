// TCP 探测器实现
// 使用 net.DialTimeout 建立 TCP 连接检测目标 IP 和端口的可达性
package prober

import (
	"context"
	"fmt"
	"net"
	"time"
)

// TCPProber TCP 协议探测器
// 通过建立 TCP 连接检测目标 IP 和端口的可达性
type TCPProber struct{}

// Probe 执行 TCP 健康探测
// target: 目标 IP 地址
// port: 目标端口号
// timeout: 超时时间，超时未建立连接则标记为失败（需求 1.6）
// 连接成功则判定为健康（需求 1.2）
func (p *TCPProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult {
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

	// 构建目标地址
	address := fmt.Sprintf("%s:%d", target, port)

	// 创建带超时的拨号器
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	// 使用 DialContext 建立 TCP 连接，支持上下文取消
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	latency := time.Since(start)

	if err != nil {
		// 区分上下文取消和其他错误
		if ctx.Err() != nil {
			return ProbeResult{
				Success: false,
				Latency: latency,
				Time:    now,
				Error:   fmt.Sprintf("上下文已取消: %v", ctx.Err()),
			}
		}
		return ProbeResult{
			Success: false,
			Latency: latency,
			Time:    now,
			Error:   fmt.Sprintf("TCP 连接失败: %v", err),
		}
	}

	// 连接成功，关闭连接
	conn.Close()

	return ProbeResult{
		Success: true,
		Latency: latency,
		Time:    now,
		Error:   "",
	}
}
