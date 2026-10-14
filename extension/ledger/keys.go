package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"gorm.io/gorm"
)

// maxKey is the longest source key stored as it is. Keys are whole command
// lines (heredoc scripts included), repeated on every event and indexed: a
// longer one keeps a readable prefix and a hash of the whole, which still
// tells sources apart.
const (
	maxKey    = 200
	keyPrefix = 160
)

// storedKey is the form of key the tables hold.
func storedKey(key string) string {
	if len(key) <= maxKey {
		return key
	}
	cut := keyPrefix
	for cut > 0 && !utf8.RuneStart(key[cut]) {
		cut--
	}
	sum := sha256.Sum256([]byte(key))
	return key[:cut] + "…#" + hex.EncodeToString(sum[:8])
}

// schemaVersion is the user_version after migrateKeys; migrateRerequested
// raises it further.
const schemaVersion = 1

// migrateKeys shortens keys stored before storedKey, once per database
// (user_version), and reclaims the space they took.
func (l *Ledger) migrateKeys(ctx context.Context) error {
	db := l.db.WithContext(ctx)
	var version int
	if err := db.Raw("PRAGMA user_version").Scan(&version).Error; err != nil {
		return err
	}
	if version >= schemaVersion {
		return nil
	}
	changed := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"events", "raw", "failures"} {
			var keys []string
			if err := tx.Table(table).Distinct("key").Where("length(key) > ?", maxKey).Pluck("key", &keys).Error; err != nil {
				return fmt.Errorf("%s keys: %w", table, err)
			}
			for _, k := range keys {
				if err := tx.Table(table).Where("key = ?", k).Update("key", storedKey(k)).Error; err != nil {
					return fmt.Errorf("%s key: %w", table, err)
				}
			}
			changed += len(keys)
		}
		return tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)).Error
	})
	if err != nil || changed == 0 {
		return err
	}
	return db.Exec("VACUUM").Error
}
