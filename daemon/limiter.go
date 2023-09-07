package daemon

import (
	"math"
	"sync"
	"time"
)

// 并发安全
type Limiter struct {
	mu           sync.Mutex
	count, limit int

	interval time.Duration  // 限制restart时间间隔
	last     time.Time      // 上次restart时间
	location *time.Location // 时区

}

func NewLimiter() *Limiter {
	location, _ := time.LoadLocation("Asia/Shanghai")
	return &Limiter{
		limit:    5,
		interval: time.Second,
		location: location,
	}
}

func (l *Limiter) Inc() bool {
	return l.add(1)
}

func (l *Limiter) add(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	// 限制重启时间间隔, 每次重启时间间隔增加2的count次方
	if !l.last.Before(l.next()) {
		return false
	}
	if l.count+n > l.limit || l.count+n < 0 {
		return false
	}
	l.count += n
	l.last = time.Now().In(l.location)
	return true
}

func (l *Limiter) Dec() bool {
	return l.add(-1)
}

func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count = 0
	l.last = time.Time{}
}

// 计算cmd下次可以启动的时间
func (l *Limiter) next() time.Time {
	if l.last.IsZero() {
		return time.Now().In(l.location)
	}
	return l.last.Add(time.Duration(math.Pow(2, float64(l.count))) * l.interval)
}
