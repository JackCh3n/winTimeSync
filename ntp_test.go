package main

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestTimeNTPRoundTrip(t *testing.T) {
	cases := []time.Time{
		time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2035, 1, 1, 0, 0, 0, 123456789, time.UTC),
		time.Date(1972, 2, 29, 23, 59, 59, 500, time.UTC),
	}
	for _, in := range cases {
		secs, frac := timeToNTP(in)
		got := ntpToTime(secs, frac)
		// NTP 时间戳分数仅 32 位，分辨率约 0.23ns，往返存在亚纳秒级截断误差，属正常。
		if diff := got.Sub(in); diff < -time.Microsecond || diff > time.Microsecond {
			t.Errorf("NTP 时间戳往返偏差过大: in=%v out=%v diff=%v", in, got, diff)
		}
	}
}

func TestNtpBytesToTime(t *testing.T) {
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secs, frac := timeToNTP(want)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[0:4], secs)
	binary.BigEndian.PutUint32(buf[4:8], frac)
	if got := ntpBytesToTime(buf); !got.Equal(want) {
		t.Errorf("ntpBytesToTime = %v, want %v", got, want)
	}
}
