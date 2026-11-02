package render

import (
	"fmt"
	"testing"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
	"github.com/adeoluo/profile-forge/internal/validate"
)

// syntheticPortfolio builds n repositories spread over 7 categories.
func syntheticPortfolio(n int) (*config.Config, []github.Repo) {
	colors := []string{"#dbeafe", "#dcfce7", "#ede9fe", "#fef3c7", "#fae8ff", "#fee2e2", "#ccfbf1"}
	langs := []string{"Python", "Go", "Rust", "TypeScript", "Shell", "Makefile", "C", "Java", "HTML", "CSS"}
	c := &config.Config{User: "bench", Name: "Bench", Sections: config.DefaultSections}
	for i, col := range colors {
		c.Category = append(c.Category, config.Category{
			ID: fmt.Sprintf("c%d", i), Title: fmt.Sprintf("Category %d", i), ColorName: col,
			Color: col, Stroke: col, Text: "#111827", Description: "Synthetic category.",
		})
	}
	repos := make([]github.Repo, n)
	for i := range repos {
		name := fmt.Sprintf("repo-%03d", i)
		repos[i] = github.Repo{
			Name: name, Description: "A synthetic repository used for benchmarking.",
			HTMLURL: "https://github.com/bench/" + name, Language: langs[i%len(langs)],
			Languages: map[string]int64{langs[i%len(langs)]: int64(1000 + i), langs[(i+3)%len(langs)]: int64(10 * i)},
		}
		if i%4 == 0 {
			repos[i].LatestRelease = "v0.1.0"
		}
		cat := &c.Category[i%len(c.Category)]
		cat.Repos = append(cat.Repos, name)
		if i%10 == 0 {
			c.Flagships = append(c.Flagships, config.Flagship{Repo: name, Result: "Measured."})
		}
		if i > 0 && i%3 == 0 {
			c.Ecosystem.Edges = append(c.Ecosystem.Edges, config.Edge{From: repos[i-1].Name, To: name})
		}
	}
	c.Languages.Top, c.Path.MaxOthers, c.Ecosystem.Direction = 8, 3, "LR"
	return c, repos
}

func BenchmarkBuildAndRender(b *testing.B) {
	for _, n := range []int{40, 200} {
		b.Run(fmt.Sprintf("repos=%d", n), func(b *testing.B) {
			c, repos := syntheticPortfolio(n)
			r, err := NewRenderer(DefaultTemplate())
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				v, err := Build(c, repos)
				if err != nil {
					b.Fatal(err)
				}
				md, err := r.Markdown(v, c.Sections)
				if err != nil {
					b.Fatal(err)
				}
				if err := validate.Output(md); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAggregateLanguages(b *testing.B) {
	_, repos := syntheticPortfolio(200)
	b.ReportAllocs()
	for b.Loop() {
		AggregateLanguages(repos, config.Languages{Top: 8})
	}
}
