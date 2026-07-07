# winTimeSync

轻量级 Windows 时间同步小工具（Go 编写，**零外部依赖**）。

兼容 **NTP 协议** 与 **内网 HTTP 时间源** 两种方式，支持按秒级间隔定时同步系统时钟，并可注册为系统开机启动。

## 功能特性

- **NTP 客户端**：依据 RFC5905 用 4 个时间戳（t0/t1/t2/t3）计算时间偏移与网络延时，毫秒级校准。
- **内网 HTTP 时间源**：请求内网地址，按 `RTT/2` 估算偏移并校准系统时间。
- **内置 HTTP 时间服务器**（`server` 模式）：把本机变成内网时间源，对外提供 `/time` 接口（可选后台用 NTP 自校准）。
- **定时同步**：`-interval` 指定秒数，循环执行。
- **只读检查**：`-check` 只打印偏差，不修改系统时间。
- **开机启动**：`install` 注册系统级计划任务（SYSTEM 账户，无需登录用户即运行）。

## 编译

需要 Go 1.22+。在项目目录执行：

```bash
go build -o winTimeSync.exe .
```

## 使用

```bash
winTimeSync run                      持续运行，按 -interval 周期同步（默认 3600 秒）
winTimeSync once                     立即同步一次后退出
winTimeSync server                   启动 HTTP 时间服务器，对内网提供时间源
winTimeSync install                  注册为系统开机启动（计划任务，需管理员）
winTimeSync uninstall                移除开机启动
winTimeSync status                   查看是否已注册开机启动
winTimeSync version                  查看版本
```

### 通用参数

| 参数 | 说明 | 默认 |
|------|------|------|
| `-source` | 时间源：`ntp` \| `http` | `ntp` |
| `-ntp-server` | NTP 服务器地址（source=ntp 时生效） | `pool.ntp.org:123` |
| `-http-url` | HTTP 时间服务器地址（source=http 时生效） | `http://127.0.0.1:8080/time` |
| `-interval` | 同步间隔（秒），run 模式生效 | `3600` |
| `-timeout` | 单次请求超时（秒） | `5` |
| `-check` | 仅检查时间偏差，不修改系统时间 | `false` |
| `-quiet` | 安静模式，仅输出错误 | `false` |

server 模式参数：`-server-addr`（监听地址，默认 `:8080`）、`-server-ntp`（后台用 NTP 校准本机时钟，默认 `true`）。

### 示例

```bash
# NTP，每 10 分钟同步一次
winTimeSync.exe run -source ntp -interval 600

# 内网 HTTP 时间源，每 60 秒同步
winTimeSync.exe run -source http -http-url http://127.0.0.1:8080/time -interval 60

# 只检查偏差不改系统时间
winTimeSync.exe once -source ntp -check

# 把本机作为内网时间源
winTimeSync.exe server -server-addr :8080

# 注册 / 查看 / 移除开机启动（需管理员）
winTimeSync.exe install
winTimeSync.exe status
winTimeSync.exe uninstall
```

## HTTP 时间接口约定

`source=http` 时，工具向 `-http-url` 发起 GET 请求，支持以下响应格式：

- JSON：`{"time":"2026-07-06T17:40:59.123Z","unix":1783331459,"unixMs":1783331459123}`
- 纯 RFC3339 字符串：`2026-07-06T17:40:59.123Z`
- 纯 Unix 时间戳（秒或毫秒）

内置 `server` 模式返回上述 JSON（路径 `/time`），可直接作为内网时间源使用。

## 修改运行参数

`run` 模式的参数在**启动时**读取：

- **手动运行**：直接 `Ctrl+C` 停止，再用新参数启动即可。
- **已注册开机启动（`install`）**：计划任务命令在注册时即固定，需重新 `install`（带 `/f` 覆盖同名任务）或先 `uninstall` 再 `install`，才能永久变更 source / 地址 / 间隔。

## 注意事项

- 修改系统时间与注册开机启动都需 **以管理员身份运行**。
- 设置系统时间使用 UTC，工具内部已自动转换时区，无需手动处理。
- 仅支持 Windows（设置系统时间依赖 `kernel32.SetSystemTime`）；非 Windows 平台编译时该能力以错误桩占位。
