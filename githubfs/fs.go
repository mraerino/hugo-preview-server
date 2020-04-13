package githubfs

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/go-github/v31/github"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"golang.org/x/oauth2"
)

type githubFS struct {
	client *github.Client

	repoOwner string
	repoName  string
	branch    string

	rootTree  string
	treeCache sync.Map // map[string]map[string]githubFile
}

// New creates an FS to get files from github on-demand
func New(accessToken string, repo string, branch string) (afero.Fs, error) {
	repoParts := strings.Split(repo, "/")
	if len(repoParts) != 2 {
		return nil, errors.New("invalid repo path, expected owner/repo style")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	return &githubFS{
		client:    client,
		repoOwner: repoParts[0],
		repoName:  repoParts[1],
		branch:    branch,
	}, nil
}

func (fs *githubFS) getTree(basePath string, id string) (map[string]githubFile, error) {
	// lookup in cache
	cacheTree, ok := fs.treeCache.Load(id)
	if ok {
		tree, ok := cacheTree.(map[string]githubFile)
		if ok {
			return tree, nil
		}
		// invalid obj in cache, remove
		fmt.Printf("invalid cache object: %T\n", cacheTree)
		fs.treeCache.Delete(id)
	}

	// get from API
	ghTree, resp, err := fs.client.Git.GetTree(context.Background(), fs.repoOwner, fs.repoName, id, false)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errNotFound
		}
		if resp.StatusCode == http.StatusForbidden {
			fmt.Printf("rate limit?\n%+v\n", resp.Header)
		}
		return nil, err
	}

	tree := make(map[string]githubFile)
	for _, entry := range ghTree.Entries {
		fullPath := filepath.Join(basePath, entry.GetPath())
		tree[entry.GetPath()] = githubFile{
			fs:      fs,
			id:      entry.GetSHA(),
			objType: entry.GetType(),
			path:    fullPath,
			size:    int64(entry.GetSize()),
		}
	}
	fs.treeCache.Store(id, tree)

	return tree, nil
}

const errNotFound = syscall.ENOENT // afero tries this in the copy on write impl

func (fs *githubFS) Open(name string) (afero.File, error) {
	path := strings.TrimPrefix(name, "/")
	parts := strings.Split(path, "/")

	if fs.rootTree == "" {
		if err := fs.loadRepoRootTree(); err != nil {
			return nil, errors.Wrap(err, "Failed to load root tree")
		}
	}

	var treeCursor string = fs.rootTree
	var file *githubFile
	for i, part := range parts {
		if treeCursor == "" {
			// in a previous loop run, there was a file there was not a tree
			return nil, errNotFound
		}

		tree, err := fs.getTree(strings.Join(parts[:i], "/"), treeCursor)
		if err != nil {
			return nil, err
		}

		treeFile, ok := tree[part]
		if !ok {
			return nil, errNotFound
		}
		file = &treeFile

		if file.objType == "tree" {
			treeCursor = file.id
		}
	}

	if file == nil {
		return nil, errNotFound
	}
	return file, nil
}

func (fs *githubFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&(os.O_APPEND|os.O_CREATE|os.O_RDWR|os.O_TRUNC|os.O_WRONLY) != 0 {
		return nil, ErrReadOnly
	}

	return fs.Open(name)
}

func (fs *githubFS) Stat(name string) (os.FileInfo, error) {
	file, err := fs.Open(name)
	if err != nil {
		return nil, err
	}

	return file.Stat()
}

func (*githubFS) Name() string {
	return "GithubFS"
}

// write operation are not supported, those are below
var (
	ErrReadOnly = errors.New("Operation not supported: FS is read-only")
)

func (fs *githubFS) Create(name string) (afero.File, error) {
	return nil, ErrReadOnly
}

func (fs *githubFS) Mkdir(name string, perm os.FileMode) error {
	return ErrReadOnly
}

func (fs *githubFS) MkdirAll(path string, perm os.FileMode) error {
	return ErrReadOnly
}

func (fs *githubFS) Remove(name string) error {
	return ErrReadOnly
}

func (fs *githubFS) RemoveAll(path string) error {
	return ErrReadOnly
}

func (fs *githubFS) Rename(oldname, newname string) error {
	return ErrReadOnly
}

func (fs *githubFS) Chmod(name string, mode os.FileMode) error {
	return ErrReadOnly
}

func (fs *githubFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return ErrReadOnly
}
