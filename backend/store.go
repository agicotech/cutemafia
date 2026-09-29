package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var (
	errNotFound = errors.New("not found")
	errConflict = errors.New("conflict")
)

type store struct {
	db       *sql.DB
	postgres bool
}

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS kittens (
        id TEXT PRIMARY KEY,
        payload TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS inquiries (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        contact TEXT NOT NULL,
        message TEXT NOT NULL,
        kitten_id TEXT,
        status TEXT NOT NULL,
        created_at TEXT NOT NULL,
        notification_status TEXT NOT NULL,
        notification_attempts INTEGER NOT NULL,
        notification_next_attempt_at TEXT NOT NULL,
        notification_last_error TEXT
    )`,
	`CREATE INDEX IF NOT EXISTS inquiries_notification_queue
      ON inquiries (notification_status, notification_next_attempt_at)`,
}

func openStore(ctx context.Context, cfg config) (*store, error) {
	driver, dsn := cfg.Database.Driver, cfg.Database.DSN
	if driver == "sqlite" {
		if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			if err := os.MkdirAll(filepath.Dir(dsn), 0o750); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
			dsn = "file:" + dsn + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
		}
	}
	if driver == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if cfg.Database.Driver == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(2)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	s := &store{db: db, postgres: cfg.Database.Driver == "postgres"}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) bind(query string) string {
	if !s.postgres {
		return query
	}
	var b strings.Builder
	arg := 1
	for _, r := range query {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", arg)
			arg++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	for index, statement := range migrations {
		version := index + 1
		var exists int
		if err := s.db.QueryRowContext(ctx, s.bind("SELECT COUNT(*) FROM schema_migrations WHERE version = ?"), version).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, statement); err == nil {
			_, err = tx.ExecContext(ctx, s.bind("INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)"), version, nowText())
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) listKittens(ctx context.Context) ([]kitten, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM kittens ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]kitten, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item kitten
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, fmt.Errorf("decode kitten: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *store) getKitten(ctx context.Context, id string) (kitten, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, s.bind("SELECT payload FROM kittens WHERE id = ?"), id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return kitten{}, errNotFound
	}
	if err != nil {
		return kitten{}, err
	}
	var item kitten
	if err := json.Unmarshal(payload, &item); err != nil {
		return kitten{}, err
	}
	return item, nil
}

func (s *store) createKitten(ctx context.Context, item kitten) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	now := nowText()
	_, err = s.db.ExecContext(ctx, s.bind("INSERT INTO kittens(id, payload, created_at, updated_at) VALUES (?, ?, ?, ?)"), item.ID, string(payload), now, now)
	if isUniqueViolation(err) {
		return errConflict
	}
	return err
}

func (s *store) updateKitten(ctx context.Context, item kitten) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.bind("UPDATE kittens SET payload = ?, updated_at = ? WHERE id = ?"), string(payload), nowText(), item.ID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *store) deleteKitten(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, s.bind("DELETE FROM kittens WHERE id = ?"), id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *store) createInquiry(ctx context.Context, item inquiry) error {
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO inquiries(
        id, name, contact, message, kitten_id, status, created_at,
        notification_status, notification_attempts, notification_next_attempt_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		item.ID, item.Name, item.Contact, item.Message, item.KittenID, item.Status,
		item.CreatedAt, "pending", 0, item.CreatedAt)
	return err
}

type pendingInquiry struct {
	inquiry
	Attempts int
}

func (s *store) nextPendingInquiry(ctx context.Context, now time.Time) (pendingInquiry, error) {
	var item pendingInquiry
	var kittenID sql.NullString
	err := s.db.QueryRowContext(ctx, s.bind(`SELECT id, name, contact, message, kitten_id, status, created_at, notification_attempts
      FROM inquiries
      WHERE notification_status = 'pending' AND notification_next_attempt_at <= ?
	      ORDER BY notification_next_attempt_at LIMIT 1`), databaseTime(now)).Scan(
		&item.ID, &item.Name, &item.Contact, &item.Message, &kittenID, &item.Status, &item.CreatedAt, &item.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errNotFound
	}
	if kittenID.Valid {
		item.KittenID = &kittenID.String
	}
	return item, err
}

func (s *store) markInquirySent(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.bind("UPDATE inquiries SET notification_status = 'sent', notification_last_error = NULL WHERE id = ?"), id)
	return err
}

func (s *store) markInquiryRetry(ctx context.Context, id string, attempts int, next time.Time, message string) error {
	_, err := s.db.ExecContext(ctx, s.bind(`UPDATE inquiries
      SET notification_attempts = ?, notification_next_attempt_at = ?, notification_last_error = ?
	      WHERE id = ?`), attempts, databaseTime(next), message, id)
	return err
}

func requireAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unique constraint") || strings.Contains(text, "duplicate key")
}

func nowText() string { return databaseTime(time.Now()) }

func databaseTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
