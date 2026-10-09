package ledger

import (
	"context"
	"time"

	"github.com/lukaszb/gascoigne/component"
)

func (l *Ledger) Stats(ctx context.Context) ([]component.Stat, error) {
	var stats []component.Stat
	err := l.db.WithContext(ctx).Model(&event{}).
		Select(`CASE route WHEN '' THEN '-' ELSE route END AS route,
			count(*) AS runs,
			sum(outcome = 'optimized') AS optimized,
			sum(outcome = 'error') AS failed,
			sum(raw_tokens) AS raw_tokens,
			sum(out_tokens) AS out_tokens`).
		Group("events.route").
		Order("sum(raw_tokens) - sum(out_tokens) DESC, route").
		Scan(&stats).Error
	return stats, err
}

type Failure struct {
	Key    string
	Count  int
	Last   time.Time
	Reason string
}

func (l *Ledger) FailureList(ctx context.Context) ([]Failure, error) {
	var rows []failure
	if err := l.db.WithContext(ctx).Order("last_ts DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	fs := make([]Failure, len(rows))
	for i, r := range rows {
		fs[i] = Failure{r.Key, r.Count, time.UnixMilli(r.LastTS), r.Reason}
	}
	return fs, nil
}
