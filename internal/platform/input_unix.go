//go:build !windows

package platform

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
)

// PrepareProcessInput borrows input through an interruptible owned descriptor.
// Unix stdin may arrive as a blocking pipe which os.Stdin.Close cannot wake.
// The duplicate is pollable by Go; closing it interrupts reads. Close restores
// the original descriptor's nonblocking flag and never closes the original.
func PrepareProcessInput(input io.Reader) (io.ReadCloser, error) {
	file, ok := input.(*os.File)
	if !ok {
		return io.NopCloser(input), nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeCharDevice) == 0 {
		return io.NopCloser(input), nil
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	var flags uintptr
	var fd int
	var nativeErr error
	err = raw.Control(func(original uintptr) {
		var errno syscall.Errno
		flags, _, errno = syscall.Syscall(syscall.SYS_FCNTL, original, syscall.F_GETFL, 0)
		if errno != 0 {
			nativeErr = errno
			return
		}
		fd, nativeErr = syscall.Dup(int(original))
	})
	if err != nil || nativeErr != nil {
		return nil, errors.Join(err, nativeErr)
	}
	syscall.CloseOnExec(fd)
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return &processInput{
		File: os.NewFile(uintptr(fd), file.Name()), original: raw,
		wasNonblocking: flags&syscall.O_NONBLOCK != 0,
	}, nil
}

type processInput struct {
	*os.File
	original       syscall.RawConn
	wasNonblocking bool
	once           sync.Once
	err            error
}

func (p *processInput) Close() error {
	p.once.Do(func() {
		closeErr := p.File.Close()
		var restoreErr error
		controlErr := p.original.Control(func(fd uintptr) { restoreErr = syscall.SetNonblock(int(fd), p.wasNonblocking) })
		p.err = errors.Join(closeErr, controlErr, restoreErr)
	})
	return p.err
}
