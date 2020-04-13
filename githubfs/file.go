package githubfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// githubFile is referencing a tree or blob
type githubFile struct {
	fs *githubFS

	id      string
	objType string
	path    string
	size    int64

	body *bytes.Reader
}

func (f *githubFile) Close() error {
	return nil
}

// ErrIsDir is returned whenever the caller tries to use a read operation on a directory
var ErrIsDir = errors.New("Cannot do read operations on non-blobs")

func (f *githubFile) initBody() error {
	if f.body != nil {
		return nil
	}

	if f.objType != "blob" {
		return ErrIsDir
	}

	blob, resp, err := f.fs.client.Git.GetBlobRaw(context.Background(), f.fs.repoOwner, f.fs.repoName, f.id)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return errNotFound
		}
		if resp.StatusCode == http.StatusForbidden {
			fmt.Printf("rate limit?\n%+v\n", resp.Header)
		}
		return err
	}

	f.body = bytes.NewReader(blob)
	return nil
}

func (f *githubFile) Read(p []byte) (n int, err error) {
	if err := f.initBody(); err != nil {
		return 0, err
	}

	return f.body.Read(p)
}

func (f *githubFile) ReadAt(p []byte, off int64) (n int, err error) {
	if err := f.initBody(); err != nil {
		return 0, err
	}

	return f.body.ReadAt(p, off)
}

func (f *githubFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.initBody(); err != nil {
		return 0, err
	}

	return f.body.Seek(offset, whence)
}

func (f *githubFile) Name() string {
	return f.path
}

func (f *githubFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.objType != "tree" {
		return nil, errors.New("Cannot list files, is not a directory")
	}

	tree, err := f.fs.getTree(f.path, f.id)
	if err != nil {
		return nil, err
	}

	infos := make([]os.FileInfo, len(tree))
	var i int
	for _, file := range tree {
		infos[i], _ = file.Stat() // current impl guarantees no error
		i++
	}
	return infos, nil
}

func (f *githubFile) Readdirnames(n int) ([]string, error) {
	if f.objType != "tree" {
		return nil, errors.New("Cannot list files, is not a directory")
	}

	tree, err := f.fs.getTree(f.path, f.id)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(tree))
	var i int
	for _, file := range tree {
		names[i] = filepath.Base(file.path)
		i++
	}
	return names, nil
}

func (f *githubFile) Stat() (os.FileInfo, error) {
	return &githubFileInfo{
		isDir: f.objType == "tree",
		name:  filepath.Base(f.path),
		size:  f.size,
	}, nil
}

func (f *githubFile) Sync() error {
	return nil
}

// not supported, because read-olny

func (f *githubFile) Write(p []byte) (n int, err error) {
	return 0, ErrReadOnly
}

func (f *githubFile) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, ErrReadOnly
}

func (f *githubFile) Truncate(size int64) error {
	return ErrReadOnly
}

func (f *githubFile) WriteString(s string) (ret int, err error) {
	return 0, ErrReadOnly
}
