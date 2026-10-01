// Package ratelimit はキー（クライアント IP）ごとの固定ウィンドウ方式のレート制限を提供する。
package ratelimit

import (
	"sync"
	"time"
)

// Limiter は window あたり limit 回までリクエストを許可する。
type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	windows   map[string]*window
	lastSweep time.Time
}

type window struct {
	start time.Time
	count int
}

// New は Limiter を作成する。
func New(limit int, per time.Duration) *Limiter {
	return &Limiter{limit: limit, window: per, now: time.Now, windows: map[string]*window{}}
}

// Allow は key のリクエストを 1 回分数え、上限内なら true を返す。
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) >= l.window {
		w = &window{start: now}
		l.windows[key] = w
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// sweep は期限切れのウィンドウを定期的に捨て、メモリが増え続けないようにする。
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k, w := range l.windows {
		if now.Sub(w.start) >= l.window {
			delete(l.windows, k)
		}
	}
}
