//go:build !windows && !linux && !darwin

package main

import (
	"errors"
	"time"
)

// 其它平台（如 freebsd 等）暂未实现设置系统时间。Windows/Linux/macOS 已有各自的实现。
func setSystemTime(t time.Time) error {
	return errors.New("设置系统时间仅支持 Windows / Linux / macOS 平台")
}
