package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adeoluo/profile-forge/internal/fakegh"
)

type harness struct {
	t      *testing.T
	dir    string
	srv    *fakegh.Server
	stdout bytes.Buffer
	stderr bytes.Buffer
	env    Env
}

// rewrite sends every request to target, so link checks hit a local server.
type rewrite struct{ target *url.URL }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), srv: fakegh.New("tester", fakegh.Sample())}
	t.Cleanup(h.srv.Close)
	h.write("profile.yaml", fakegh.SampleConfig)
	h.env = Env{
		Stdout:  &h.stdout,
		Stderr:  &h.stderr,
		Getenv:  func(string) string { return "" },
		GHToken: func() (string, error) { return "", errors.New("no gh") },
		HTTP:    &http.Client{Timeout: 5 * time.Second},
		Now:     time.Now,
	}
	return h
}

func (h *harness) path(name string) string { return filepath.Join(h.dir, name) }

func (h *harness) write(name, body string) {
	h.t.Helper()
	if err := os.WriteFile(h.path(name), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) read(name string) string {
	h.t.Helper()
	b, err := os.ReadFile(h.path(name))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

func (h *harness) run(args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	return Run(context.Background(), args, h.env)
}

func (h *harness) common() []string {
	return []string{"--config", h.path("profile.yaml"), "--cache-dir", h.path("cache"), "--api", h.srv.URL}
}

func TestGenerateCheckAndOffline(t *testing.T) {
	h := newHarness(t)
	args := append([]string{"generate"}, h.common()...)
	args = append(args, "--out", h.path("out/README.md"), "--json", h.path("out/portfolio.json"))
	if code := h.run(args...); code != 0 {
		t.Fatalf("generate exit %d: %s", code, h.stderr.String())
	}
	md := h.read("out/README.md")
	for _, want := range []string{"# Test Person", "```mermaid", "Beta retrieval engine, fast and small.", "| Go |"} {
		if !strings.Contains(md, want) {
			t.Errorf("README missing %q", want)
		}
	}
	if !strings.Contains(h.read("out/portfolio.json"), `"flagship": true`) {
		t.Error("portfolio JSON missing flagship")
	}
	if !strings.Contains(h.stderr.String(), "18 API requests, 0 cache hits") {
		t.Errorf("stderr: %s", h.stderr.String())
	}

	check := append([]string{"check"}, h.common()...)
	if code := h.run(append(check, "--readme", h.path("out/README.md"))...); code != 0 {
		t.Fatalf("check exit %d: %s", code, h.stderr.String())
	}
	if !strings.Contains(h.stderr.String(), "0 API requests, 18 cache hits") || !strings.HasPrefix(h.stdout.String(), "ok:") {
		t.Errorf("check output: %s / %s", h.stdout.String(), h.stderr.String())
	}

	h.write("stale.md", md+"edited\n")
	if code := h.run(append(check, "--readme", h.path("stale.md"))...); code != 1 || !strings.Contains(h.stderr.String(), "out of date") {
		t.Fatalf("stale check exit %d: %s", code, h.stderr.String())
	}
	if code := h.run(append(check, "--readme", h.path("nope.md"))...); code != 1 {
		t.Fatalf("missing readme exit %d", code)
	}

	// The API goes away; --offline still produces identical output from cache.
	h.srv.Close()
	off := append([]string{"generate", "--offline", "--cache-ttl", "1ns", "--out", "-"}, h.common()...)
	if code := h.run(off...); code != 0 {
		t.Fatalf("offline exit %d: %s", code, h.stderr.String())
	}
	if h.stdout.String() != md {
		t.Error("offline output differs from online output")
	}
}

func TestGenerateFailures(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		name   string
		config string
		extra  []string
		want   string
	}{
		{"dash in config", strings.Replace(fakegh.SampleConfig, "Engineer", "Engineer \u2014 builder", 1), nil, "em dash"},
		{"unknown repo", strings.Replace(fakegh.SampleConfig, "[alpha, delta]", "[alpha, delta, ghost]", 1), nil, "ghost"},
		{"bad template path", fakegh.SampleConfig, []string{"--template", "/nonexistent/t.tmpl"}, "read template"},
		{"offline empty cache", fakegh.SampleConfig, []string{"--offline", "--cache-dir", "EMPTY"}, "offline mode"},
		{"bad flag", fakegh.SampleConfig, []string{"--nope"}, "flag provided but not defined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.write("profile.yaml", tc.config)
			args := append([]string{"generate", "--out", h.path("x.md")}, h.common()...)
			for _, e := range tc.extra {
				if e == "EMPTY" {
					e = h.path("empty-cache")
				}
				args = append(args, e)
			}
			if code := h.run(args...); code == 0 {
				t.Fatalf("expected failure")
			}
			if !strings.Contains(h.stderr.String(), tc.want) {
				t.Fatalf("stderr %q does not contain %q", h.stderr.String(), tc.want)
			}
		})
	}
}

func TestGenerateWithCustomTemplateAndWarnings(t *testing.T) {
	h := newHarness(t)
	h.write("profile.yaml", strings.Replace(fakegh.SampleConfig, "ignore: [stray]", "ignore: []", 1))
	tmpl := `{{define "header"}}# {{.Name}} has {{.PublicRepos}} repos{{end}}
{{define "path"}}{{end}}{{define "ecosystem"}}{{end}}{{define "flagships"}}{{end}}
{{define "categories"}}{{end}}{{define "languages"}}{{end}}{{define "releases"}}{{end}}`
	h.write("t.tmpl", tmpl)
	args := append([]string{"generate", "--out", "-", "--template", h.path("t.tmpl")}, h.common()...)
	if code := h.run(args...); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if !strings.Contains(h.stdout.String(), "# Test Person has 6 repos") {
		t.Errorf("stdout: %s", h.stdout.String())
	}
	if !strings.Contains(h.stderr.String(), "warning: public repo stray is not in any category") {
		t.Errorf("stderr: %s", h.stderr.String())
	}
	h.write("bad.tmpl", `{{define "header"}}{{end}}`)
	args = append([]string{"generate", "--out", "-", "--template", h.path("bad.tmpl")}, h.common()...)
	if code := h.run(args...); code != 1 || !strings.Contains(h.stderr.String(), "missing section") {
		t.Fatalf("bad template: %d %s", code, h.stderr.String())
	}
}

func TestCheckLinks(t *testing.T) {
	h := newHarness(t)
	var failPath string
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failPath != "" && strings.Contains(r.URL.Path, failPath) {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("<svg/>"))
	}))
	defer img.Close()
	u, _ := url.Parse(img.URL)
	h.env.HTTP = &http.Client{Transport: rewrite{u}}
	args := append([]string{"check", "--check-links"}, h.common()...)
	if code := h.run(args...); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if !strings.Contains(h.stderr.String(), "image URLs: all returned HTTP 200") {
		t.Errorf("stderr: %s", h.stderr.String())
	}
	failPath = "release"
	if code := h.run(args...); code != 1 || !strings.Contains(h.stderr.String(), "HTTP 404") {
		t.Fatalf("broken link: %d %s", code, h.stderr.String())
	}
}

func TestTokenIsUsedOnline(t *testing.T) {
	h := newHarness(t)
	h.env.Getenv = func(k string) string {
		if k == "GITHUB_TOKEN" {
			return "tok"
		}
		return ""
	}
	if code := h.run(append([]string{"generate", "--out", "-"}, h.common()...)...); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if got := h.srv.AuthHeader.Load(); got != "Bearer tok" {
		t.Fatalf("auth = %v", got)
	}
}

func TestInit(t *testing.T) {
	h := newHarness(t)
	out := h.path("new/profile.yaml")
	if code := h.run("init", "--out", out); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}
	if h.read("new/profile.yaml") != string(ExampleConfig()) {
		t.Fatal("init wrote unexpected content")
	}
	if code := h.run("init", "--out", out); code != 1 || !strings.Contains(h.stderr.String(), "already exists") {
		t.Fatalf("overwrite guard: %d %s", code, h.stderr.String())
	}
	if code := h.run("init", "--out", out, "--force"); code != 0 {
		t.Fatalf("force: %d", code)
	}
	if code := h.run("init", "--out", "-"); code != 0 || h.stdout.String() != string(ExampleConfig()) {
		t.Fatal("init to stdout")
	}
	if code := h.run("init", "--bogus"); code != 2 && code != 1 {
		t.Fatalf("bad flag: %d", code)
	}
}

func TestMiscCommands(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		args []string
		code int
		out  string
		err  string
	}{
		{nil, 2, "", "Usage:"},
		{[]string{"version"}, 0, "profile-forge dev", ""},
		{[]string{"help"}, 0, "Usage:", ""},
		{[]string{"frobnicate"}, 2, "", `unknown command "frobnicate"`},
		{[]string{"generate", "-h"}, 0, "", "-config"},
		{[]string{"check", "--config", "/nonexistent.yaml"}, 1, "", "read config"},
	}
	for _, tc := range cases {
		if code := h.run(tc.args...); code != tc.code {
			t.Errorf("%v: exit %d want %d (%s)", tc.args, code, tc.code, h.stderr.String())
		}
		if !strings.Contains(h.stdout.String(), tc.out) || !strings.Contains(h.stderr.String(), tc.err) {
			t.Errorf("%v: stdout %q stderr %q", tc.args, h.stdout.String(), h.stderr.String())
		}
	}
}

func TestWriteOutError(t *testing.T) {
	h := newHarness(t)
	h.write("file", "")
	if err := writeOut(h.path("file/sub/x.md"), []byte("x"), nil); err == nil {
		t.Fatal("expected error")
	}
	if err := writeOut(h.path("dir"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(h.path("d2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeOut(h.path("d2"), []byte("x"), nil); err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultEnv(t *testing.T) {
	env := DefaultEnv()
	if env.Stdout == nil || env.HTTP == nil || env.Getenv == nil {
		t.Fatal("DefaultEnv incomplete")
	}
}

func TestGenerateRenderedDiagrams(t *testing.T) {
	h := newHarness(t)
	var calls [][]string
	h.env.Mermaid = func(_ context.Context, argv []string) error {
		calls = append(calls, argv)
		out := argv[len(argv)-1]
		for i, a := range argv {
			if a == "-o" {
				out = argv[i+1]
			}
		}
		return os.WriteFile(out, []byte("<svg/>"), 0o644)
	}
	flags := []string{"--diagrams-dir", "assets", "--mermaid-config", "theme.json", "--mermaid-cmd", "mmdc --quiet"}
	args := append(append([]string{"generate"}, h.common()...), flags...)
	if code := h.run(append(args, "--out", h.path("out/README.md"))...); code != 0 {
		t.Fatalf("generate exit %d: %s", code, h.stderr.String())
	}
	md := h.read("out/README.md")
	if strings.Contains(md, "```mermaid") {
		t.Error("README still has mermaid blocks in image mode")
	}
	for _, want := range []string{`<img src="assets/path.svg"`, `<img src="assets/ecosystem.svg"`, `<img src="assets/languages.svg"`} {
		if !strings.Contains(md, want) {
			t.Errorf("README missing %s", want)
		}
	}
	if len(calls) != 3 {
		t.Fatalf("renderer called %d times, want 3", len(calls))
	}
	got := strings.Join(calls[0], " ")
	for _, want := range []string{"mmdc --quiet -q -i", "ecosystem.mmd", "-b white", "-c theme.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderer argv %q missing %q", got, want)
		}
	}
	if !strings.HasPrefix(h.read("out/assets/path.mmd"), `%%{init:`) || h.read("out/assets/languages.svg") != "<svg/>" {
		t.Error("chart sources or images not written next to the README")
	}

	check := append(append([]string{"check"}, h.common()...), flags...)
	check = append(check, "--readme", h.path("out/README.md"))
	if code := h.run(check...); code != 0 {
		t.Fatalf("check exit %d: %s", code, h.stderr.String())
	}
	h.write("out/assets/path.mmd", "flowchart LR\n  a --> b\n")
	if code := h.run(check...); code != 1 || !strings.Contains(h.stderr.String(), "chart sources out of date (path)") {
		t.Fatalf("stale chart check exit %d: %s", code, h.stderr.String())
	}
}

func TestRenderedDiagramFailures(t *testing.T) {
	h := newHarness(t)
	h.env.Mermaid = func(context.Context, []string) error { return errors.New("renderer missing") }
	args := append(append([]string{"generate"}, h.common()...), "--diagrams-dir", "assets")
	if code := h.run(append(args, "--out", "-")...); code != 1 || !strings.Contains(h.stderr.String(), "needs --out to be a file") {
		t.Fatalf("stdout mode exit %d: %s", code, h.stderr.String())
	}
	if code := h.run(append(args, "--out", h.path("r/README.md"))...); code != 1 || !strings.Contains(h.stderr.String(), "render ecosystem: renderer missing") {
		t.Fatalf("renderer failure exit %d: %s", code, h.stderr.String())
	}
	if err := runCommand(context.Background(), []string{"false"}); err == nil || !strings.Contains(err.Error(), "false") {
		t.Fatalf("runCommand error = %v", err)
	}
	if err := runCommand(context.Background(), []string{"true"}); err != nil {
		t.Fatalf("runCommand(true) = %v", err)
	}
}
