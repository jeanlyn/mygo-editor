//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRejectFIFOWithoutBlocking(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(p, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		if _, err := readFile(p); err == nil {
			done <- nil
			return
		}
		if _, _, err := diskStamp(p); err == nil {
			done <- nil
			return
		}
		_, err := saveFile(saveRequest{Path: p, Content: content{Text: "replace", Ending: "\n"}, Overwrite: true})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO operation blocked")
	}
	st, err := os.Lstat(p)
	if err != nil || st.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("FIFO replaced", err)
	}
}

func TestRejectDeviceFileOperations(t *testing.T) {
	const p = "/dev/null"
	if _, err := os.Stat(p); err != nil {
		t.Skip(err)
	}
	if _, err := readFile(p); err == nil {
		t.Fatal("opened device")
	}
	if _, _, err := diskStamp(p); err == nil {
		t.Fatal("hashed device")
	}
}
