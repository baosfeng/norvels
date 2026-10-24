// Command healthcheck 是镜像内置的健康检查器。
//
// distroless/static 运行镜像里没有 shell、curl 或 wget，Dockerfile 的
// HEALTHCHECK 无法直接调用 HTTP 客户端；因此在构建阶段编译这个静态小工具，
// 由它请求 GET /api/health 并按响应判定容器健康状态。
//
// 为了把镜像体积压到最小，这里只用 net + bufio 手写一个最小 HTTP/1.1 客户端，
// 不引入 net/http 与 crypto/tls（健康检查固定走明文回环地址）。
//
// 环境变量：
//
//	NORVELS_HEALTH_URL  健康检查地址，默认 http://127.0.0.1:8080/api/health
//
// 判定规则：HTTP 200 且响应 JSON 里 status == "ok" 视为健康，退出码 0；
// 否则输出原因并以退出码 1 结束。
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// defaultURL 与服务默认监听地址（NORVELS_ADDR=:8080）保持一致。
const defaultURL = "http://127.0.0.1:8080/api/health"

// maxBodyBytes 限制读取的响应体大小，避免异常响应拖住探针。
const maxBodyBytes = 4 << 10

func main() {
	raw := os.Getenv("NORVELS_HEALTH_URL")
	if raw == "" {
		raw = defaultURL
	}
	if err := check(raw, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("healthcheck: ok")
}

// check 请求 raw 指向的 /api/health 并在 timeout 内判定服务是否健康。
func check(raw string, timeout time.Duration) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("解析地址 %q: %w", raw, err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("仅支持 http:// 地址，实际 %q", u.Scheme)
	}
	host := u.Host
	if host == "" {
		return fmt.Errorf("地址 %q 缺少主机名", raw)
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "80")
	}

	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return fmt.Errorf("连接 %s: %w", host, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("设置超时: %w", err)
	}

	req := "GET " + u.RequestURI() + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"User-Agent: norvels-healthcheck\r\n" +
		"Accept: application/json\r\n" +
		"Connection: close\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		return fmt.Errorf("发送请求: %w", err)
	}

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("读取状态行: %w", err)
	}
	fields := strings.Fields(statusLine)
	if len(fields) < 2 || fields[1] != "200" {
		return fmt.Errorf("GET %s: 状态行 %q", raw, strings.TrimSpace(statusLine))
	}
	// 跳过响应头。
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("读取响应头: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	body, err := io.ReadAll(io.LimitReader(br, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("读取响应体: %w", err)
	}
	// 服务端用 encoding/json 序列化，键值间没有空格；仍去掉空格以容忍格式差异。
	if !strings.Contains(strings.ReplaceAll(string(body), " ", ""), "\"status\":\"ok\"") {
		return fmt.Errorf("响应体 %q 中 status 不是 ok", strings.TrimSpace(string(body)))
	}
	return nil
}
