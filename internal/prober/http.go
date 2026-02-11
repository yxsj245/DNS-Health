// HTTP 探测器实现
// 使用 http.Client 发送 HTTP GET 请求检测目标 IP 和端口的可达性
package prober

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// HTTPProber HTTP 协议探测器
// 通过发送 HTTP GET 请求并检查响应状态码判定健康状态
// 状态码 2xx/3xx (200-399) 为健康，其他为不健康
type HTTPProber struct{}

// Probe 执行 HTTP 健康探测
// target: 目标 IP 地址
// port: 目标端口号
// timeout: 超时时间，超时未收到响应则标记为失败（需求 1.6）
// 状态码 200-399 判定为健康（需求 1.4）
func (p *HTTPProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult {
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

	// 构建目标 URL
	url := fmt.Sprintf("http://%s:%d/", target, port)

	// 创建带超时的 HTTP 客户端
	// 不自动跟随重定向，以便正确检测 3xx 状态码
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 不跟随重定向，直接返回第一个响应
			return http.ErrUseLastResponse
		},
	}

	// 创建带上下文的 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{
			Success: false,
			Latency: 0,
			Time:    now,
			Error:   fmt.Sprintf("创建 HTTP 请求失败: %v", err),
		}
	}

	// 发送请求并计算延迟
	start := time.Now()
	resp, err := client.Do(req)
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
			Error:   fmt.Sprintf("HTTP 请求失败: %v", err),
		}
	}
	defer resp.Body.Close()

	// 判定状态码：2xx/3xx (200-399) 为健康
	success := resp.StatusCode >= 200 && resp.StatusCode < 400

	if !success {
		return ProbeResult{
			Success: false,
			Latency: latency,
			Time:    now,
			Error:   fmt.Sprintf("HTTP 状态码异常: %d", resp.StatusCode),
		}
	}

	return ProbeResult{
		Success: true,
		Latency: latency,
		Time:    now,
		Error:   "",
	}
}
