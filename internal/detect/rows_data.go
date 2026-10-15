package detect

// Structured formats declared as rows.
func init() {
	registerRows(
		Row{Type: Lock, Priority: 25, Files: []string{"yarn.lock", "go.sum", "gemfile.lock"},
			Sniff:  `^# yarn lockfile v1|^\S+ v\d\S* h1:\S+$|^GEM$`,
			Sample: "github.com/pkg/errors v0.9.1 h1:FEBLx1zS214owpjy7qsBeixbURkuhQAwrK5UwLGTwt4=\ngithub.com/pkg/errors v0.9.1/go.mod h1:bwawxfHBFNV+L2hUp1rHADufV3IMtnDRdf1r5NINEl0=\n"},
	)
}

// Columnar system tools declared as rows; the table engine reads any header.
func init() {
	registerRows(
		Row{Type: "table/sys", Cmds: []string{"lsblk", "lsmod", "w"}, Cmd: "lsblk -f",
			Sniff: `^NAME\s+MAJ:MIN\s+RM\s+SIZE|^Module\s+Size\s+Used by|^\s*USER\s+TTY\s+FROM\s+LOGIN@`, Sample: "NAME        MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS\nsda           8:0    0 465.8G  0 disk\n"},
	)
}
