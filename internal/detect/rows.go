package detect

// Row declares a tool in one place: how to recognize it and an example of
// each way, so the matrix test needs no separate entry. Tools of a family
// whose engine reads its rules from a table (diag, testreport, pkgnoise,
// trace) are added as rows.
type Row struct {
	Type     string   // family/dialect
	Cmds     []string // see Spec.Cmds
	Files    []string
	Sniff    string // pattern one sampled line must match; "" = not sniffable
	Priority int    // 0 = rowPriority
	Refines  []string

	Cmd    string // example command line; required with Cmds
	Sample string // example content; required with Sniff
}

// rowPriority sorts rows after the hand-written specs of a family but before
// the generic formats.
const rowPriority = 60

var rows []Row

// registerRows adds tools declared as rows; family files call it from init.
func registerRows(rs ...Row) {
	for _, r := range rs {
		rows = append(rows, r)
		s := Spec{Type: r.Type, Cmds: r.Cmds, Files: r.Files, Priority: r.Priority, Refines: r.Refines}
		if s.Priority == 0 {
			s.Priority = rowPriority
		}
		if r.Sniff != "" {
			s.Sniff = anyLine(r.Sniff)
		}
		register(s)
	}
}

// IsRow reports whether a dialect is declared as a row.
func IsRow(typ string) bool {
	for _, r := range rows {
		if r.Type == typ {
			return true
		}
	}
	return false
}
