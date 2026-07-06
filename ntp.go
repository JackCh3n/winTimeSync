package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// ntpEpochOffset 是 1900-01-01 (NTP 纪元) 与 1970-01-01 (Unix 纪元) 之间的秒数差。
const ntpEpochOffset = 2208988800

// queryNTP 向 NTP 服务器发送请求，依据 RFC5905 的 4 个时间戳计算时间偏移与网络延时，
// 返回校准后的当前时间、offset（服务端 - 客户端）以及往返延时。
func queryNTP(server string, timeout time.Duration) (corrected time.Time, offset, delay time.Duration, err error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	raddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("解析地址失败: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	// 构造 48 字节请求包：LI=0, VN=3, Mode=3(client) => 0x1B
	req := make([]byte, 48)
	req[0] = 0x1B

	t0 := time.Now()
	secs, frac := timeToNTP(t0)
	binary.BigEndian.PutUint32(req[40:], secs)
	binary.BigEndian.PutUint32(req[44:], frac)

	if _, err = conn.Write(req); err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("发送请求失败: %w", err)
	}

	resp := make([]byte, 48)
	if _, err = conn.Read(resp); err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("读取响应失败: %w", err)
	}
	t3 := time.Now()

	// 响应包中：接收时间戳 t1 位于 [32:40]，发送时间戳 t2 位于 [40:48]
	t1 := ntpBytesToTime(resp[32:40])
	t2 := ntpBytesToTime(resp[40:48])

	t0n := t0.UnixNano()
	t1n := t1.UnixNano()
	t2n := t2.UnixNano()
	t3n := t3.UnixNano()

	// offset = ((t1 - t0) + (t2 - t3)) / 2  （服务端时间 - 客户端时间）
	offset = time.Duration(((t1n-t0n)+(t2n-t3n))/2) * time.Nanosecond
	// delay  = (t3 - t0) - (t2 - t1)
	delay = time.Duration((t3n-t0n)-(t2n-t1n)) * time.Nanosecond
	corrected = t3.Add(offset)

	return corrected, offset, delay, nil
}

func timeToNTP(t time.Time) (uint32, uint32) {
	t = t.UTC()
	secs := uint32(t.Unix() + ntpEpochOffset)
	frac := uint32(float64(t.Nanosecond()) * (1 << 32) / 1e9)
	return secs, frac
}

func ntpBytesToTime(b []byte) time.Time {
	sec := binary.BigEndian.Uint32(b[0:4])
	frac := binary.BigEndian.Uint32(b[4:8])
	return ntpToTime(sec, frac)
}

func ntpToTime(sec, frac uint32) time.Time {
	unix := int64(sec) - ntpEpochOffset
	nsec := int64(float64(frac) * 1e9 / (1 << 32))
	return time.Unix(unix, nsec).UTC()
}
