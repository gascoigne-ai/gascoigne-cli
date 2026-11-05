//go:build unix

package pricing

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO (or a device such as /dev/zero) never ends: reading it would hang or fill memory.
func TestNonRegularFileDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "sheet.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no mkfifo here:", err)
	}
	done := make(chan Result, 1)
	go func() { done <- Load(Config{File: fifo, StateDir: dir}) }()
	select {
	case load := <-done:
		if len(load.Layers) != 1 || load.Layers[0].Error == "" {
			t.Fatalf("%+v", load)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reading a FIFO hung")
	}
}
