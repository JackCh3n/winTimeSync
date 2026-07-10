//go:build linux

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

// clockRealtime 对应 Linux 的 CLOCK_REALTIME。
const clockRealtime = 0

// setSystemTime 通过 clock_settime(CLOCK_REALTIME) 设置 Linux 系统时钟为给定 UTC 时间。需要 root 权限。
func setSystemTime(t time.Time) error {
	ts := syscall.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
	_, _, errno := syscall.Syscall(syscall.SYS_CLOCK_SETTIME, uintptr(clockRealtime), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		return fmt.Errorf("设置系统时间失败(clock_settime): %v（需要 root 权限）", errno)
	}
	return nil
}
