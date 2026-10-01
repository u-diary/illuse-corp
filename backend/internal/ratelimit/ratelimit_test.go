package ratelimit

import (
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	l := New(2, time.Minute)
	l.now = func() time.Time { return now }

	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("上限までは許可される")
	}
	if l.Allow("a") {
		t.Fatal("上限を超えたら拒否される")
	}
	if !l.Allow("b") {
		t.Fatal("別のキーは独立して数える")
	}

	now = now.Add(time.Minute)
	if !l.Allow("a") {
		t.Fatal("ウィンドウが過ぎたら再び許可される")
	}
}

func TestSweep(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	l := New(1, time.Minute)
	l.now = func() time.Time { return now }

	l.Allow("a")
	l.Allow("b")
	now = now.Add(2 * time.Minute)
	l.Allow("c")
	if _, ok := l.windows["a"]; ok {
		t.Error("期限切れのキーは掃除される")
	}
	if len(l.windows) != 1 {
		t.Errorf("windows = %d, want 1", len(l.windows))
	}
}
