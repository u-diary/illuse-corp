package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFormatNumber(t *testing.T) {
	for n, want := range map[int64]string{1: "000001", 42: "000042", 999999: "999999", 1000000: "1000000"} {
		if got := FormatNumber(n); got != want {
			t.Errorf("FormatNumber(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestIssueIsSequentialWithoutGaps(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	ok := func(int64) error { return nil }

	if n, err := s.Issue(ctx, "営業部", at, ok); err != nil || n != 1 {
		t.Fatalf("1 件目: n=%d err=%v", n, err)
	}

	// 生成に失敗したら記録されず、番号も消費されない。
	boom := errors.New("boom")
	var attempted int64
	_, err := s.Issue(ctx, "技術開発部", at, func(n int64) error { attempted = n; return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if attempted != 2 {
		t.Fatalf("失敗時に試した番号 = %d, want 2", attempted)
	}

	if n, err := s.Issue(ctx, "業務統括部", at, ok); err != nil || n != 2 {
		t.Fatalf("失敗の次: n=%d err=%v, want 2（欠番なし）", n, err)
	}

	records, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{
		{1, "営業部", "2026-10-01T12:00:00+09:00"},
		{2, "業務統括部", "2026-10-01T12:00:00+09:00"},
	}
	if len(records) != len(want) {
		t.Fatalf("records = %+v, want %+v", records, want)
	}
	for i := range want {
		if records[i] != want[i] {
			t.Errorf("records[%d] = %+v, want %+v", i, records[i], want[i])
		}
	}
}

func TestIssueConcurrentHasNoDuplicates(t *testing.T) {
	s := openTemp(t)
	const n = 30
	var wg sync.WaitGroup
	numbers := make(chan int64, n)
	for range n {
		wg.Go(func() {
			got, err := s.Issue(context.Background(), "営業部", time.Now(), func(int64) error { return nil })
			if err != nil {
				t.Error(err)
				return
			}
			numbers <- got
		})
	}
	wg.Wait()
	close(numbers)

	seen := map[int64]bool{}
	for got := range numbers {
		if seen[got] {
			t.Errorf("社員番号 %d が重複", got)
		}
		seen[got] = true
	}
	for i := int64(1); i <= n; i++ {
		if !seen[i] {
			t.Errorf("社員番号 %d が欠番", i)
		}
	}
}

func TestReopenContinuesNumbering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	ok := func(int64) error { return nil }
	for want := int64(1); want <= 2; want++ {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		n, err := s.Issue(context.Background(), "営業部", time.Now(), ok)
		s.Close()
		if err != nil || n != want {
			t.Fatalf("再オープン後: n=%d err=%v, want %d", n, err, want)
		}
	}
}
