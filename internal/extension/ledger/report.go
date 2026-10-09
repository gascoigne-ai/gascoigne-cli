package ledger

import (
	"context"
	"time"
)

type Stat struct {
	Route     string
	Runs      int
	Optimized int
	Failed    int
	RawTokens int
	OutTokens int
}

func (s Stat) Saved() int { return s.RawTokens - s.OutTokens }

// Gain aggregates events per route, biggest savings first.
func (l *Ledger) Gain(ctx context.Context) ([]Stat, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT
			CASE route WHEN '' THEN '-' ELSE route END,
			count(*), sum(outcome = 'optimized'), sum(outcome = 'error'),
			sum(raw_tokens), sum(out_tokens)
		FROM events GROUP BY 1 ORDER BY sum(raw_tokens) - sum(out_tokens) DESC, 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []Stat
	for rows.Next() {
		var s Stat
		if err := rows.Scan(&s.Route, &s.Runs, &s.Optimized, &s.Failed, &s.RawTokens, &s.OutTokens); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

type Failure struct {
	Key    string
	Count  int
	Last   time.Time
	Reason string
}

func (l *Ledger) FailureList(ctx context.Context) ([]Failure, error) {
	rows, err := l.db.QueryContext(ctx, "SELECT key, count, last_ts, reason FROM failures ORDER BY last_ts DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fs []Failure
	for rows.Next() {
		var (
			f  Failure
			ts int64
		)
		if err := rows.Scan(&f.Key, &f.Count, &ts, &f.Reason); err != nil {
			return nil, err
		}
		f.Last = time.UnixMilli(ts)
		fs = append(fs, f)
	}
	return fs, rows.Err()
}
