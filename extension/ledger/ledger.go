// Package ledger is a sqlite-backed extension recording gains, failures and
// original payloads, so optimizations stay reversible and bad ones are learned.
package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")

type Ledger struct {
	cfg *Config
	db  *sql.DB
}

var _ component.Ledger = (*Ledger)(nil)

func New(cfg *Config) *Ledger {
	return &Ledger{cfg: cfg}
}

func (l *Ledger) Start(ctx context.Context, _ component.Host) error {
	path, err := l.cfg.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	l.db, err = sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	if err := l.migrate(ctx); err != nil {
		return fmt.Errorf("ledger migrate: %w", err)
	}
	return l.prune(ctx)
}

func (l *Ledger) Shutdown(context.Context) error {
	if l.db == nil {
		return nil
	}
	return l.db.Close()
}

func (l *Ledger) migrate(ctx context.Context) error {
	var version int
	if err := l.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	for i, name := range files[min(version, len(files)):] {
		data, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := l.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, string(data))
		if err == nil {
			_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version+i+1))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (l *Ledger) prune(ctx context.Context) error {
	if l.cfg.RawTTL == 0 {
		return nil
	}
	cutoff := time.Now().Add(-l.cfg.RawTTL).UnixMilli()
	_, err := l.db.ExecContext(ctx, "DELETE FROM raw WHERE created < ?", cutoff)
	return err
}

// SaveRaw stores p and returns a content-derived id; saving again refreshes its TTL.
func (l *Ledger) SaveRaw(ctx context.Context, key string, p *payload.Payload) (string, error) {
	blob, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(blob)
	id := hex.EncodeToString(sum[:8])
	_, err = l.db.ExecContext(ctx, `INSERT INTO raw(id, created, key, payload) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET created = excluded.created`,
		id, time.Now().UnixMilli(), key, blob)
	return id, err
}

func (l *Ledger) GetRaw(ctx context.Context, id string) (string, *payload.Payload, error) {
	var (
		key  string
		blob []byte
	)
	err := l.db.QueryRowContext(ctx, "SELECT key, payload FROM raw WHERE id = ?", id).Scan(&key, &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("raw %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", nil, err
	}
	var p payload.Payload
	return key, &p, json.Unmarshal(blob, &p)
}

func (l *Ledger) Record(ctx context.Context, ev component.Event) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO events
		(ts, pipeline, key, route, optimizers, raw_tokens, out_tokens, duration_ms, raw_id, outcome)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.TS.UnixMilli(), ev.Pipeline, ev.Key, ev.Route, strings.Join(ev.Optimizers, ","),
		ev.RawTokens, ev.OutTokens, ev.Duration.Milliseconds(), ev.RawID, string(ev.Outcome))
	return err
}

func (l *Ledger) MarkFailure(ctx context.Context, key, reason string) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO failures(key, count, last_ts, reason) VALUES (?, 1, ?, ?)
		ON CONFLICT(key) DO UPDATE SET count = count + 1, last_ts = excluded.last_ts, reason = excluded.reason`,
		key, time.Now().UnixMilli(), reason)
	return err
}

func (l *Ledger) Failures(ctx context.Context, key string) (int, error) {
	var n int
	err := l.db.QueryRowContext(ctx, "SELECT count FROM failures WHERE key = ?", key).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// Forget clears failure history for key, re-enabling optimization.
func (l *Ledger) Forget(ctx context.Context, key string) error {
	_, err := l.db.ExecContext(ctx, "DELETE FROM failures WHERE key = ?", key)
	return err
}
