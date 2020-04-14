package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/fsnotify/fsnotify"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/mraerino/hugo-preview-server/githubfs"
	nutil "github.com/netlify/netlify-commons/util"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v2"
)

type previewAPI struct {
	hugo  *hugolib.HugoSites
	memFS afero.Fs

	initialBuildDone nutil.AtomicBool
}

func writeFiles(fs afero.Fs, files map[string]string) error {
	for name, content := range files {
		err := afero.WriteFile(fs, name, []byte(content), os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

func setupGithubFS() (afero.Fs, error) {
	token, ok := os.LookupEnv("HUGO_PREVIEW_GITHUB_TOKEN")
	if !ok || token == "" {
		return nil, errors.New("missing github token: HUGO_PREVIEW_GITHUB_TOKEN")
	}

	repo, ok := os.LookupEnv("HUGO_PREVIEW_GITHUB_REPO")
	if !ok || repo == "" {
		return nil, errors.New("missing github repo: HUGO_PREVIEW_GITHUB_REPO")
	}

	return githubfs.New(token, repo)
}

func newPreviewAPI() (*previewAPI, error) {
	ghFS, err := setupGithubFS()
	if err != nil {
		return nil, err
	}

	mm := afero.NewMemMapFs()
	cachedFs := afero.NewCacheOnReadFs(ghFS, mm, 0)
	fs := afero.NewCopyOnWriteFs(cachedFs, mm)

	basePath := os.Getenv("HUGO_PREVIEW_BASE")
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	fmt.Printf("basePath: %s\n", basePath)

	cfg, _, err := hugolib.LoadConfig(hugolib.ConfigSourceDescriptor{
		Fs:         fs,
		Filename:   filepath.Join(basePath, "config.yaml"),
		WorkingDir: basePath + "/",
	})
	if err != nil {
		return nil, err
	}

	cfg.Set("buildDrafts", true)
	cfg.Set("buildFuture", true)
	cfg.Set("buildExpired", true)
	cfg.Set("environment", "preview")

	hugoFs := hugofs.NewFrom(fs, cfg)
	deps := deps.DepsCfg{
		Fs:      hugoFs,
		Cfg:     cfg,
		Logger:  loggers.NewDebugLogger(),
		Running: true,
	}

	site, err := hugolib.NewHugoSites(deps)
	if err != nil {
		return nil, err
	}

	return &previewAPI{
		hugo:  site,
		memFS: mm,

		initialBuildDone: nutil.NewAtomicBool(false),
	}, nil
}

func (a *previewAPI) insertData(path string, data map[string]interface{}) (err error) {
	if !strings.HasSuffix(path, ".md") {
		return errors.New("Only md files supported right now")
	}

	var body string
	bodyIf, ok := data["body"]
	if ok {
		bodyStr, ok := bodyIf.(string)
		if ok {
			body = bodyStr
		}
	}
	delete(data, "body")

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	targetFile, err := a.memFS.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	defer func() {
		err = targetFile.Close()
	}()

	_, err = targetFile.WriteString("---\n")
	if err != nil {
		return
	}

	err = yaml.NewEncoder(targetFile).Encode(data)
	if err != nil {
		return
	}

	_, err = targetFile.WriteString("---\n")
	if err != nil {
		return
	}

	_, err = targetFile.WriteString(body)
	if err != nil {
		return
	}

	return
}

func (a *previewAPI) build(path string) error {
	partialBuild := a.initialBuildDone.Get()
	var events []fsnotify.Event
	if partialBuild {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		events = append(events, fsnotify.Event{
			Name: path,
			Op:   fsnotify.Write,
		})
	}

	err := a.hugo.Build(hugolib.BuildCfg{}, events...)
	if err != nil {
		return err
	}

	if !partialBuild {
		a.initialBuildDone.Set(true)
	}
	return nil
}

func (a *previewAPI) getPublicPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	page := a.hugo.GetContentPage(path)
	if page == nil {
		return ""
	}
	permalink := page.RelPermalink()
	if strings.HasSuffix(permalink, "/") {
		permalink += "index.html"
	}
	return permalink
}

type payload struct {
	Path   string `json:"path"`
	Layout string `json:"layout"`
	Type   string `json:"type"`

	Data map[string]interface{} `json:"data"`
}

func errResp(code int, msg string, err error) (*events.APIGatewayProxyResponse, error) {
	if err != nil {
		fmt.Printf("Err: %+v\n", err)
	}
	return &events.APIGatewayProxyResponse{
		StatusCode: code,
		Body:       msg,
	}, nil
}

func (a *previewAPI) handler(request events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	if request.HTTPMethod != http.MethodPost {
		return errResp(http.StatusBadRequest, "Only POST is allowed", nil)
	}

	pl := new(payload)
	if err := json.Unmarshal([]byte(request.Body), pl); err != nil {
		return errResp(http.StatusBadRequest, "Failed to read request body", err)
	}

	if err := a.insertData(pl.Path, pl.Data); err != nil {
		return errResp(http.StatusInternalServerError, "Failed to insert page data", err)
	}

	if err := a.build(pl.Path); err != nil {
		return errResp(http.StatusInternalServerError, "Failed to render site", err)
	}

	publicPath := a.getPublicPath(pl.Path)
	if publicPath == "" {
		return errResp(http.StatusNotFound, "Failed to find public path", nil)
	}

	content, err := afero.ReadFile(a.memFS, filepath.Join("public", publicPath))
	if err != nil {
		afero.Walk(a.memFS, "public", func(path string, file os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			fmt.Printf("file: %s\n", path)
			return nil
		})
		return errResp(http.StatusInternalServerError, "Failed to read content", err)
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type":                "text/html",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(content),
	}, nil
}

func main() {
	api, err := newPreviewAPI()
	if err != nil {
		panic(err)
	}
	lambda.Start(api.handler)
}
