package coderutil

import (
	"os"
	"os/exec"
)

// OSer wraps methods in package "os" and friends to allow for ease of testing
type OSer interface {
	// Create does the same thing as os.Create
	Create(path string) (*os.File, error)
	// ExecCommand runs exec.Command(name, args...) and returns its CombinedOutput.
	ExecCommand(name string, args ...string) ([]byte, error)
	// Executable does the same thing as os.Executable
	Executable() (string, error)
	// Stat does the same thing as os.Stat
	Stat(path string) (os.FileInfo, error)
	// RemoveAll does the same thing as os.RemoveAll
	RemoveAll(path string) error
	// Rename does the same thing as os.Rename
	Rename(src, dest string) error
}

// OS implements OSer
type OS struct {
	CreateF      func(string) (*os.File, error)
	ExecCommandF func(string, ...string) *exec.Cmd
	ExecutableF  func() (string, error)
	StatF        func(string) (os.FileInfo, error)
	RemoveAllF   func(string) error
	RenameF      func(string, string) error
}

var _ OSer = &OS{}

func RealOS() OSer {
	return &OS{
		CreateF:      os.Create,
		ExecCommandF: exec.Command,
		ExecutableF:  os.Executable,
		StatF:        os.Stat,
		RemoveAllF:   os.RemoveAll,
		RenameF:      os.Rename,
	}
}

func (o *OS) Create(path string) (*os.File, error) {
	return o.CreateF(path)
}

func (o *OS) ExecCommand(name string, args ...string) ([]byte, error) {
	return o.ExecCommandF(name, args...).CombinedOutput()
}

func (o *OS) Executable() (string, error) {
	return o.ExecutableF()
}

func (o *OS) Stat(name string) (os.FileInfo, error) {
	return o.StatF(name)
}

func (o *OS) RemoveAll(path string) error {
	return o.RemoveAllF(path)
}

func (o *OS) Rename(oldpath, newpath string) error {
	return o.RenameF(oldpath, newpath)
}
