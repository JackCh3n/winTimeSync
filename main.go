package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	source     = flag.String("source", "ntp", "时间源: ntp | http（未指定 -chain 时生效）")
	chain      = flag.String("chain", "", "主备链：按顺序尝试，用逗号分隔。每项格式 ntp:地址 或 http:地址。例: ntp:pool.ntp.org:123,http:http://127.0.0.1:8080/time")
	ntpServer  = flag.String("ntp-server", "pool.ntp.org:123", "NTP 服务器地址 (source=ntp 时生效)")
	httpURL    = flag.String("http-url", "http://127.0.0.1:8080/time", "HTTP 时间服务器地址 (source=http 时生效)")
	interval   = flag.Int("interval", 3600, "同步间隔（秒），run 模式生效")
	check      = flag.Bool("check", false, "仅检查时间偏差，不修改系统时间")
	timeoutSec = flag.Int("timeout", 5, "单次请求超时（秒）")
	serverAddr     = flag.String("server-addr", ":8080", "HTTP 时间服务器监听地址 (server 模式)")
	serverNTP      = flag.Bool("server-ntp", true, "server 模式下是否后台用 NTP 校准本机时钟")
	serverNTPServe = flag.Bool("server-ntp-serve", true, "server 模式下是否同时启动 NTP 服务器(UDP)，使本机兼作 NTP 时间源")
	serverNTPPort  = flag.String("server-ntp-port", "123", "NTP 服务器监听端口 (server 模式, 默认 123，需管理员)")
	quiet          = flag.Bool("quiet", false, "安静模式，仅输出错误")
)

// timeSource 表示主备链中的一个时间源。
type timeSource struct {
	kind   string // "ntp" 或 "http"
	target string // NTP 地址 或 HTTP URL
	label  string // 日志用标识
}

// buildChain 解析出本次同步要尝试的时间源顺序。
// 若指定了 -chain 则按其顺序；否则回退到旧的 -source 单源模式（保持向后兼容）。
func buildChain() ([]timeSource, error) {
	if *chain != "" {
		var out []timeSource
		for _, part := range strings.Split(*chain, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			i := strings.Index(part, ":")
			if i <= 0 {
				return nil, fmt.Errorf("无法解析源: %q（格式应为 ntp:地址 或 http:地址）", part)
			}
			kind, target := part[:i], part[i+1:]
			switch kind {
			case "ntp":
				out = append(out, timeSource{kind: "ntp", target: target, label: "ntp:" + target})
			case "http":
				out = append(out, timeSource{kind: "http", target: target, label: "http:" + target})
			default:
				return nil, fmt.Errorf("未知源类型: %q（支持 ntp: / http:）", kind)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("-chain 为空或无法解析")
		}
		return out, nil
	}
	// 旧模式：单一 -source
	switch *source {
	case "ntp":
		return []timeSource{{kind: "ntp", target: *ntpServer, label: "ntp:" + *ntpServer}}, nil
	case "http":
		return []timeSource{{kind: "http", target: *httpURL, label: "http:" + *httpURL}}, nil
	default:
		return nil, fmt.Errorf("未知时间源: %s（支持 ntp | http）", *source)
	}
}

func logf(format string, args ...interface{}) {
	if !*quiet {
		fmt.Printf(format+"\n", args...)
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	// 子命令之后的参数再交给 flag 解析
	if err := flag.CommandLine.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}

	switch cmd {
	case "run":
		runLoop()
	case "once":
		if err := doSync(); err != nil {
			fmt.Fprintf(os.Stderr, "同步失败: %v\n", err)
			os.Exit(1)
		}
	case "server":
		if *serverNTPServe {
			if err := startNTPServer(":" + *serverNTPPort); err != nil {
				fmt.Fprintf(os.Stderr, "警告: NTP 服务器启动失败，仅启用 HTTP 时间服务器: %v\n", err)
			}
		}
		if err := startTimeServer(*serverAddr, *serverNTP, *ntpServer, time.Duration(*interval)*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "HTTP 时间服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := installStartup(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		if err := uninstallStartup(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	case "status":
		if isInstalled() {
			fmt.Println("已注册开机启动（计划任务 " + taskName + "）")
		} else {
			fmt.Println("未注册开机启动")
		}
	case "version", "-v", "--version":
		fmt.Println("winTimeSync v1.0.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

func runLoop() {
	chain, err := buildChain()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	labels := make([]string, len(chain))
	for i, s := range chain {
		labels[i] = s.label
	}
	logf("winTimeSync 启动 | 主备链=[%s] | 间隔=%d秒 | 检查模式=%v",
		strings.Join(labels, " > "), *interval, *check)
	if *interval < 1 {
		*interval = 1
	}
	// 启动后立即同步一次
	if err := doSync(); err != nil {
		fmt.Fprintf(os.Stderr, "初始同步失败: %v\n", err)
	}
	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := doSync(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] 同步失败: %v\n",
				time.Now().Format("2006-01-02 15:04:05"), err)
		}
	}
}

// doSync 按主备链顺序尝试各时间源，首个成功者用于校准系统时间。
func doSync() error {
	timeout := time.Duration(*timeoutSec) * time.Second

	chain, err := buildChain()
	if err != nil {
		return err
	}

	var lastErr error
	for _, s := range chain {
		var (
			corrected time.Time
			offset    time.Duration
			delay     time.Duration
		)
		switch s.kind {
		case "ntp":
			corrected, offset, delay, err = queryNTP(s.target, timeout)
		case "http":
			corrected, offset, delay, err = queryHTTPTime(s.target, timeout, nil)
		default:
			err = fmt.Errorf("未知源类型: %s", s.kind)
		}
		if err != nil {
			lastErr = err
			logf("源 %s 失败: %v，尝试下一个", s.label, err)
			continue
		}
		// 成功
		now := time.Now()
		logf("[%s] 命中源=%s 偏移=%s 延时=%s 校准后=%s",
			now.Format("2006-01-02 15:04:05"),
			s.label,
			offset.Round(time.Millisecond),
			delay.Round(time.Millisecond),
			corrected.Format("2006-01-02 15:04:05.000 MST"),
		)
		if *check {
			logf("仅检查模式，未修改系统时间")
			return nil
		}
		if err := setSystemTime(corrected); err != nil {
			return fmt.Errorf("设置系统时间失败: %w", err)
		}
		logf("已同步系统时间 -> %s", corrected.Format("2006-01-02 15:04:05 MST"))
		return nil
	}
	return fmt.Errorf("所有时间源均失败，最后错误: %w", lastErr)
}

func printUsage() {
	fmt.Print(`winTimeSync - 轻量级时间同步工具（NTP / 内网 HTTP 双协议；server 模式可同时充当 NTP+HTTP 时间源，支持开机启动）

用法:
  winTimeSync run                      持续运行，按 -interval 周期同步（默认 3600 秒）
  winTimeSync once                     立即同步一次后退出
  winTimeSync server                   启动 HTTP 时间服务器，对内网提供时间源
  winTimeSync install                  注册为系统开机启动（计划任务，需管理员）
  winTimeSync uninstall                移除开机启动
  winTimeSync status                   查看是否已注册开机启动
  winTimeSync version                  查看版本

通用参数:
  -source string       单源模式时间源: ntp | http（未指定 -chain 时生效，默认 ntp）
  -chain string        主备链：按顺序尝试，逗号分隔。每项 ntp:地址 或 http:地址
  -ntp-server string   NTP 服务器（默认 pool.ntp.org:123）
  -http-url string     HTTP 时间地址（默认 http://127.0.0.1:8080/time）
  -interval int        同步间隔秒数（默认 3600）
  -timeout int         单次请求超时秒数（默认 5）
  -check               仅检查偏差，不修改系统时间
  -quiet               安静模式，仅输出错误

server 模式参数:
  -server-addr string       HTTP 时间服务器监听地址（默认 :8080）
  -server-ntp bool          后台用 NTP 校准本机时钟（默认 true）
  -server-ntp-serve bool    同时启动 NTP 服务器(UDP)，使本机兼作 NTP 时间源（默认 true）
  -server-ntp-port string   NTP 服务器端口（默认 123，需管理员；若被占用可改用其它端口）

示例:
  # 单源
  winTimeSync run -source ntp -interval 600
  winTimeSync run -source http -http-url http://192.168.1.10:8080/time -interval 60

  # 主备：主用 NTP，备用 HTTP
  winTimeSync run -chain "ntp:pool.ntp.org:123,http:http://127.0.0.1:8080/time" -interval 60

  # 主备：NTP A 主，NTP B/C 备
  winTimeSync run -chain "ntp:time1.aliyun.com:123,ntp:time2.aliyun.com:123,ntp:time.windows.com:123" -interval 300

  # A 机：同时作为 NTP(123) + HTTP 时间源，并后台用 NTP 自校准（需管理员，且 123 端口未被占用）
  winTimeSync server -server-addr :8080 -server-ntp-port 123

  # B 机：用 NTP 同步 A（假设 A 的 IP 为 192.168.1.10）
  winTimeSync run -source ntp -ntp-server 192.168.1.10:123 -interval 60

  # B 机主备：主用 A 的 NTP，备用 A 的 HTTP
  winTimeSync run -chain "ntp:192.168.1.10:123,http:http://192.168.1.10:8080/time" -interval 60

  winTimeSync once -chain "ntp:pool.ntp.org:123,http:http://127.0.0.1:8080/time" -check
  winTimeSync install -chain "ntp:pool.ntp.org:123,http:http://127.0.0.1:8080/time" -interval 60   （请以管理员身份运行）
`)
}
