package ledger

// Tables are created and extended by gorm AutoMigrate on Start. AutoMigrate only
// adds tables, columns and indexes: renames or drops need an explicit migration.
// Timestamps are unix milliseconds.

// event is one payload's trip through a pipeline.
type event struct {
	ID         uint   `gorm:"primaryKey"`
	TS         int64  `gorm:"not null;index"`
	Session    string `gorm:"not null;default:'';index"`
	Agent      string `gorm:"not null;default:'';index"`
	Model      string `gorm:"not null;default:'';index"`
	Command    string `gorm:"not null;default:''"`
	Pipeline   string `gorm:"not null"`
	Key        string `gorm:"not null;index"`
	Route      string `gorm:"not null"`
	Optimizers string `gorm:"not null"` // comma separated ids
	RawTokens  int    `gorm:"not null"`
	OutTokens  int    `gorm:"not null"`
	DurationMS int64  `gorm:"not null"`
	RawID      string `gorm:"not null"`
	Outcome    string `gorm:"not null"`
	Filter     string `gorm:"not null;default:''"`
	Reason     string `gorm:"not null;default:''"`
	Steps      []step `gorm:"foreignKey:EventID"`
}

// step is one optimizer's effect within an event.
type step struct {
	ID        uint   `gorm:"primaryKey"`
	EventID   uint   `gorm:"not null;index"`
	Optimizer string `gorm:"not null"`
	Family    string `gorm:"not null;index"`
	Dialect   string `gorm:"not null;default:'';index"`
	RawTokens int    `gorm:"not null"`
	OutTokens int    `gorm:"not null"`
}

// raw is an original payload kept retrievable until TTL prune.
type raw struct {
	ID      string `gorm:"primaryKey"`
	Created int64  `gorm:"not null;index"`
	Key     string `gorm:"not null"`
	Payload []byte `gorm:"not null"` // JSON encoded payload.Payload
	Output  []byte // gzip optimized text of the latest run with this original, for comparison
}

func (raw) TableName() string { return "raw" }

// failure counts how often a source key optimized badly.
type failure struct {
	Key    string `gorm:"primaryKey"`
	Count  int    `gorm:"not null"`
	LastTS int64  `gorm:"not null"`
	Reason string `gorm:"not null"`
}

// dailyStat holds the totals of runs pruned at events_ttl, one row per local
// day and dimension. It has no session or reason: those stay with the runs.
type dailyStat struct {
	Day       string `gorm:"primaryKey"` // local YYYY-MM-DD
	Agent     string `gorm:"primaryKey"`
	Model     string `gorm:"primaryKey"`
	Command   string `gorm:"primaryKey"`
	Route     string `gorm:"primaryKey"`
	Outcome   string `gorm:"primaryKey"`
	Runs      int    `gorm:"not null"`
	RawTokens int    `gorm:"not null"`
	OutTokens int    `gorm:"not null"` // as events count it: a rerequested run adds its original
	FirstTS   int64  `gorm:"not null"`
	LastTS    int64  `gorm:"not null"`
}

// dailyStepStat is dailyStat for the optimizer steps of the pruned runs.
type dailyStepStat struct {
	Day       string `gorm:"primaryKey"`
	Agent     string `gorm:"primaryKey"`
	Model     string `gorm:"primaryKey"`
	Optimizer string `gorm:"primaryKey"`
	Family    string `gorm:"primaryKey"`
	Dialect   string `gorm:"primaryKey"`
	Runs      int    `gorm:"not null"`
	RawTokens int    `gorm:"not null"`
	OutTokens int    `gorm:"not null"`
	FirstTS   int64  `gorm:"not null"`
	LastTS    int64  `gorm:"not null"`
}
