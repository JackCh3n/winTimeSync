//go:build !windows

package main

import (
	"errors"
	"time"
)

// 非 Windows 平台暂不支持设置系统时间（本工具面向 Windows 开机启动场景）。
func setSystemTime(t time.Time) error {
	return errors.New("设置系统时间仅支持 Windows 平台")
}
