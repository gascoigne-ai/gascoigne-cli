package ledger

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"gorm.io/gorm"
)

// usage is one LLM API request's billed tokens, from the proxied response.
type usage struct {
	ID         uint   `gorm:"primaryKey"`
	TS         int64  `gorm:"not null;index"`
	Agent      string `gorm:"not null;default:'';index"`
	Model      string `gorm:"not null;default:'';index"`
	Input      int    `gorm:"not null"`
	CacheRead  int    `gorm:"not null"`
	CacheWrite int    `gorm:"not null"`
	Output     int    `gorm:"not null"`
}

func (usage) TableName() string { return "usage" }

func (l *Ledger) RecordUsage(ctx context.Context, u component.Usage) error {
	return l.db.WithContext(ctx).Create(&usage{
		TS: u.TS.UnixMilli(), Agent: u.Agent, Model: u.Model,
		Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output,
	}).Error
}

// UsageStat sums the requests of one agent and model.
type UsageStat struct {
	Agent      string `json:"agent"`
	Model      string `json:"model"`
	Requests   int    `json:"requests"`
	Input      int    `json:"input"`
	CacheRead  int    `json:"cache_read"`
	CacheWrite int    `json:"cache_write"`
	Output     int    `json:"output"`
}

// UsageStats sums recorded API usage per agent and model, most requests
// first. Usage carries no session: a session filter matches nothing.
func (l *Ledger) UsageStats(ctx context.Context, q component.StatsQuery) ([]UsageStat, error) {
	var out []UsageStat
	if q.Session != "" {
		return out, nil
	}
	err := usageFilter(l.db.WithContext(ctx).Model(&usage{}), q).
		Select(`agent, model, count(*) AS requests, sum(input) AS input, sum(cache_read) AS cache_read,
			sum(cache_write) AS cache_write, sum(output) AS output`).
		Group("agent, model").Order("requests DESC, agent, model").Scan(&out).Error
	return out, err
}

func usageFilter(tx *gorm.DB, q component.StatsQuery) *gorm.DB {
	if q.Agent != "" {
		tx = tx.Where("agent = ?", q.Agent)
	}
	if q.Model != "" {
		tx = tx.Where("model = ?", q.Model)
	}
	if !q.Since.IsZero() {
		tx = tx.Where("ts >= ?", q.Since.UnixMilli())
	}
	return tx
}
