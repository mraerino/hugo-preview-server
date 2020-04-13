package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/gohugoio/hugo/common/loggers"
	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/mraerino/hugo-preview-server/githubfs"
	"github.com/spf13/afero"
)

func main() {
	baseFs := afero.NewOsFs()
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	token := os.Getenv("HUGO_PREVIEW_GITHUB_TOKEN")
	repo := os.Getenv("HUGO_PREVIEW_GITHUB_REPO")
	if token != "" && repo != "" {
		ghFS, err := githubfs.New(
			os.Getenv("HUGO_PREVIEW_GITHUB_TOKEN"),
			os.Getenv("HUGO_PREVIEW_GITHUB_REPO"),
			"",
		)
		if err != nil {
			panic(err)
		}
		baseFs = ghFS
		cwd = "/" // virtual fs is at repo root
	}

	mm := afero.NewMemMapFs()
	cachedFs := afero.NewCacheOnReadFs(baseFs, mm, 0)
	fs := afero.NewCopyOnWriteFs(cachedFs, mm)

	cfg, _, err := hugolib.LoadConfig(hugolib.ConfigSourceDescriptor{
		Fs:         fs,
		Filename:   "config.yaml",
		WorkingDir: cwd,
	})
	if err != nil {
		panic(err)
	}

	cfg.Set("buildDrafts", true)
	cfg.Set("buildFuture", true)
	cfg.Set("buildExpired", true)
	cfg.Set("environment", "preview")

	// BasePathFs is required so public files are actually written
	//hugoFs := hugofs.NewFrom(afero.NewBasePathFs(fs, "/"), cfg)
	hugoFs := hugofs.NewFrom(fs, cfg)
	deps := deps.DepsCfg{
		Fs:      hugoFs,
		Cfg:     cfg,
		Logger:  loggers.NewDebugLogger(),
		Running: true,
	}

	site, err := hugolib.NewHugoSites(deps)
	if err != nil {
		panic(err)
	}

	if err := site.Build(hugolib.BuildCfg{}); err != nil {
		panic(err)
	}

	if err := afero.WriteFile(mm, filepath.Join(cwd, "content/about.md"), []byte(`
---
title: Bla
layout: page
---
blubb
		`), os.ModePerm); err != nil {
		panic(err)
	}

	if err := site.Build(hugolib.BuildCfg{}, fsnotify.Event{
		Name: filepath.Join(cwd, "content/about.md"),
		Op:   fsnotify.Write,
	}); err != nil {
		panic(err)
	}

	content, err := afero.ReadFile(mm, "public/about/index.html")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(content))
}
