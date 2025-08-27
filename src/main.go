/* clang-format off */
/*
 * @file main.go
 * @date 2025-08-27
 * @license MIT License
 *
 * Copyright (c) 2025 BinRacer <native.lab@outlook.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
/* clang-format on */
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/term"

	"github.com/sirupsen/logrus"
)

// 颜色常量定义
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorWhite   = "\033[37m"
	colorBold    = "\033[1m"
)

// IPInfo 存储IP地址、Ping时间和端口状态
type IPInfo struct {
	IP          string
	PingTime    time.Duration
	PingSuccess bool
	Port22Open  bool
	Port80Open  bool
	Port443Open bool
}

// 全局日志变量
var logger *logrus.Logger

// CustomConsoleFormatter 自定义控制台日志格式
type CustomConsoleFormatter struct{}

func (f *CustomConsoleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006-01-02 15:04:05.000")
	level := strings.ToUpper(entry.Level.String())
	message := entry.Message

	// 根据日志级别选择颜色
	var color string
	switch entry.Level {
	case logrus.DebugLevel:
		color = colorBlue
	case logrus.InfoLevel:
		color = colorGreen
	case logrus.WarnLevel:
		color = colorYellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		color = colorRed
	default:
		color = colorReset
	}

	formatted := fmt.Sprintf("%s%s [%s] %s%s\n", color, timestamp, level, message, colorReset)
	return []byte(formatted), nil
}

// ConsoleHook 自定义Hook用于控制台输出
type ConsoleHook struct {
	Formatter logrus.Formatter
}

// Levels 返回所有日志级别
func (hook *ConsoleHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 实现Hook接口，输出到控制台
func (hook *ConsoleHook) Fire(entry *logrus.Entry) error {
	formatted, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	fmt.Print(string(formatted))
	return nil
}

// 初始化日志
func initLogger() {
	logger = logrus.New()

	// 确保 logs 目录存在
	logDir := "logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		// 创建目录（权限 0755：所有者可读写执行，其他用户可读执行）
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "无法创建日志目录: %v\n", err)
			return // 目录创建失败时终止日志初始化
		}
	}

	// 打开或创建日志文件（目录已存在）
	file, err := os.OpenFile(filepath.Join(logDir, "app.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666) // 文件权限 0666：所有用户可读写
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法创建日志文件: %v\n", err)
	} else {
		logger.SetOutput(file)
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
		})
	}

	// 控制台输出（通过 Hook）
	consoleFormatter := &CustomConsoleFormatter{}
	consoleHook := &ConsoleHook{Formatter: consoleFormatter}
	logger.AddHook(consoleHook)
	logger.Info("日志系统初始化完成")
}

// 获取GitHub Meta数据中的web字段IP列表
func getGitHubMetaIPs() ([]string, error) {
	logger.Info("获取GitHub Meta数据...")

	resp, err := http.Get("https://api.github.com/meta")
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回非200状态码: %s", resp.Status)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %v", err)
	}

	webIPs, ok := data["web"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("web字段不存在或类型错误")
	}

	ips := make([]string, 0, len(webIPs))
	for _, ip := range webIPs {
		if ipStr, ok := ip.(string); ok {
			ips = append(ips, ipStr)
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("未找到有效的IP地址")
	}

	logger.Infof("获取到 %d 个CIDR地址", len(ips))
	return ips, nil
}

// 解析CIDR并获取单个IPv4地址
func parseIPv4FromCIDR(cidr string) (string, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("无效的CIDR格式: %s", cidr)
	}

	if ip.To4() == nil {
		return "", fmt.Errorf("非IPv4地址: %s", cidr)
	}

	return ip.String(), nil
}

// 测试TCP端口可用性
func testTCPPort(ip string, port int, timeout time.Duration) bool {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		logger.Debugf("端口 %d 测试失败: %v", port, err)
		return false
	}
	conn.Close()
	logger.Debugf("端口 %d 测试成功", port)
	return true
}

// Ping IPv4地址并返回响应时间
func pingIPv4(ip string, timeout time.Duration) (time.Duration, bool, error) {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return 0, false, fmt.Errorf("ICMP监听失败: %v", err)
	}
	defer conn.Close()

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("HELLO"),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return 0, false, fmt.Errorf("ICMP消息构造失败: %v", err)
	}

	dest := &net.IPAddr{IP: net.ParseIP(ip)}
	if dest.IP == nil {
		return 0, false, fmt.Errorf("无效的IP地址: %s", ip)
	}

	start := time.Now()
	if _, err := conn.WriteTo(msgBytes, dest); err != nil {
		return 0, false, fmt.Errorf("ICMP发送失败: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, false, fmt.Errorf("设置超时失败: %v", err)
	}

	recvBytes := make([]byte, 1500)
	n, _, err := conn.ReadFrom(recvBytes)
	if err != nil {
		return 0, false, fmt.Errorf("ICMP接收失败: %v", err)
	}

	elapsed := time.Since(start)

	recvMsg, err := icmp.ParseMessage(1, recvBytes[:n])
	if err != nil {
		return 0, false, fmt.Errorf("ICMP解析失败: %v", err)
	}

	if recvMsg.Type != ipv4.ICMPTypeEchoReply {
		return 0, false, fmt.Errorf("非Echo回复类型: %v", recvMsg.Type)
	}

	return elapsed, true, nil
}

// 获取终端宽度
func getTerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		return width
	}
	return 80
}

// 主函数
func main() {
	initLogger()
	logger.Info("程序启动")

	if strings.ToLower(os.Getenv("OS")) == "windows_nt" {
		logger.Warnf("%s注意: 在Windows上运行ICMP Ping可能需要管理员权限%s", colorYellow, colorReset)
		logger.Warnf("%s如果Ping测试失败，请尝试以管理员身份运行此程序%s", colorYellow, colorReset)
	}

	cidrs, err := getGitHubMetaIPs()
	if err != nil {
		logger.Errorf("%s错误: %v%s", colorRed, err, colorReset)
		return
	}

	ips := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		ip, err := parseIPv4FromCIDR(cidr)
		if err != nil {
			logger.Debugf("跳过CIDR: %s, 原因: %v", cidr, err)
			continue
		}
		ips = append(ips, ip)
	}

	if len(ips) == 0 {
		logger.Errorf("%s错误: 没有有效的IPv4地址可测试%s", colorRed, colorReset)
		return
	}

	logger.Infof("解析到 %d 个IPv4地址", len(ips))

	timeout := 5 * time.Second
	var wg sync.WaitGroup
	results := make([]IPInfo, len(ips))
	mu := sync.Mutex{}

	logger.Infof("开始测试IP地址，超时时间: %v", timeout)
	logger.Info("只有Ping成功的IP才会进行端口测试")

	for i, ip := range ips {
		wg.Add(1)
		go func(index int, ipAddr string) {
			defer wg.Done()
			info := IPInfo{IP: ipAddr}

			logger.Debugf("开始Ping测试: %s", ipAddr)
			pingTime, pingSuccess, pingErr := pingIPv4(ipAddr, timeout)
			if pingErr != nil {
				logger.Errorf("Ping %s 失败: %v", ipAddr, pingErr)
				info.PingTime = timeout
				info.PingSuccess = false
			} else {
				if pingSuccess {
					logger.Infof("Ping %s 成功: %s", ipAddr, pingTime.String())
					info.PingTime = pingTime
					info.PingSuccess = true

					logger.Debugf("开始端口测试: %s", ipAddr)
					info.Port22Open = testTCPPort(ipAddr, 22, timeout)
					info.Port80Open = testTCPPort(ipAddr, 80, timeout)
					info.Port443Open = testTCPPort(ipAddr, 443, timeout)

					logger.Infof("IP %s 端口状态: (%t), 80(%t), 443(%t)",
						ipAddr, info.Port22Open, info.Port80Open, info.Port443Open)
				} else {
					logger.Debugf("Ping %s 响应", ipAddr)
					info.PingTime = timeout
					info.PingSuccess = false
				}
			}

			mu.Lock()
			results[index] = info
			mu.Unlock()
		}(i, ip)
	}

	wg.Wait()

	successfulResults := make([]IPInfo, 0)
	for _, info := range results {
		if info.PingSuccess {
			successfulResults = append(successfulResults, info)
		}
	}

	if len(successfulResults) == 0 {
		logger.Errorf("%s错误: 没有IP地址Ping成功%s，请检查网络连接或权限。%s", colorRed, colorBold, colorReset)
		return
	}

	logger.Infof("Ping成功的IP数量: %d", len(successfulResults))

	sort.Slice(successfulResults, func(i, j int) bool {
		return successfulResults[i].PingTime < successfulResults[j].PingTime
	})

	logger.Infof("%s测试结果汇总:%s", colorBold+colorMagenta, colorReset)
	terminalWidth := getTerminalWidth()
	separator := strings.Repeat("=", terminalWidth)
	logger.Infof("%s%s%s", colorMagenta, separator, colorReset)
	logger.Infof("%s%-20s %-12s %-8s %-8s %-8s%s",
		colorBold+colorGreen,
		"IP地址", "Ping时间", "端口22", "端口80", "端口443",
		colorReset)
	logger.Infof("%s%s%s", colorMagenta, separator, colorReset)

	for _, info := range successfulResults {
		port22Color := colorRed
		if info.Port22Open {
			port22Color = colorGreen
		}

		port80Color := colorRed
		if info.Port80Open {
			port80Color = colorGreen
		}

		port443Color := colorRed
		if info.Port443Open {
			port443Color = colorGreen
		}

		pingColor := colorGreen
		if info.PingTime > 100*time.Millisecond {
			pingColor = colorYellow
		}
		if info.PingTime > 300*time.Millisecond {
			pingColor = colorRed
		}

		logger.Infof("%-20s %s%-12s%s %s%-8t%s %s%-8t%s %s%-8t%s",
			info.IP,
			pingColor, info.PingTime.String(), colorReset,
			port22Color, info.Port22Open, colorReset,
			port80Color, info.Port80Open, colorReset,
			port443Color, info.Port443Open, colorReset)
	}

	logger.Infof("%s%s%s", colorMagenta, separator, colorReset)

	open22 := 0
	open80 := 0
	open443 := 0
	for _, info := range successfulResults {
		if info.Port22Open {
			open22++
		}
		if info.Port80Open {
			open80++
		}
		if info.Port443Open {
			open443++
		}
	}

	logger.Infof("%s统计: 总共Ping通 %d 个IP%s", colorCyan, len(successfulResults), colorReset)
	logger.Infof("%s端口22开放: %d, 端口80开放: %d, 端口443开放: %d%s",
		colorCyan, open22, open80, open443, colorReset)

	if len(successfulResults) > 0 {
		fastest := successfulResults[0]
		logger.Infof("%s最快IP: %s (Ping: %s)%s",
			colorBold+colorGreen, fastest.IP, fastest.PingTime.String(), colorReset)
	}

	logger.Info("程序执行完成")
}
