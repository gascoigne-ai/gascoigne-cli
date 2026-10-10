// Package ledger is a sqlite-backed extension recording gains, failures and
// original payloads, so optimizations stay reversible and bad ones are learned.
package ledger

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

var ErrNotFound = errors.New("not found")

type Ledger struct {
	cfg  *Config
	db   *gorm.DB
	path string
}

// Path is the database file in use, after the env override and defaults.
func (l *Ledger) Path() string { return l.path }

var _ component.Ledger = (*Ledger)(nil)

func New(cfg *Config) *Ledger {
	return &Ledger{cfg: cfg}
}

func (l *Ledger) Start(ctx context.Context, _ component.Host) error {
	path, err := l.cfg.path()
	if err != nil {
		return err
	}
	l.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	dsn := "file:" + url.PathEscape(path) + "?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)"
	l.db, err = gorm.Open(gormlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	if err := l.db.WithContext(ctx).AutoMigrate(&event{}, &step{}, &raw{}, &failure{}); err != nil {
		return fmt.Errorf("ledger migrate: %w", err)
	}
	return l.prune(ctx)
}

func (l *Ledger) Shutdown(context.Context) error {
	if l.db == nil {
		return nil
	}
	db, err := l.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

func (l *Ledger) prune(ctx context.Context) error {
	if l.cfg.RawTTL == 0 {
		return nil
	}
	cutoff := time.Now().Add(-l.cfg.RawTTL).UnixMilli()
	return l.db.WithContext(ctx).Where("created < ?", cutoff).Delete(&raw{}).Error
}

// SaveRaw stores p and returns a content-derived id; saving again refreshes its TTL.
func (l *Ledger) SaveRaw(ctx context.Context, key string, p *payload.Payload) (string, error) {
	blob, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(blob)
	packed, err := pack(blob)
	if err != nil {
		return "", err
	}
	r := raw{ID: hex.EncodeToString(sum[:8]), Created: time.Now().UnixMilli(), Key: key, Payload: packed}
	err = l.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"created"}),
	}).Create(&r).Error
	return r.ID, err
}

func (l *Ledger) GetRaw(ctx context.Context, id string) (string, *payload.Payload, error) {
	var r raw
	err := l.db.WithContext(ctx).Take(&r, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil, fmt.Errorf("raw %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", nil, err
	}
	blob, err := unpack(r.Payload)
	if err != nil {
		return "", nil, fmt.Errorf("raw %s: %w", id, err)
	}
	var p payload.Payload
	return r.Key, &p, json.Unmarshal(blob, &p)
}

// pack gzips an original: command output compresses several times over.
func pack(blob []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(blob); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// unpack reverses pack. Rows saved before compression are plain JSON,
// which never starts with the gzip magic.
func unpack(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		return data, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// RawUsage is what stored originals take up.
type RawUsage struct {
	Count int64 `json:"count"`
	Bytes int64 `json:"bytes"` // compressed, as stored
}

// RawUsage counts stored originals and their size.
func (l *Ledger) RawUsage(ctx context.Context) (RawUsage, error) {
	var u RawUsage
	err := l.db.WithContext(ctx).Model(&raw{}).Select("count(*) AS count, coalesce(sum(length(payload)), 0) AS bytes").Scan(&u).Error
	return u, err
}

// FlushRaw deletes stored originals saved before cutoff (zero: all) and
// gives the space back to the disk. Their hints stop resolving.
func (l *Ledger) FlushRaw(ctx context.Context, cutoff time.Time) (int64, error) {
	q := l.db.WithContext(ctx).Where("1 = 1")
	if !cutoff.IsZero() {
		q = q.Where("created < ?", cutoff.UnixMilli())
	}
	res := q.Delete(&raw{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, l.db.WithContext(ctx).Exec("VACUUM").Error
}

func (l *Ledger) Record(ctx context.Context, ev component.Event) error {
	steps := make([]step, len(ev.Steps))
	for i, s := range ev.Steps {
		steps[i] = step{Optimizer: s.Optimizer, Family: s.Family, Dialect: s.Dialect, RawTokens: s.RawTokens, OutTokens: s.OutTokens}
	}
	return l.db.WithContext(ctx).Create(&event{
		TS:         ev.TS.UnixMilli(),
		Session:    ev.Session,
		Agent:      ev.Agent,
		Model:      ev.Model,
		Command:    ev.Command,
		Filter:     ev.Filter,
		Reason:     ev.Reason,
		Pipeline:   ev.Pipeline,
		Key:        ev.Key,
		Route:      ev.Route,
		Optimizers: strings.Join(ev.Optimizers, ","),
		RawTokens:  ev.RawTokens,
		OutTokens:  ev.OutTokens,
		DurationMS: ev.Duration.Milliseconds(),
		RawID:      ev.RawID,
		Outcome:    string(ev.Outcome),
		Steps:      steps,
	}).Error
}

func (l *Ledger) MarkFailure(ctx context.Context, key, reason string) error {
	f := failure{Key: key, Count: 1, LastTS: time.Now().UnixMilli(), Reason: reason}
	return l.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":   gorm.Expr("count + 1"),
			"last_ts": f.LastTS,
			"reason":  reason,
		}),
	}).Create(&f).Error
}

func (l *Ledger) Failures(ctx context.Context, key string) (component.FailureInfo, error) {
	var f failure
	err := l.db.WithContext(ctx).Take(&f, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return component.FailureInfo{}, nil
	}
	return component.FailureInfo{Count: f.Count, Reason: f.Reason, Last: time.UnixMilli(f.LastTS)}, err
}

// Forget clears failure history for key, re-enabling optimization.
func (l *Ledger) Forget(ctx context.Context, key string) error {
	return l.db.WithContext(ctx).Delete(&failure{Key: key}).Error
}
