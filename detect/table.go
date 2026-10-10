package detect

import (
	"strings"

	"github.com/lukaszb/gascoigne/lazyre"
)

// Columnar CLI output.
const (
	PS        = "table/ps"
	Sockets   = "table/ss" // ss, netstat
	Lsof      = "table/lsof"
	Top       = "table/top"
	Systemctl = "table/systemctl"
	DockerPS  = "table/docker" // docker/podman ps, images, volume/network ls
	Aligned   = "table/aligned"
	Boxed     = "table/boxed"
)

func init() {
	register(
		Spec{Type: PS, Cmds: []string{"ps"}, Priority: 60, Sniff: firstLine(`^\s*(USER|UID)\s+PID\s+|^\s*PID\s+(TTY|USER|PPID)\s+`)},
		Spec{Type: Sockets, Cmds: []string{"ss", "netstat"}, Priority: 60, Sniff: firstLine(`^(Netid\s+State|State\s+Recv-Q|Proto Recv-Q)`)},
		Spec{Type: Lsof, Cmds: []string{"lsof"}, Priority: 60, Sniff: firstLine(`^COMMAND\s+PID\s+(TID\s+TASKCMD\s+)?USER\s+FD\s+TYPE`)},
		Spec{Type: Top, Cmds: []string{"top", "htop"}, Priority: 60, Sniff: anyLine(`^top - \d\d:\d\d:\d\d up `)},
		Spec{Type: Systemctl, Cmds: []string{"systemctl list-units", "systemctl list-unit-files", "systemctl list-timers", "systemctl --failed"}, Priority: 60,
			Sniff: firstLine(`^\s*(UNIT\s+(LOAD|FILE)\s+|NEXT\s+LEFT\s+LAST)`)},
		Spec{Type: DockerPS, Cmds: []string{"docker ps", "docker images", "docker image ls", "docker container ls", "docker volume ls", "docker network ls",
			"podman ps", "podman images", "docker compose ps"}, Priority: 60,
			Sniff: firstLine(`^(CONTAINER ID\s+IMAGE|REPOSITORY\s+TAG\s+IMAGE ID|DRIVER\s+VOLUME NAME|NETWORK ID\s+NAME|NAME\s+IMAGE\s+COMMAND\s+SERVICE)`)},
		Spec{Type: Boxed, Priority: 61, Sniff: all(anyLine(`^\s*[+┌╔][-─═]{3,}`), ratio(`[|│]`, 0.6))},
		Spec{Type: Aligned, Priority: 85, Sniff: alignedTable},
	)
}

// alignedTable: an upper-case header followed by rows with the same number
// of 2+-space-separated columns.
func alignedTable(_ string, lines []string) bool {
	if len(lines) < 3 {
		return false
	}
	head := splitGap(lines[0])
	if len(head) < 3 {
		return false
	}
	for _, w := range head {
		if strings.ToUpper(w) != w {
			return false
		}
	}
	n, ok := 0, 0
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n++
		if len(splitGap(l)) == len(head) {
			ok++
		}
	}
	return n >= 2 && ok*10 >= n*8
}

var gap = lazyre.New(`\s{2,}`)

func splitGap(l string) []string { return gap.Split(strings.TrimSpace(l), -1) }
