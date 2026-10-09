package ledger

// Tables are created and extended by gorm AutoMigrate on Start. AutoMigrate only
// adds tables, columns and indexes: renames or drops need an explicit migration.
// Timestamps are unix milliseconds.

// event is one payload's trip through a pipeline.
type event struct {
	ID         uint   `gorm:"primaryKey"`
	TS         int64  `gorm:"not null"`
	Pipeline   string `gorm:"not null"`
	Key        string `gorm:"not null;index"`
	Route      string `gorm:"not null"`
	Optimizers string `gorm:"not null"` // comma separated ids
	RawTokens  int    `gorm:"not null"`
	OutTokens  int    `gorm:"not null"`
	DurationMS int64  `gorm:"not null"`
	RawID      string `gorm:"not null"`
	Outcome    string `gorm:"not null"`
}

// raw is an original payload kept retrievable until TTL prune.
type raw struct {
	ID      string `gorm:"primaryKey"`
	Created int64  `gorm:"not null;index"`
	Key     string `gorm:"not null"`
	Payload []byte `gorm:"not null"` // JSON encoded payload.Payload
}

func (raw) TableName() string { return "raw" }

// failure counts how often a source key optimized badly.
type failure struct {
	Key    string `gorm:"primaryKey"`
	Count  int    `gorm:"not null"`
	LastTS int64  `gorm:"not null"`
	Reason string `gorm:"not null"`
}
