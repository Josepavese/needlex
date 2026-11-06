package platform

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DownloadDirectory owns files exclusively created by one download. Its
// caller closes it once and chooses whether to retain the files.
type DownloadDirectory interface {
	Path() string
	Save(name string, source io.Reader, limit int64) (path string, size int64, err error)
	Open(name string) (io.ReadCloser, error)
	Close(retain bool) error
}

type downloadDirectory struct {
	root  *os.Root
	names []string
}

func NewDownloadDirectory(parent, destination string) (DownloadDirectory, error) {
	var err error
	if destination != "" {
		destination, err = filepath.Abs(destination)
		if err == nil {
			err = os.Mkdir(destination, 0o700)
		}
	} else {
		parent, err = filepath.Abs(parent)
		if err == nil {
			err = os.MkdirAll(parent, 0o700)
		}
		if err == nil {
			destination, err = os.MkdirTemp(parent, "fetch-")
		}
	}
	if err != nil {
		return nil, fmt.Errorf("create new download directory: %w", err)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return nil, errors.Join(err, os.Remove(destination))
	}
	return &downloadDirectory{root: root}, nil
}

func (d *downloadDirectory) Path() string { return d.root.Name() }

func (d *downloadDirectory) Save(name string, source io.Reader, limit int64) (string, int64, error) {
	file, err := d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, err
	}
	d.names = append(d.names, name)
	size, copyErr := io.Copy(file, io.LimitReader(source, limit))
	if copyErr == nil && size == limit {
		var extra [1]byte
		_, copyErr = io.ReadFull(source, extra[:])
		switch {
		case copyErr == nil:
			copyErr = errors.New("download exceeds total byte limit")
		case errors.Is(copyErr, io.EOF):
			copyErr = nil
		}
	}
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return "", 0, err
	}
	return filepath.Join(d.Path(), name), size, nil
}

func (d *downloadDirectory) Open(name string) (io.ReadCloser, error) {
	return d.root.Open(name)
}

func (d *downloadDirectory) Close(retain bool) error {
	if retain {
		return d.root.Close()
	}
	var err error
	for _, name := range d.names {
		err = errors.Join(err, d.root.Remove(name))
	}
	return errors.Join(err, d.root.Close(), os.Remove(d.Path()))
}
