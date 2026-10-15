package detect

import "strings"

// Databases: query results, plans, key stores, migrations.
const (
	Psql    = "db/psql"
	MySQL   = "db/mysql"
	SQLite  = "db/sqlite"
	Explain = "db/explain"
	Redis   = "db/redis"
	Mongosh = "db/mongosh"
	Migrate = "db/migrate"
)

func init() {
	register(
		Spec{Type: Explain, Priority: 58,
			CmdFunc: func(args []string) bool {
				return (args[0] == "psql" || args[0] == "mysql") && strings.Contains(strings.ToUpper(strings.Join(args, " ")), "EXPLAIN")
			},
			Sniff: anyLine(`^\s*(->\s+)?(Seq Scan|Index Scan|Index Only Scan|Bitmap Heap Scan|Hash Join|Nested Loop|Merge Join|Sort|Aggregate|HashAggregate|Limit|Gather)\b.*\(cost=[\d.]+\.\.[\d.]+ rows=`)},
		Spec{Type: Psql, Cmds: []string{"psql", "pgcli"}, Priority: 59, Sniff: all(anyLine(`^\(\d+ rows?\)$`), anyLine(`^-+(\+-+)+$`))},
		Spec{Type: MySQL, Cmds: []string{"mysql", "mariadb"}, Priority: 59, Sniff: all(anyLine(`^\d+ rows? in set \(|^Empty set \(`), anyLine(`^\+-+(\+-+)*\+$`))},
		Spec{Type: SQLite, Cmds: []string{"sqlite3", "duckdb", "litecli"}},
		Spec{Type: Redis, Cmds: []string{"redis-cli", "valkey-cli"}, Priority: 62,
			Sniff: func(t string, l []string) bool { return redisList(t, l) || redisInfo(t, l) }},
		Spec{Type: Mongosh, Cmds: []string{"mongosh", "mongo"}, Priority: 58, Sniff: anyLine(`ObjectId\(['"][0-9a-f]{24}['"]\)`)},
		Spec{Type: Migrate, Cmds: []string{"prisma migrate", "alembic", "migrate", "goose", "flyway", "dbmate", "atlas migrate"}, Priority: 58,
			Sniff: anyLine(`^INFO  \[alembic\.runtime\.migration\] Running|^Applying migration ` + "`" + `|^The following migration\(s\) have been applied|^\d+/u \S+ \(\d+(\.\d+)?m?s\)$|^OK   \d+_\S+\.sql`)},
	)
}

var (
	redisList = ratio(`^\d+\) `, 0.8)
	redisInfo = all(anyLine(`^# Server\s*$`), anyLine(`^redis_version:`))
)
