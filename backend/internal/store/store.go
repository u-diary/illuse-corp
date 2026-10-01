// Package store は社員番号の採番と発行記録、および退職・解雇の処理記録を SQLite に保存する。
// サーバー B（onboard）とサーバー C（fire）が同じデータベースファイルを共有する。
package store

import (
	"context"
	"database/sql"
	"errors"
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

// migrations は PRAGMA user_version で管理するスキーマの変更。i 番目を適用すると user_version が i+1 になる。
var migrations = []string{
	// v1: fire で処理した内容と時刻
	`ALTER TABLE employees ADD COLUMN action TEXT;     -- 処理内容（例: 退職届の受理）
	 ALTER TABLE employees ADD COLUMN action_at TEXT;  -- 処理時刻（RFC 3339）`,
}

// 処理を記録できなかった理由。
var (
	ErrNotFound         = errors.New("store: 該当する社員がいません")
	ErrAlreadyProcessed = errors.New("store: すでに処理済みです")
)

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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("スキーマを作成できません: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate はテーブルを作成し、未適用のスキーマ変更を適用する。
// トランザクションは BEGIN IMMEDIATE なので、サーバー B と C が同時に起動しても順番に処理される。
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.Exec(migrations[i]); err != nil {
			return fmt.Errorf("v%d: %w", i+1, err)
		}
	}
	if version < len(migrations) {
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// Record は 1 件分の記録。Action と ActionAt は未処理なら空文字列。
type Record struct {
	Number      int64
	Department  string
	GeneratedAt string
	Action      string
	ActionAt    string
}

const selectRecord = `SELECT id, department, generated_at, coalesce(action, ''), coalesce(action_at, '') FROM employees`

func scanRecord(row interface{ Scan(...any) error }) (Record, error) {
	var r Record
	err := row.Scan(&r.Number, &r.Department, &r.GeneratedAt, &r.Action, &r.ActionAt)
	return r, err
}

// Get は社員番号 number の記録を返す。存在しなければ ErrNotFound を返す。
func (s *Store) Get(ctx context.Context, number int64) (Record, error) {
	r, err := scanRecord(s.db.QueryRowContext(ctx, selectRecord+` WHERE id = ?`, number))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return r, err
}

// List はすべての記録を社員番号順に返す。
func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, selectRecord+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// RecordAction は社員番号 number の社員に、処理内容 action と処理時刻 at を記録する。
// 社員がいなければ ErrNotFound、すでに処理済みなら ErrAlreadyProcessed を返す。
// 判定と記録は 1 つの UPDATE で行うので、同じ社員を同時に処理しても記録されるのは 1 回だけ。
func (s *Store) RecordAction(ctx context.Context, number int64, action string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE employees SET action = ?, action_at = ? WHERE id = ? AND action IS NULL`,
		action, at.Format(time.RFC3339), number)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	// 記録できなかった理由を調べる。
	if _, err := s.Get(ctx, number); err != nil {
		return err
	}
	return ErrAlreadyProcessed
}
