// Package store は社員番号の採番と発行記録を SQLite に保存する。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS employees (
	id           INTEGER PRIMARY KEY AUTOINCREMENT, -- 社員番号
	department   TEXT    NOT NULL,                  -- 社員証に印字した部署名
	generated_at TEXT    NOT NULL                   -- 画像生成時刻（RFC 3339）
);`

// Store は社員番号の発行記録を扱う。
type Store struct {
	db *sql.DB
}

// Open は path の SQLite データベースを開き、必要ならテーブルを作成する。
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	// 書き込みは常に 1 本ずつ行い、採番を直列化する。
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("スキーマを作成できません: %w", err)
	}
	return &Store{db: db}, nil
}

// Close はデータベースを閉じる。
func (s *Store) Close() error {
	return s.db.Close()
}

// FormatNumber は社員番号を 6 桁ゼロ埋めの文字列にする（1 → "000001"）。
func FormatNumber(n int64) string {
	return fmt.Sprintf("%06d", n)
}

// Issue は新しい社員番号を採番して記録し、その番号で generate を呼ぶ。
// generate が失敗した場合は記録を取り消すため、社員番号に欠番は生じない
// （SQLite の AUTOINCREMENT のカウンタもロールバックで巻き戻る）。
func (s *Store) Issue(ctx context.Context, department string, generatedAt time.Time, generate func(number int64) error) (number int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO employees (department, generated_at) VALUES (?, ?)`,
		department, generatedAt.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	number, err = res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := generate(number); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return number, nil
}

// Record は 1 件分の発行記録。
type Record struct {
	Number      int64
	Department  string
	GeneratedAt string
}

// List はすべての発行記録を社員番号順に返す。
func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, department, generated_at FROM employees ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.Number, &r.Department, &r.GeneratedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
