package ledger

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// rawReason starts the reason of a run skipped because the agent asked for
// the original.
const rawReason = "raw requested"

// markRerequested turns the latest optimized run of key at or before ts
// into a rerequested one: its result was not enough for the agent. With a
// session, only that session's runs count.
func markRerequested(db *gorm.DB, key string, ts int64, session *string) error {
	latest := db.Table("events").Select("max(id)").
		Where("key = ? AND outcome = ? AND ts <= ?", key, component.Optimized, ts)
	if session != nil {
		latest = latest.Where("session = ?", *session)
	}
	return db.Table("events").Where("id = (?)", latest).Update("outcome", string(component.Rerequested)).Error
}

// schemaRerequested is the user_version after migrateRerequested.
const schemaRerequested = 2

// migrateRerequested marks runs recorded before the rerequested outcome
// existed: optimized runs whose original was fetched in full (failure
// raw_requested) or that a raw rerun of the same source followed.
// Once per database (user_version), as it is not idempotent.
func (l *Ledger) migrateRerequested(ctx context.Context) error {
	db := l.db.WithContext(ctx)
	var version int
	if err := db.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		return err
	}
	if version >= schemaRerequested {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var fetched []failure
		if err := tx.Where("reason = ?", component.ReasonRawRequested).Find(&fetched).Error; err != nil {
			return err
		}
		for _, f := range fetched {
			if err := markRerequested(tx, f.Key, f.LastTS, nil); err != nil {
				return fmt.Errorf("rerequested %s: %w", f.Key, err)
			}
		}
		var reruns []event
		if err := tx.Where("reason LIKE ?", rawReason+"%").Order("id").Find(&reruns).Error; err != nil {
			return err
		}
		for _, e := range reruns {
			if err := markRerequested(tx, e.Key, e.TS, &e.Session); err != nil {
				return fmt.Errorf("rerequested %s: %w", e.Key, err)
			}
		}
		return tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaRerequested)).Error
	})
}

// isRawRerun says whether ev is a run skipped on the agent's request.
func isRawRerun(ev component.Event) bool {
	return ev.Outcome == component.Bypassed && strings.HasPrefix(ev.Reason, rawReason)
}
