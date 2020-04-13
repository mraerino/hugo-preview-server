package githubfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const rootTreeQuery = `
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

func (fs *githubFS) loadRepoRootTree() error {
	bodyR, bodyW := io.Pipe()
	graphql := &graphqlReq{
		Query: fmt.Sprintf(rootTreeQuery, fs.repoOwner, fs.repoName),
	}
	go func() {
		json.NewEncoder(bodyW).Encode(graphql)
		bodyW.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/graphql", bodyR)
	if err != nil {
		return err
	}

	res := rootTreeResult{}
	if _, err := fs.client.Do(context.Background(), req, &res); err != nil {
		return err
	}

	fs.rootTree = res.Data.Repository.DefaultBranchRef.Target.Tree.SHA

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

	return nil
}
