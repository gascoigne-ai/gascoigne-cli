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

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

var ErrNotFound = errors.New("not found")

type Ledger struct {
	cfg  *Config
	db   *gorm.DB
	path string
	now  func() time.Time // a field so tests can move the clock
}

// Path is the database file in use, after the env override and defaults.
func (l *Ledger) Path() string { return l.path }

var _ component.Ledger = (*Ledger)(nil)

func New(cfg *Config) *Ledger {
	return &Ledger{cfg: cfg, now: time.Now}
}

func (l *Ledger) Start(ctx context.Context, _ component.Host) error {
	path, err := l.cfg.File()
	if err != nil {
		return err
	}
	l.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Originals of tool output live here; sqlite would create the file 0644.
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		f.Close()
	}
	dsn := "file:" + url.PathEscape(path) + "?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)"
	l.db, err = gorm.Open(gormlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	if err := l.db.WithContext(ctx).AutoMigrate(&event{}, &step{}, &raw{}, &failure{}, &usage{}, &toolStat{}); err != nil {
		return fmt.Errorf("ledger migrate: %w", err)
	}
	if err := l.migrateKeys(ctx); err != nil {
		return fmt.Errorf("ledger migrate keys: %w", err)
	}
	if err := l.migrateRerequested(ctx); err != nil {
		return fmt.Errorf("ledger migrate rerequested: %w", err)
	}
	if err := l.pruneToolStats(ctx); err != nil {
		return err
	}
	if err := l.pruneEvents(ctx); err != nil {
		return err
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

// pruneEvents drops runs, their steps, API usage and failure history older
// than EventsTTL; 0 keeps them all.
func (l *Ledger) pruneEvents(ctx context.Context) error {
	if l.cfg.EventsTTL == 0 {
		return nil
	}
	cutoff := l.now().Add(-l.cfg.EventsTTL).UnixMilli()
	return l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		old := tx.Model(&event{}).Select("id").Where("ts < ?", cutoff)
		if err := tx.Where("event_id IN (?)", old).Delete(&step{}).Error; err != nil {
			return err
		}
		if err := tx.Where("ts < ?", cutoff).Delete(&event{}).Error; err != nil {
			return err
		}
		if err := tx.Where("ts < ?", cutoff).Delete(&usage{}).Error; err != nil {
			return err
		}
		return tx.Where("last_ts < ?", cutoff).Delete(&failure{}).Error
	})
}

// SaveRaw stores p and returns a content-derived id; saving again refreshes its TTL.
// idLengths are the hex lengths a raw id tries, shortest first: every hint
// carries one, and 8 hex digits cost ~7 fewer tokens than 16. A prefix
// already holding another original moves on to a longer one, so ids stay
// unique and content-addressed: the same text gets the same id, whatever
// its attrs (session, agent), so an output seen twice is stored once.
var idLengths = []int{8, 12, 16, 64}

func (l *Ledger) SaveRaw(ctx context.Context, key string, p *payload.Payload) (string, error) {
	blob, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	full := sha256.Sum256([]byte(p.Text()))
	sum := hex.EncodeToString(full[:])
	packed, err := pack(blob)
	if err != nil {
		return "", err
	}
	db := l.db.WithContext(ctx)
	for _, n := range idLengths {
		id := sum[:n]
		taken, same, err := l.holds(ctx, id, p.Text())
		if err != nil {
			return "", err
		}
		if !taken {
			// Another process may insert the same id in between: then the
			// row decides, as if it had been there.
			err := db.Create(&raw{ID: id, Created: time.Now().UnixMilli(), Key: storedKey(key), Payload: packed}).Error
			if err == nil {
				return id, nil
			}
			if _, same, _ = l.holds(ctx, id, p.Text()); !same {
				continue
			}
		}
		if same {
			return id, db.Model(&raw{}).Where("id = ?", id).Update("created", time.Now().UnixMilli()).Error
		}
	}
	return "", fmt.Errorf("raw: no free id for %s", sum)
}

// holds reports whether id is taken and whether by the same text.
func (l *Ledger) holds(ctx context.Context, id, text string) (taken, same bool, err error) {
	var r raw
	err = l.db.WithContext(ctx).Select("payload").Take(&r, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	blob, err := unpack(r.Payload)
	if err != nil {
		return true, false, nil
	}
	var old payload.Payload
	return true, json.Unmarshal(blob, &old) == nil && old.Text() == text, nil
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
	if ev.RawID != "" && ev.Output != "" {
		if err := l.saveOutput(ctx, ev.RawID, ev.Output); err != nil {
			return err
		}
	}
	steps := make([]step, len(ev.Steps))
	for i, s := range ev.Steps {
		steps[i] = step{Optimizer: s.Optimizer, Family: s.Family, Dialect: s.Dialect, RawTokens: s.RawTokens, OutTokens: s.OutTokens}
	}
	db := l.db.WithContext(ctx)
	if isRawRerun(ev) { // the agent was not content with an earlier optimized result
		if err := markRerequested(db, storedKey(ev.Key), ev.TS.UnixMilli(), &ev.Session); err != nil {
			return err
		}
	}
	return db.Create(&event{
		TS:         ev.TS.UnixMilli(),
		Session:    ev.Session,
		Agent:      ev.Agent,
		Model:      ev.Model,
		Command:    ev.Command,
		Filter:     ev.Filter,
		Reason:     ev.Reason,
		Pipeline:   ev.Pipeline,
		Key:        storedKey(ev.Key),
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
	f := failure{Key: storedKey(key), Count: 1, LastTS: time.Now().UnixMilli(), Reason: reason}
	if reason == component.ReasonRawRequested {
		if err := markRerequested(l.db.WithContext(ctx), f.Key, f.LastTS, nil); err != nil {
			return err
		}
	}
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
	err := l.db.WithContext(ctx).Take(&f, "key = ?", storedKey(key)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return component.FailureInfo{}, nil
	}
	return component.FailureInfo{Count: f.Count, Reason: f.Reason, Last: time.UnixMilli(f.LastTS)}, err
}

// Forget clears failure history for key, re-enabling optimization.
func (l *Ledger) Forget(ctx context.Context, key string) error {
	return l.db.WithContext(ctx).Delete(&failure{Key: storedKey(key)}).Error
}

func (l *Ledger) saveOutput(ctx context.Context, id, text string) error {
	packed, err := pack([]byte(text))
	if err != nil {
		return err
	}
	return l.db.WithContext(ctx).Model(&raw{}).Where("id = ?", id).Update("output", packed).Error
}

// RecentOutputs returns outputs of the latest optimized runs of key in
// session since the given time, newest first (component.OutputHistory).
func (l *Ledger) RecentOutputs(ctx context.Context, session, key string, since time.Time, limit int) ([]string, error) {
	var ids []string
	err := l.db.WithContext(ctx).Model(&event{}).
		Where("session = ? AND key = ? AND raw_id != '' AND ts >= ?", session, storedKey(key), since.UnixMilli()).
		Order("ts DESC, id DESC").Limit(limit).Pluck("raw_id", &ids).Error
	if err != nil {
		return nil, err
	}
	var outs []string
	for _, id := range ids {
		out, ok, err := l.GetOutput(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			outs = append(outs, out)
		}
	}
	return outs, nil
}

// GetOutput returns the optimized text last produced from original id;
// false when none was kept (runs from before outputs were kept, or pruned).
func (l *Ledger) GetOutput(ctx context.Context, id string) (string, bool, error) {
	var r raw
	err := l.db.WithContext(ctx).Select("output").Where("id = ?", id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && len(r.Output) == 0 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	out, err := unpack(r.Output)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}
