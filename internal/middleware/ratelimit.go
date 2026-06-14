package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen int64 // unix nano
}

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
}

func newRateLimiter(r rate.Limit, burst int) *rateLimiter {
	return &rateLimiter{
		visitors: make(map[string]*visitor),
		rate:     r,
		burst:    burst,
	}
}

// getLimiter 获取或创建 IP 对应的令牌桶
func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitor{limiter: limiter}
		return limiter
	}
	return v.limiter
}

// RateLimitMiddleware 返回 IP 级令牌桶限流中间件
// reqPerSec: 每秒允许的请求数，burst: 突发容量
func RateLimitMiddleware(reqPerSec int, burst int) gin.HandlerFunc {
	rl := newRateLimiter(rate.Limit(reqPerSec), burst)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "请求过于频繁，请稍后重试",
			})
			return
		}

		c.Next()
	}
}
