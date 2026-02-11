// UDP 探测器实现
// 使用 net.Dialer 发送 UDP 数据包检测目标 IP 和端口的可达性
// UDP 是无连接协议，"成功"意味着可以发送数据包且未收到 ICMP 端口不可达错误
package prober

import (
	"context"
	"fmt"
	"net"
	"time"
)

// UDPProber UDP 协议探测器
// 通过发送 UDP 数据包检测目标 IP 和端口的可达性
type UDPProber struct{}

// Probe 执行 UDP 健康探测
// target: 目标 IP 地址
// port: 目标端口号
// timeout: 超时时间，超时未收到响应则根据情况判定（需求 1.6）
// UDP 无连接，发送数据包后如果未收到 ICMP 端口不可达错误则判定为健康（需求 1.3）
func (p *UDPProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult {
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

	// 使用 DialContext 创建 UDP 连接，支持上下文取消
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		latency := time.Since(start)
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
			Error:   fmt.Sprintf("UDP 连接创建失败: %v", err),
		}
	}
	defer conn.Close()

	// 设置读写截止时间
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	// 发送一个小的探测数据包
	probeData := []byte{0x00}
	_, err = conn.Write(probeData)
	if err != nil {
		latency := time.Since(start)
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
			Error:   fmt.Sprintf("UDP 数据包发送失败: %v", err),
		}
	}

	// 尝试读取响应，检测是否收到 ICMP 端口不可达错误
	// 对于 UDP，如果目标端口未监听，操作系统通常会返回 ICMP 端口不可达
	// 这会在 Read 时表现为 "connection refused" 错误
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	latency := time.Since(start)

	if err != nil {
		// 检查上下文是否已取消
		if ctx.Err() != nil {
			return ProbeResult{
				Success: false,
				Latency: latency,
				Time:    now,
				Error:   fmt.Sprintf("上下文已取消: %v", ctx.Err()),
			}
		}

		// 检查是否为 ICMP 端口不可达（connection refused）
		if opErr, ok := err.(*net.OpError); ok {
			if opErr.Err.Error() == "read: connection refused" ||
				isConnectionRefused(opErr) {
				return ProbeResult{
					Success: false,
					Latency: latency,
					Time:    now,
					Error:   fmt.Sprintf("UDP 端口不可达: %v", err),
				}
			}
		}

		// 超时错误表示没有收到 ICMP 错误，认为端口可达
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return ProbeResult{
				Success: true,
				Latency: latency,
				Time:    now,
				Error:   "",
			}
		}

		// 其他错误视为探测失败
		return ProbeResult{
			Success: false,
			Latency: latency,
			Time:    now,
			Error:   fmt.Sprintf("UDP 读取响应失败: %v", err),
		}
	}

	// 收到响应数据，探测成功
	return ProbeResult{
		Success: true,
		Latency: latency,
		Time:    now,
		Error:   "",
	}
}

// isConnectionRefused 检查错误是否为连接被拒绝（ICMP 端口不可达）
func isConnectionRefused(opErr *net.OpError) bool {
	// 不同操作系统的错误信息可能不同，统一检查
	errStr := opErr.Err.Error()
	return errStr == "connection refused" ||
		errStr == "read: connection refused" ||
		// Windows 系统可能返回不同的错误信息
		errStr == "connectex: No connection could be made because the target machine actively refused it."
}
