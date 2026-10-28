//go:build !windows

package platform

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func inputFlags(t *testing.T, file *os.File) uintptr {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags uintptr
	if err := raw.Control(func(fd uintptr) { flags, _, _ = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	return flags
}

func TestProcessInputCloseInterruptsReadAndPreservesOriginal(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close() //nolint:errcheck
	before := inputFlags(t, reader)
	input, err := PrepareProcessInput(reader)
	if err != nil {
		t.Fatal(err)
	}
	reading := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(reading); _, err := input.Read(make([]byte, 1)); done <- err }()
	<-reading
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed input read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not interrupt read")
	}
	if after := inputFlags(t, reader); after != before {
		t.Fatalf("original flags changed: %d -> %d", before, after)
	}
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 1)
	if _, err := reader.Read(data); err != nil || string(data) != "x" {
		t.Fatalf("original input unusable: %q %v", data, err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
}
