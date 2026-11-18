package lsoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// The regexp isPseudoFS replaced, kept to pin the old behavior.
var oldPseudoFS = lazyre.New(`^(tmpfs|devtmpfs|devfs|map|udev|dev|overlay|shm|efivarfs|squashfs|/dev/loop\d+|none|run|cgroup2?|proc|sysfs|portal)$`)

func TestPseudoFSSameAsRegexp(t *testing.T) {
	examples := []string{"tmpfs", "devtmpfs", "devfs", "map", "udev", "dev", "overlay", "shm", "efivarfs", "squashfs", "/dev/loop0", "/dev/loop12", "/dev/loop", "/dev/loopx", "/dev/loop1x",
		"none", "run", "cgroup", "cgroup2", "cgroup3", "proc", "sysfs", "portal", "/dev/sda1", "/dev/nvme0n1p2", "C:", "tmpfs2", "map auto.home", "/dev/loop٣"}
	sameAsOld(t, oldPseudoFS.MatchString, isPseudoFS, examples)
}
