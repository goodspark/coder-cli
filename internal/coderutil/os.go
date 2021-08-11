package coderutil

import (
	"io"
	"io/fs"
	"os"
)

// OSer wraps methods in package "os" and friends to allow for ease of testing
type OSer interface {
	// OpenFile does the same thing as os.OpenFile
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	// Executable does the same thing as os.Executable
	Executable() (string, error)
	// Stat does the same thing as os.Stat()
	Stat(path string) (fs.FileInfo, error)
	// RemoveAll does the same thing as os.RemoveAll
	RemoveAll(path string) error
	// Rename does the same thing as os.Rename
	Rename(src, dest string) error
	// TempDir does the same as os.Tempdir
	TempDir() string
}

type DefaultOS struct{}

var _ OSer = &DefaultOS{}

func NewDefaultOS() *DefaultOS {
	return &DefaultOS{}
}

func (d *DefaultOS) Executable() (string, error) {
	return os.Executable()
}

func (d *DefaultOS) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (d *DefaultOS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (d *DefaultOS) RemoveAll(path string) error {
	// TODO: use fs.RemoveAll when it becomes available (hopefully)
	return os.RemoveAll(path)
}

func (d *DefaultOS) Rename(src, dest string) error {
	// TODO: use fs.Rename when it becomes available (hopefully)
	return os.Rename(src, dest)
}

func (d *DefaultOS) TempDir() string {
	return os.TempDir()
}

// ReadWriteCloserAt is a ReadWriteCloser that also implements ReaderAt.
// Just like *os.File.
type ReadWriteCloserAt interface {
	io.ReadWriteCloser
	io.ReaderAt
}
