package store

import (
	"context"
	"database/sql"
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
		{Number: 1, Department: "営業部", GeneratedAt: "2026-10-01T12:00:00+09:00"},
		{Number: 2, Department: "業務統括部", GeneratedAt: "2026-10-01T12:00:00+09:00"},
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

func TestRecordAction(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	jst := time.FixedZone("JST", 9*60*60)
	ok := func(int64) error { return nil }
	for range 2 {
		if _, err := s.Issue(ctx, "営業部", time.Date(2026, 10, 1, 12, 0, 0, 0, jst), ok); err != nil {
			t.Fatal(err)
		}
	}

	at := time.Date(2026, 10, 2, 9, 30, 0, 0, jst)
	if err := s.RecordAction(ctx, 1, "退職届の受理", at); err != nil {
		t.Fatalf("1 回目: %v", err)
	}
	if err := s.RecordAction(ctx, 1, "懲戒解雇(事由:犯罪行為の発覚)", at.Add(time.Hour)); err != ErrAlreadyProcessed {
		t.Errorf("処理済みの社員: err = %v, want ErrAlreadyProcessed", err)
	}
	if err := s.RecordAction(ctx, 99, "退職届の受理", at); err != ErrNotFound {
		t.Errorf("いない社員: err = %v, want ErrNotFound", err)
	}

	r, err := s.Get(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Action != "退職届の受理" || r.ActionAt != "2026-10-02T09:30:00+09:00" {
		t.Errorf("1 回目の記録が上書きされている: %+v", r)
	}
	if r, err := s.Get(ctx, 2); err != nil || r.Action != "" || r.ActionAt != "" {
		t.Errorf("未処理の社員: %+v, %v", r, err)
	}
	if _, err := s.Get(ctx, 99); err != ErrNotFound {
		t.Errorf("Get(いない社員): err = %v", err)
	}
}

func TestRecordActionConcurrentRecordsOnce(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if _, err := s.Issue(ctx, "営業部", time.Now(), func(int64) error { return nil }); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	results := make(chan error, n)
	for range n {
		wg.Go(func() { results <- s.RecordAction(ctx, 1, "退職届の受理", time.Now()) })
	}
	wg.Wait()
	close(results)

	succeeded := 0
	for err := range results {
		switch err {
		case nil:
			succeeded++
		case ErrAlreadyProcessed:
		default:
			t.Errorf("想定外のエラー: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("記録できた回数 = %d, want 1", succeeded)
	}
}

// TestMigrateExistingDatabase は、列を追加する前の（すでに本番で使っている）DB を開いても
// 既存の記録が残り、新しい列が使えることを確かめる。
func TestMigrateExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		schema,
		`INSERT INTO employees (department, generated_at) VALUES ('営業部', '2026-10-01T20:59:08+09:00')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	// 2 回開いても（サーバー B と C の両方が起動しても）壊れない。
	for range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordAction(ctx, 1, "退職届の受理", time.Now()); err != nil {
		t.Fatalf("既存の記録に処理を記録できない: %v", err)
	}
	r, err := s.Get(ctx, 1)
	if err != nil || r.Department != "営業部" || r.GeneratedAt != "2026-10-01T20:59:08+09:00" || r.Action != "退職届の受理" {
		t.Errorf("record = %+v, %v", r, err)
	}
	if n, err := s.Issue(ctx, "営業部", time.Now(), func(int64) error { return nil }); err != nil || n != 2 {
		t.Errorf("採番が続きから: n=%d err=%v", n, err)
	}
}
