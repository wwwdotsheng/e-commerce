package coupon

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"e-commerce/pkg/errno"
)

const legacyTimeLayout = "2006-01-02 15:04:05"

// parseCouponTime 解析优惠券请求里的时间：
//   - 优先接受带时区的 RFC3339（推荐，例如 2026-06-01T00:00:00+08:00）；
//   - 兼容历史格式 "2006-01-02 15:04:05"，按业务时区解释。
//
// 解析失败统一返回参数错误 A00001，而不是退化成 B01001。
func parseCouponTime(raw string, location *time.Location) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, errno.ErrInvalidParam.WithRaw(errors.New("empty coupon time"))
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if location == nil {
		location = time.Local
	}
	if parsed, err := time.ParseInLocation(legacyTimeLayout, value, location); err == nil {
		return parsed, nil
	}
	return time.Time{}, errno.ErrInvalidParam.WithRaw(fmt.Errorf("invalid coupon time %q", raw))
}
