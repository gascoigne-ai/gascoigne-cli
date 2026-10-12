package ledger

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// toolStat is one MCP tool's footprint on one day: what its definition costs
// in every request and how often it was called. One row per day, so a year
// is a few thousand rows however busy the agent is.
type toolStat struct {
	Day    string `gorm:"primaryKey"` // UTC, 2006-01-02
	Agent  string `gorm:"primaryKey"`
	Server string `gorm:"primaryKey"`
	Tool   string `gorm:"primaryKey"`
	Tokens int    `gorm:"not null"` // definition size at the last listing that day; 0 when never listed
	Calls  int    `gorm:"not null"`
}

func (toolStat) TableName() string { return "tool_stats" }

// ToolStat is one tool on one day, as ToolStats returns it.
type ToolStat struct {
	Day    string `json:"day"`
	Agent  string `json:"agent"`
	Server string `json:"server"`
	Tool   string `json:"tool"`
	Tokens int    `json:"tokens"`
	Calls  int    `json:"calls"`
}

const dayLayout = "2006-01-02"

func (l *Ledger) today() string { return l.now().UTC().Format(dayLayout) }

// RecordToolList stores the definition size of each tool a server lists
// (tokens by tool name). Calls already counted today are kept.
func (l *Ledger) RecordToolList(ctx context.Context, agent, server string, tokens map[string]int) error {
	day := l.today()
	for tool, n := range tokens {
		err := l.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "day"}, {Name: "agent"}, {Name: "server"}, {Name: "tool"}},
			DoUpdates: clause.AssignmentColumns([]string{"tokens"}),
		}).Create(&toolStat{Day: day, Agent: agent, Server: server, Tool: tool, Tokens: n}).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// RecordToolCall counts one call of a tool.
func (l *Ledger) RecordToolCall(ctx context.Context, agent, server, tool string) error {
	return l.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "day"}, {Name: "agent"}, {Name: "server"}, {Name: "tool"}},
		DoUpdates: clause.Assignments(map[string]any{"calls": clause.Expr{SQL: "calls + 1"}}),
	}).Create(&toolStat{Day: l.today(), Agent: agent, Server: server, Tool: tool, Calls: 1}).Error
}

// ToolStats returns the rows from the day of since on, oldest day first.
func (l *Ledger) ToolStats(ctx context.Context, since time.Time) ([]ToolStat, error) {
	var out []ToolStat
	tx := l.db.WithContext(ctx).Model(&toolStat{})
	if !since.IsZero() {
		tx = tx.Where("day >= ?", since.UTC().Format(dayLayout))
	}
	err := tx.Order("day, agent, server, tool").Scan(&out).Error
	return out, err
}

// pruneToolStats drops days older than StatsTTL; 0 keeps them all.
func (l *Ledger) pruneToolStats(ctx context.Context) error {
	if l.cfg.StatsTTL == 0 {
		return nil
	}
	cutoff := l.now().UTC().Add(-l.cfg.StatsTTL).Format(dayLayout)
	return l.db.WithContext(ctx).Where("day < ?", cutoff).Delete(&toolStat{}).Error
}

// CommandDay is how many runs of one command were recorded on a day.
type CommandDay struct {
	Day     string `json:"day"`
	Agent   string `json:"agent"`
	Command string `json:"command"`
	Runs    int    `json:"runs"`
}

// MCPCommands counts, per day and agent, the runs recorded for tools named
// mcp__server__tool: the hook's record of calls to MCP servers (Claude and
// Codex naming) that do not go through mcp-proxy. Everything from the day of
// since on.
func (l *Ledger) MCPCommands(ctx context.Context, since time.Time) ([]CommandDay, error) {
	var out []CommandDay
	tx := l.db.WithContext(ctx).Model(&event{}).
		Select(`strftime('%Y-%m-%d', ts / 1000, 'unixepoch') AS day, agent,
			CASE WHEN instr(command, ' ') > 0 THEN substr(command, 1, instr(command, ' ') - 1) ELSE command END AS command,
			count(*) AS runs`).
		Where(`command LIKE 'mcp\_\_%' ESCAPE '\'`)
	if !since.IsZero() {
		tx = tx.Where("ts >= ?", time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC).UnixMilli())
	}
	err := tx.Group("day, agent, 3").Order("day, agent, command").Scan(&out).Error
	return out, err
}
