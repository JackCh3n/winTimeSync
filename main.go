package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

var (
	source     = flag.String("source", "ntp", "时间源: ntp | http")
	ntpServer  = flag.String("ntp-server", "pool.ntp.org:123", "NTP 服务器地址 (source=ntp 时生效)")
	httpURL    = flag.String("http-url", "http://127.0.0.1:8080/time", "HTTP 时间服务器地址 (source=http 时生效)")
	interval   = flag.Int("interval", 3600, "同步间隔（秒），run 模式生效")
	check      = flag.Bool("check", false, "仅检查时间偏差，不修改系统时间")
	timeoutSec = flag.Int("timeout", 5, "单次请求超时（秒）")
	serverAddr = flag.String("server-addr", ":8080", "HTTP 时间服务器监听地址 (server 模式)")
	serverNTP  = flag.Bool("server-ntp", true, "server 模式下是否后台用 NTP 校准本机时钟")
	quiet      = flag.Bool("quiet", false, "安静模式，仅输出错误")
)

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
	logf("winTimeSync 启动 | 时间源=%s | 间隔=%d秒 | 检查模式=%v", *source, *interval, *check)
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

// doSync 执行一次同步（NTP 或 HTTP），并按需设置系统时间。
func doSync() error {
	timeout := time.Duration(*timeoutSec) * time.Second

	var (
		corrected time.Time
		offset    time.Duration
		delay     time.Duration
		err       error
	)

	switch *source {
	case "ntp":
		corrected, offset, delay, err = queryNTP(*ntpServer, timeout)
		if err != nil {
			return fmt.Errorf("NTP 查询 %s 失败: %w", *ntpServer, err)
		}
	case "http":
		corrected, offset, delay, err = queryHTTPTime(*httpURL, timeout, nil)
		if err != nil {
			return fmt.Errorf("HTTP 时间查询 %s 失败: %w", *httpURL, err)
		}
	default:
		return fmt.Errorf("未知时间源: %s（支持 ntp | http）", *source)
	}

	now := time.Now()
	logf("[%s] 源=%s 偏移=%s 延时=%s 校准后=%s",
		now.Format("2006-01-02 15:04:05"),
		*source,
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

func printUsage() {
	fmt.Print(`winTimeSync - 轻量级时间同步工具（NTP / 内网 HTTP 双协议，支持开机启动）

用法:
  winTimeSync run                      持续运行，按 -interval 周期同步（默认 3600 秒）
  winTimeSync once                     立即同步一次后退出
  winTimeSync server                   启动 HTTP 时间服务器，对内网提供时间源
  winTimeSync install                  注册为系统开机启动（计划任务，需管理员）
  winTimeSync uninstall                移除开机启动
  winTimeSync status                   查看是否已注册开机启动
  winTimeSync version                  查看版本

通用参数:
  -source string       时间源: ntp | http（默认 ntp）
  -ntp-server string   NTP 服务器（默认 pool.ntp.org:123）
  -http-url string     HTTP 时间地址（默认 http://127.0.0.1:8080/time）
  -interval int        同步间隔秒数（默认 3600）
  -timeout int         单次请求超时秒数（默认 5）
  -check               仅检查偏差，不修改系统时间
  -quiet               安静模式，仅输出错误

server 模式参数:
  -server-addr string  监听地址（默认 :8080）
  -server-ntp bool     后台用 NTP 校准本机时钟（默认 true）

示例:
  winTimeSync run -source ntp -interval 600
  winTimeSync run -source http -http-url http://192.168.1.10:8080/time -interval 60
  winTimeSync once -source ntp -check
  winTimeSync server -server-addr :8080
  winTimeSync install   （请以管理员身份运行）
`)
}
