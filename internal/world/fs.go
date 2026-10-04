package world

import "os"

// Files is the save-directory operations on the data volume.
type Files interface {
	MkdirAll(path string, perm os.FileMode) error
	RemoveAll(path string) error
}

type diskFiles struct{}

func (diskFiles) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }

func (diskFiles) RemoveAll(path string) error { return os.RemoveAll(path) }
