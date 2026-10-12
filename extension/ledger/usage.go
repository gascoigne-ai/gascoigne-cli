package ledger

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"gorm.io/gorm"
)

// usage is one LLM API request's billed tokens, from the proxied response.
type usage struct {
	ID           uint   `gorm:"primaryKey"`
	TS           int64  `gorm:"not null;index"`
	Agent        string `gorm:"not null;default:'';index"`
	Model        string `gorm:"not null;default:'';index"`
	Input        int    `gorm:"not null"`
	CacheRead    int    `gorm:"not null"`
	CacheWrite   int    `gorm:"not null"`
	CacheWrite1h int    `gorm:"not null;default:0"`
	Output       int    `gorm:"not null"`
}

func (usage) TableName() string { return "usage" }

func (l *Ledger) RecordUsage(ctx context.Context, u component.Usage) error {
	return l.db.WithContext(ctx).Create(&usage{
		TS: u.TS.UnixMilli(), Agent: u.Agent, Model: u.Model,
		Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, CacheWrite1h: u.CacheWrite1h, Output: u.Output,
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
	// CacheWrite1h is the part of CacheWrite kept for an hour (dearer).
	CacheWrite1h int `json:"cache_write_1h"`
	Output       int `json:"output"`
	// MeanContext and PeakContext are the tokens one request carried (input,
	// cache read and cache write): the size of the conversation it re-read.
	MeanContext int `json:"mean_context"`
	PeakContext int `json:"peak_context"`
}

// UsageDay sums the requests of one UTC day over all agents and models matching
// the filters.
type UsageDay struct {
	Day          string `json:"day"`
	Requests     int    `json:"requests"`
	CacheRead    int    `json:"cache_read"`
	CacheWrite   int    `json:"cache_write"`
	CacheWrite1h int    `json:"cache_write_1h"`
	Output       int    `json:"output"`
	MeanContext  int    `json:"mean_context"`
	PeakContext  int    `json:"peak_context"`
}

const contextSQL = "(input + cache_read + cache_write)"

// UsageStats sums recorded API usage per agent and model, most requests
// first. Usage carries no session: a session filter matches nothing.
func (l *Ledger) UsageStats(ctx context.Context, q component.StatsQuery) ([]UsageStat, error) {
	var out []UsageStat
	if q.Session != "" {
		return out, nil
	}
	err := usageFilter(l.db.WithContext(ctx).Model(&usage{}), q).
		Select(`agent, model, count(*) AS requests, sum(input) AS input, sum(cache_read) AS cache_read,
			sum(cache_write) AS cache_write, sum(cache_write1h) AS cache_write1h, sum(output) AS output,
			sum` + contextSQL + ` / count(*) AS mean_context, max` + contextSQL + ` AS peak_context`).
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

// UsageDays sums recorded API usage per UTC day, oldest first. Usage carries no
// session: a session filter matches nothing.
func (l *Ledger) UsageDays(ctx context.Context, q component.StatsQuery) ([]UsageDay, error) {
	var out []UsageDay
	if q.Session != "" {
		return out, nil
	}
	err := usageFilter(l.db.WithContext(ctx).Model(&usage{}), q).
		Select(`strftime('%Y-%m-%d', ts / 1000, 'unixepoch') AS day, count(*) AS requests,
			sum(cache_read) AS cache_read, sum(cache_write) AS cache_write, sum(cache_write1h) AS cache_write1h, sum(output) AS output,
			sum` + contextSQL + ` / count(*) AS mean_context, max` + contextSQL + ` AS peak_context`).
		Group("day").Order("day").Scan(&out).Error
	return out, err
}
