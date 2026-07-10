package main

import (
	"testing"
	"time"
)

func TestBuildChain(t *testing.T) {
	cases := []struct {
		name    string
		ec      effectiveConfig
		want    int
		wantErr bool
	}{
		{"单 NTP 源", effectiveConfig{Source: "ntp", NTPServer: "a:123"}, 1, false},
		{"单 HTTP 源", effectiveConfig{Source: "http", HTTPURL: "http://x/time"}, 1, false},
		{"主备链", effectiveConfig{Chain: "ntp:a:123,http://b/time"}, 2, false},
		{"链格式错误", effectiveConfig{Chain: "foo:bar"}, 0, true},
		{"未知源类型", effectiveConfig{Source: "x"}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ch, err := buildChain(c.ec)
			if c.wantErr {
				if err == nil {
					t.Fatal("期望返回错误，但成功了")
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			if len(ch) != c.want {
				t.Errorf("源数量=%d，期望 %d", len(ch), c.want)
			}
		})
	}
}

func TestOffsetDecision(t *testing.T) {
	cases := []struct {
		abs   time.Duration
		minMs int64
		maxMs int64
		want  string
	}{
		{time.Second, 0, 0, "apply"},
		{500 * time.Millisecond, 1000, 0, "skip"},
		{2 * time.Hour, 0, 3600000, "reject"},
		{time.Hour, 0, 3600000, "apply"},
		{2 * time.Hour, 0, 0, "apply"},
	}
	for _, c := range cases {
		if got := offsetDecision(c.abs, c.minMs, c.maxMs); got != c.want {
			t.Errorf("offsetDecision(%v,%d,%d)=%s，期望 %s", c.abs, c.minMs, c.maxMs, got, c.want)
		}
	}
}
