package coupon

import (
	"errors"
	"testing"
	"time"

	"e-commerce/pkg/errno"
)

func TestParseCouponTime(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载 Asia/Shanghai 失败: %v", err)
	}
	utc := time.UTC

	cases := []struct {
		name string
		raw  string
		loc  *time.Location
		want time.Time
		bad  bool
	}{
		{"RFC3339 带东八区偏移", "2026-06-01T00:00:00+08:00", utc, time.Date(2026, 6, 1, 0, 0, 0, 0, shanghai), false},
		{"RFC3339 UTC", "2026-06-01T00:00:00Z", shanghai, time.Date(2026, 6, 1, 0, 0, 0, 0, utc), false},
		{"历史格式按业务时区解释", "2026-06-01 00:00:00", shanghai, time.Date(2026, 6, 1, 0, 0, 0, 0, shanghai), false},
		{"历史格式前后空格", " 2026-06-01 00:00:00 ", shanghai, time.Date(2026, 6, 1, 0, 0, 0, 0, shanghai), false},
		{"非法日期", "2026-13-01 00:00:00", shanghai, time.Time{}, true},
		{"非法时区偏移", "2026-06-01T00:00:00+25:00", shanghai, time.Time{}, true},
		{"空字符串", "", shanghai, time.Time{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseCouponTime(c.raw, c.loc)
			if c.bad {
				if err == nil {
					t.Fatalf("期望解析失败，实际得到 %v", got)
				}
				var target *errno.Errno
				if !errors.As(err, &target) || target.FullCode() != errno.ErrInvalidParam.FullCode() {
					t.Fatalf("期望 A00001，实际=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if !got.Equal(c.want) {
				t.Fatalf("解析结果=%v（%d），期望=%v（%d）", got, got.Unix(), c.want, c.want.Unix())
			}
		})
	}
}

// 同一个历史格式字符串在不同业务时区下必须解析成不同时刻，证明没有依赖 time.Local。
func TestParseCouponTimeUsesGivenLocation(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("加载 Asia/Shanghai 失败: %v", err)
	}
	inShanghai, err := parseCouponTime("2026-06-01 12:00:00", shanghai)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	inUTC, err := parseCouponTime("2026-06-01 12:00:00", time.UTC)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if inShanghai.Unix() == inUTC.Unix() {
		t.Fatalf("不同业务时区解析出了同一时刻：%d", inShanghai.Unix())
	}
	if diff := inUTC.Unix() - inShanghai.Unix(); diff != 8*60*60 {
		t.Fatalf("时区差=%d 秒，期望 8 小时", diff)
	}
}
