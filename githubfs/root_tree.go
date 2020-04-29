package githubfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

const initialRootTreeQuery = `
{
	repository(owner: "%s", name: "%s") {
		defaultBranchRef {
			target {
				... on Commit {
					tree {
						entries {
							name
							type
							object {
								... on Blob {
									byteSize
								}
								oid
							}
						}
						oid
					}
				}
			}
		}
	}
}
`

const updateRootTreeQuery = `
{
	repository(owner: "%s", name: "%s") {
		defaultBranchRef {
			target {
				... on Commit {
					tree {
						oid
					}
				}
			}
		}
	}
}
`

type graphqlReq struct {
	Query     string   `json:"query"`
	Variables struct{} `json:"variables"`
}

type rootTreeResult struct {
	Data struct {
		Repository struct {
			DefaultBranchRef struct {
				Target struct {
					Tree struct {
						SHA     string `json:"oid"`
						Entries []struct {
							Name   string `json:"name"`
							Type   string `json:"type"`
							Object struct {
								SHA      string `json:"oid"`
								ByteSize int64  `json:"byteSize"`
							} `json:"object"`
						} `json:"entries"`
					} `json:"tree"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
	} `json:"data"`
}

func (fs *githubFS) queryGraphQL(query string, res interface{}) error {
	bodyR, bodyW := io.Pipe()
	graphql := &graphqlReq{
		Query: query,
	}
	go func() {
		json.NewEncoder(bodyW).Encode(graphql)
		bodyW.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/graphql", bodyR)
	if err != nil {
		return err
	}

	if _, err := fs.client.Do(context.Background(), req, &res); err != nil {
		return err
	}

	return nil
}

func (fs *githubFS) loadRepoRootTree() error {
	query := fmt.Sprintf(initialRootTreeQuery, fs.repoOwner, fs.repoName)
	res := rootTreeResult{}
	if err := fs.queryGraphQL(query, &res); err != nil {
		return err
	}

	fs.rootTreeMu.Lock()
	fs.rootTree.Store(res.Data.Repository.DefaultBranchRef.Target.Tree.SHA)
	fs.rootTreeMu.Unlock()

	entries := res.Data.Repository.DefaultBranchRef.Target.Tree.Entries
	treeFiles := make(map[string]githubFile, len(entries))
	for _, entry := range entries {
		treeFiles[entry.Name] = githubFile{
			fs:      fs,
			id:      entry.Object.SHA,
			objType: entry.Type,
			path:    entry.Name,
			size:    entry.Object.ByteSize,
		}
	}
	fs.treeCache.Store(fs.rootTree, treeFiles)

	atomic.StoreInt64(fs.lastCheck, time.Now().UnixNano())
	return nil
}

func (fs *githubFS) updateRepoRootTree() error {
	lastCheck := atomic.LoadInt64(fs.lastCheck)
	expired := time.Now().Add(time.Minute * -1).UnixNano()
	if lastCheck > expired {
		return nil
	}

	// store
	old := atomic.SwapInt64(fs.lastCheck, time.Now().UnixNano())
	if old > expired {
		return nil
	}

	query := fmt.Sprintf(updateRootTreeQuery, fs.repoOwner, fs.repoName)
	res := rootTreeResult{}
	if err := fs.queryGraphQL(query, &res); err != nil {
		return err
	}

	fs.rootTreeMu.Lock()
	fs.rootTree.Store(res.Data.Repository.DefaultBranchRef.Target.Tree.SHA)
	fs.rootTreeMu.Unlock()

	return nil
}
