package e2e

// How the binary picks a command and reports failure: exit codes and which stream speaks.
var _ = group("dispatch",
	Scenario{Name: "no arguments print the usage on stderr and exit 2", Steps: []Step{
		{Run: "gascoigne", Exit: 2, Stdout: []Check{Empty()}, Stderr: []Check{Has("USAGE", "gascoigne <command>")}},
	}},
	Scenario{Name: "an unknown command suggests the near match and exits 2", Steps: []Step{
		{Run: "gascoigne gian", Exit: 2, Stdout: []Check{Empty()},
			Stderr: []Check{Has(`unknown command "gian"`, `did you mean "gain"`)}},
	}},
	Scenario{Name: "a command error prints one gascoigne: line on stderr and exits 1", Steps: []Step{
		{Run: "gascoigne gain -csv", Exit: 1, Stdout: []Check{Empty()},
			Stderr: []Check{Has("gascoigne: gain: -csv needs -by or -failures")}},
	}},
	Scenario{Name: "-h, -help and --help show the overview", Steps: []Step{
		{Run: "gascoigne -h", Stdout: []Check{Has("USAGE", "exec -- <cmd>")}},
		{Run: "gascoigne -help", Stdout: []Check{Has("USAGE")}},
		{Run: "gascoigne --help", Stdout: []Check{Has("USAGE")}},
	}},
	Scenario{Name: "-v, -version and --version print the version like the version command", Steps: []Step{
		{Run: "gascoigne version", Stdout: []Check{Like(`^gascoigne \S+ \(\S+\)\n$`)}},
		{Run: "gascoigne -v", Stdout: []Check{Like(`^gascoigne \S+ \(\S+\)\n$`)}},
		{Run: "gascoigne -version", Stdout: []Check{Like(`^gascoigne \S+ \(\S+\)\n$`)}},
		{Run: "gascoigne --version", Stdout: []Check{Like(`^gascoigne \S+ \(\S+\)\n$`)}},
	}},
	Scenario{Name: "help <command> and <command> -h show that command's flags and exit 0", Steps: []Step{
		{Run: "gascoigne help exec", Stdout: []Check{Has("gascoigne exec [flags] -- <cmd>", "-raw")}},
		{Run: "gascoigne exec -h", Stdout: []Check{Has("gascoigne exec [flags] -- <cmd>")}},
	}},
)
