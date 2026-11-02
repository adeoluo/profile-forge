package render

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/fakegh"
	"github.com/adeoluo/profile-forge/internal/github"
	"github.com/adeoluo/profile-forge/internal/validate"
)

var update = flag.Bool("update", false, "rewrite golden files")

func sampleRepos(t *testing.T) []github.Repo {
	t.Helper()
	srv := fakegh.New("tester", fakegh.Sample())
	defer srv.Close()
	c := github.NewClient("", nil)
	c.BaseURL = srv.URL
	repos, err := c.FetchAll(context.Background(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	return repos
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/render -update)", err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs from golden file; run go test ./internal/render -update and review the diff\n--- got ---\n%s", name, got)
	}
}

func TestGolden(t *testing.T) {
	repos := sampleRepos(t)
	cases := []struct {
		name   string
		mutate func(c *config.Config)
	}{
		{"full.md", func(*config.Config) {}},
		{"reordered.md", func(c *config.Config) { c.Sections = []string{"flagships", "header", "languages"} }},
		{"minimal.md", func(c *config.Config) {
			c.Flagships, c.Ecosystem.Edges, c.Badges, c.Links = nil, nil, nil, nil
			c.Intro, c.Footer, c.Headline, c.Bio = config.Intro{}, "", "", ""
			c.Path.MaxOthers = 1
		}},
	}
	r, err := NewRenderer(DefaultTemplate())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Parse([]byte(fakegh.SampleConfig))
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(cfg)
			v, err := Build(cfg, repos)
			if err != nil {
				t.Fatal(err)
			}
			md, err := r.Markdown(v, cfg.Sections)
			if err != nil {
				t.Fatal(err)
			}
			if err := validate.Output(md); err != nil {
				t.Fatalf("generated output fails validation: %v", err)
			}
			golden(t, tc.name, []byte(md))
		})
	}
}

func TestGoldenJSON(t *testing.T) {
	cfg, err := config.Parse([]byte(fakegh.SampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	v, err := Build(cfg, sampleRepos(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := JSON(v)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatal(err)
	}
	golden(t, "portfolio.json", b)
}

func TestBuildRejectsMissingRepos(t *testing.T) {
	cfg, err := config.Parse([]byte(fakegh.SampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Category[0].Repos = append(cfg.Category[0].Repos, "ghost", "hidden")
	_, err = Build(cfg, sampleRepos(t))
	if err == nil || !strings.Contains(err.Error(), "2 repositories") || !strings.Contains(err.Error(), "ghost, hidden") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildSanitizesDescriptions(t *testing.T) {
	cfg, err := config.Parse([]byte(fakegh.SampleConfig))
	if err != nil {
		t.Fatal(err)
	}
	v, err := Build(cfg, sampleRepos(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Categories {
		for _, r := range c.Repos {
			if r.Name == "beta" && r.Description != "Beta retrieval engine, fast and small." {
				t.Fatalf("beta description = %q", r.Description)
			}
		}
	}
}

func TestAggregateLanguages(t *testing.T) {
	repos := []github.Repo{
		{Name: "a", Languages: map[string]int64{"Go": 600, "Shell": 100}},
		{Name: "b", Languages: map[string]int64{"Go": 200, "Python": 100}},
		{Name: "c", Languages: map[string]int64{"C": 5000}, Fork: true},
		{Name: "d", Languages: map[string]int64{"Java": 1000}, Archived: true},
		{Name: "e", Languages: map[string]int64{"HTML": 0}},
	}
	type row struct {
		name  string
		bytes int64
		repos int
	}
	cases := []struct {
		name  string
		opts  config.Languages
		rows  []row
		total int64
		count int
	}{
		{"defaults skip forks and archived", config.Languages{},
			[]row{{"Go", 800, 2}, {"Python", 100, 1}, {"Shell", 100, 1}}, 1000, 2},
		{"include archived", config.Languages{IncludeArchived: true},
			[]row{{"Java", 1000, 1}, {"Go", 800, 2}, {"Python", 100, 1}, {"Shell", 100, 1}}, 2000, 3},
		{"top folds into Other", config.Languages{Top: 2},
			[]row{{"Go", 800, 2}, {"Other", 200, 0}}, 1000, 2},
		{"exclude repo and language", config.Languages{ExcludeRepos: []string{"b"}, ExcludeLanguages: []string{"Shell"}},
			[]row{{"Go", 600, 1}}, 600, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AggregateLanguages(repos, tc.opts)
			if got.TotalBytes != tc.total || got.RepoCount != tc.count {
				t.Fatalf("total=%d count=%d", got.TotalBytes, got.RepoCount)
			}
			if len(got.Rows) != len(tc.rows) {
				t.Fatalf("rows = %+v", got.Rows)
			}
			var share float64
			for i, w := range tc.rows {
				g := got.Rows[i]
				if g.Name != w.name || g.Bytes != w.bytes || g.Repos != w.repos {
					t.Errorf("row %d = %+v want %+v", i, g, w)
				}
				share += g.Share
			}
			if share < 99.999 || share > 100.001 {
				t.Errorf("shares sum to %f", share)
			}
		})
	}
	if empty := AggregateLanguages(nil, config.Languages{}); len(empty.Rows) != 0 || LanguagePie(empty) != "" {
		t.Error("empty input should produce no rows and no pie")
	}
}

func TestStaticBadge(t *testing.T) {
	cases := []struct {
		label, msg, color, style, logo, want string
	}{
		{"built with", "Go", "00ADD8", "", "", "https://img.shields.io/badge/built%20with-Go-00ADD8"},
		{"", "blue", "#dbeafe", "flat-square", "", "https://img.shields.io/badge/blue-dbeafe?style=flat-square"},
		{"pre-release", "v0.1.0_rc", "2ea44f", "", "", "https://img.shields.io/badge/pre--release-v0.1.0__rc-2ea44f"},
		{"share", "Python 91% | Go", "1f6feb", "", "go", "https://img.shields.io/badge/share-Python%2091%25%20%7C%20Go-1f6feb?logo=go&logoColor=white"},
	}
	for _, tc := range cases {
		if got := StaticBadge(tc.label, tc.msg, tc.color, tc.style, tc.logo); got != tc.want {
			t.Errorf("StaticBadge(%q,%q) = %s want %s", tc.label, tc.msg, got, tc.want)
		}
	}
}

func TestNewRendererErrors(t *testing.T) {
	if _, err := NewRenderer("{{define \"header\"}}{{.Nope"); err == nil {
		t.Error("expected parse error")
	}
	if _, err := NewRenderer(`{{define "header"}}x{{end}}`); err == nil || !strings.Contains(err.Error(), "missing section") {
		t.Errorf("got %v", err)
	}
	src := DefaultTemplate() + `{{define "broken"}}{{.Missing}}{{end}}`
	r, err := NewRenderer(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Markdown(&View{}, []string{"broken"}); err == nil {
		t.Error("expected execution error")
	}
}

func TestMermaidTextEscaping(t *testing.T) {
	if got := mermaidText("say \"hi\"\nnow"); got != "say #quot;hi#quot; now" {
		t.Fatal(got)
	}
	if got := nodeID("r", "my.repo-name_x"); got != "r_my_repo_name_x" {
		t.Fatal(got)
	}
}

func TestHumanBytes(t *testing.T) {
	h := funcs["human"].(func(int64) string)
	for in, want := range map[int64]string{12: "12 B", 2048: "2.0 KiB", 3 << 20: "3.0 MiB"} {
		if got := h(in); got != want {
			t.Errorf("human(%d) = %s", in, got)
		}
	}
}

func TestStackSection(t *testing.T) {
	c := &config.Config{Stack: []config.StackGroup{
		{Title: "Languages", Items: []config.Badge{{Message: "Go", Color: "00ADD8", Logo: "go"}, {Label: "py", Message: "3.12", Color: "3776AB"}}},
	}}
	v := &View{Stack: nil}
	for _, g := range c.Stack {
		row := StackView{Title: g.Title}
		for _, b := range g.Items {
			row.Badges = append(row.Badges, BadgeView{Alt: b.Message, URL: StaticBadge(b.Label, b.Message, b.Color, "flat-square", b.Logo)})
		}
		v.Stack = append(v.Stack, row)
	}
	custom := `{{define "header"}}H{{end}}{{define "path"}}{{end}}{{define "ecosystem"}}{{end}}` +
		`{{define "flagships"}}{{end}}{{define "categories"}}{{end}}{{define "languages"}}{{end}}{{define "releases"}}{{end}}`
	r, err := NewRenderer(custom)
	if err != nil {
		t.Fatalf("custom template without stack must inherit the built-in: %v", err)
	}
	out, err := r.Markdown(v, []string{"header", "stack"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Stack", "**Languages**", "https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white&style=flat-square", ") ![3.12]("} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	empty, err := r.Markdown(&View{}, []string{"header", "stack"})
	if err != nil || strings.Contains(empty, "Stack") {
		t.Fatalf("an empty stack renders nothing: %q %v", empty, err)
	}
	if _, err := NewRenderer(`{{define "stack"}}{{end}}`); err == nil {
		t.Fatal("non-inheritable sections are still required")
	}
}

func TestPathChartListsOneRepositoryPerLine(t *testing.T) {
	c := &config.Config{
		Path: config.PathChart{Prompt: "Explore", MaxOthers: 3},
		Category: []config.Category{{
			ID: "model", Title: "Models", Color: "#dbeafe",
			Repos: []string{"alpha", "beta", "gamma", "delta", "epsilon"},
		}},
		Flagships: []config.Flagship{{Repo: "alpha", Result: "ok"}},
	}
	got := PathChart(c)
	want := `n_model["<u>Models</u><br/><b>alpha</b><br/>beta<br/>gamma<br/>delta<br/>+1 more"]`
	if !strings.Contains(got, want) {
		t.Fatalf("path chart node:\n%s\nwant it to contain:\n%s", got, want)
	}
	if strings.Contains(got, ", ") {
		t.Fatalf("repository names must not be comma-joined:\n%s", got)
	}
	if !strings.HasPrefix(got, flowInit+"\nflowchart TD\n") {
		t.Fatalf("path chart must start with the wrapping directive:\n%s", got)
	}
}
