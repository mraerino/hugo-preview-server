package githubfs

import (
	"os"
	"time"
)

type githubFileInfo struct {
	isDir bool
	name  string
	size  int64
}

func (fi *githubFileInfo) Name() string {
	return fi.name
}

func (fi *githubFileInfo) Size() int64 {
	return fi.size
}

func (githubFileInfo) Mode() os.FileMode {
	return os.ModePerm
}

func (githubFileInfo) ModTime() time.Time {
	return time.Now()
}

func (fi *githubFileInfo) IsDir() bool {
	return fi.isDir
}

func (githubFileInfo) Sys() interface{} {
	return nil
}
