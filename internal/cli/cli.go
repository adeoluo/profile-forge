// Package cli implements the profile-forge command line.
package cli

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
	"github.com/adeoluo/profile-forge/internal/render"
	"github.com/adeoluo/profile-forge/internal/validate"
)

// Version is set at build time with -ldflags "-X ...cli.Version=v0.1.0".
var Version = "dev"

//go:embed example.yaml
var exampleConfig []byte

// ExampleConfig returns the config written by `profile-forge init`.
func ExampleConfig() []byte { return exampleConfig }

// Env carries process dependencies so commands are testable.
type Env struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Getenv  func(string) string
	GHToken func() (string, error)
	HTTP    *http.Client // used for link checks
	Now     func() time.Time
}

// DefaultEnv uses the real process environment.
func DefaultEnv() Env {
	return Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		GHToken: github.GHAuthToken,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		Now:     time.Now,
	}
}

const usage = `profile-forge generates a GitHub profile README from live repository metadata.

Usage:
  profile-forge generate --config profile.yaml --out README.md [--json portfolio.json]
  profile-forge check    --config profile.yaml [--readme README.md]
  profile-forge init     [--out profile.yaml] [--force]
  profile-forge version

Shared flags for generate and check:
  --offline           serve every API response from the cache, ignoring its TTL
  --cache-dir DIR     cache directory (default: user cache dir/profile-forge)
  --cache-ttl DUR     reuse cached responses younger than DUR (default 6h; 0 disables reuse)
  --check-links       request every image URL and fail unless each returns HTTP 200
  --template FILE     use a custom text/template instead of the built-in one
  --api URL           GitHub API base URL (default https://api.github.com)

Authentication: GITHUB_TOKEN or GH_TOKEN, else the output of "gh auth token", else anonymous.
`

// Run executes the command line and returns a process exit code.
func Run(ctx context.Context, args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "generate":
		err = runGenerate(ctx, args[1:], env)
	case "check":
		err = runCheck(ctx, args[1:], env)
	case "init":
		err = runInit(args[1:], env)
	case "version", "--version", "-v":
		fmt.Fprintln(env.Stdout, "profile-forge", Version)
	case "help", "--help", "-h":
		fmt.Fprint(env.Stdout, usage)
	default:
		fmt.Fprintf(env.Stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(env.Stderr, "error:", err)
		return 1
	}
	return 0
}

type common struct {
	config     string
	offline    bool
	cacheDir   string
	cacheTTL   time.Duration
	checkLinks bool
	template   string
	api        string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "profile.yaml", "path to the YAML config")
	fs.BoolVar(&c.offline, "offline", false, "serve API responses only from the cache")
	fs.StringVar(&c.cacheDir, "cache-dir", github.DefaultDir(), "cache directory")
	fs.DurationVar(&c.cacheTTL, "cache-ttl", 6*time.Hour, "cache time-to-live")
	fs.BoolVar(&c.checkLinks, "check-links", false, "verify every image URL returns HTTP 200")
	fs.StringVar(&c.template, "template", "", "custom template file")
	fs.StringVar(&c.api, "api", github.DefaultBaseURL, "GitHub API base URL")
}

type result struct {
	cfg      *config.Config
	view     *render.View
	markdown string
	repos    []github.Repo
	stats    github.Stats
	elapsed  time.Duration
}

// pipeline loads config, fetches metadata, renders, and validates.
func pipeline(ctx context.Context, c common, env Env) (*result, error) {
	start := env.Now()
	cfg, err := config.Load(c.config)
	if err != nil {
		return nil, err
	}
	src := render.DefaultTemplate()
	if c.template != "" {
		b, err := os.ReadFile(c.template)
		if err != nil {
			return nil, fmt.Errorf("read template: %w", err)
		}
		src = string(b)
	}
	r, err := render.NewRenderer(src)
	if err != nil {
		return nil, err
	}

	token := ""
	if !c.offline {
		token = github.ResolveToken(env.Getenv, env.GHToken)
	}
	client := github.NewClient(token, github.NewCache(c.cacheDir, c.cacheTTL))
	client.BaseURL = c.api
	client.Offline = c.offline
	repos, err := client.FetchAll(ctx, cfg.User)
	if err != nil {
		return nil, fmt.Errorf("fetch repositories: %w", err)
	}
	view, err := render.Build(cfg, repos)
	if err != nil {
		return nil, err
	}
	md, err := r.Markdown(view, cfg.Sections)
	if err != nil {
		return nil, err
	}
	if err := validate.Output(md); err != nil {
		return nil, err
	}
	if c.checkLinks {
		urls := validate.ImageURLs(md)
		if _, err := validate.CheckLinks(ctx, env.HTTP, urls, 8); err != nil {
			return nil, err
		}
		fmt.Fprintf(env.Stderr, "checked %d image URLs: all returned HTTP 200\n", len(urls))
	}
	for _, name := range validate.Coverage(cfg, repos) {
		fmt.Fprintf(env.Stderr, "warning: public repo %s is not in any category (add it or list it under ignore)\n", name)
	}
	return &result{
		cfg: cfg, view: view, markdown: md, repos: repos,
		stats: client.Stats(), elapsed: env.Now().Sub(start),
	}, nil
}

func summary(env Env, res *result) {
	fmt.Fprintf(env.Stderr, "%d repos fetched, %d categories, %d flagships; %d API requests, %d cache hits; %s\n",
		len(res.repos), len(res.cfg.Category), len(res.cfg.Flagships),
		res.stats.Requests, res.stats.CacheHits, res.elapsed.Round(time.Millisecond))
}

func runGenerate(ctx context.Context, args []string, env Env) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var c common
	c.register(fs)
	out := fs.String("out", "README.md", "output Markdown path (- for stdout)")
	jsonOut := fs.String("json", "", "optional portfolio JSON output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := pipeline(ctx, c, env)
	if err != nil {
		return err
	}
	if err := writeOut(*out, []byte(res.markdown), env.Stdout); err != nil {
		return err
	}
	if *jsonOut != "" {
		b, err := render.JSON(res.view)
		if err != nil {
			return err
		}
		if err := writeOut(*jsonOut, b, env.Stdout); err != nil {
			return err
		}
	}
	summary(env, res)
	if *out != "-" {
		fmt.Fprintf(env.Stderr, "wrote %s (%d bytes)\n", *out, len(res.markdown))
	}
	return nil
}

func runCheck(ctx context.Context, args []string, env Env) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var c common
	c.register(fs)
	readme := fs.String("readme", "", "fail if this file differs from what generate would write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := pipeline(ctx, c, env)
	if err != nil {
		return err
	}
	if *readme != "" {
		cur, err := os.ReadFile(*readme)
		if err != nil {
			return fmt.Errorf("read %s: %w", *readme, err)
		}
		if string(cur) != res.markdown {
			return fmt.Errorf("%s is out of date: run profile-forge generate --config %s --out %s", *readme, c.config, *readme)
		}
	}
	summary(env, res)
	fmt.Fprintln(env.Stdout, "ok: config valid, all referenced repos exist, output passes dash and mermaid checks")
	return nil
}

func runInit(args []string, env Env) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	out := fs.String("out", "profile.yaml", "where to write the example config (- for stdout)")
	force := fs.Bool("force", false, "overwrite an existing file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out != "-" && !*force {
		if _, err := os.Stat(*out); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", *out)
		}
	}
	if err := writeOut(*out, exampleConfig, env.Stdout); err != nil {
		return err
	}
	if *out != "-" {
		fmt.Fprintf(env.Stderr, "wrote %s; edit it, then run profile-forge generate --config %s\n", *out, *out)
	}
	return nil
}

func writeOut(path string, data []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Main is the process entry point used by cmd/profile-forge.
func Main() {
	os.Exit(Run(context.Background(), os.Args[1:], DefaultEnv()))
}
