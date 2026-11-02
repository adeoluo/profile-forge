package render

import (
	"sort"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
)

// LangShare is one row of the aggregated language breakdown.
type LangShare struct {
	Name  string  `json:"name"`
	Bytes int64   `json:"bytes"`
	Share float64 `json:"share"` // percent of all counted bytes
	Repos int     `json:"repos"` // repositories containing this language (0 for Other)
}

// LanguageSummary is the aggregate across all counted repositories.
type LanguageSummary struct {
	Rows       []LangShare `json:"rows"`
	TotalBytes int64       `json:"total_bytes"`
	RepoCount  int         `json:"repo_count"`
}

// AggregateLanguages sums bytes per language across repositories. Forks are
// always skipped; archived repositories are skipped unless opts says
// otherwise. Languages beyond opts.Top are folded into "Other".
func AggregateLanguages(repos []github.Repo, opts config.Languages) LanguageSummary {
	skipRepo := set(opts.ExcludeRepos)
	skipLang := set(opts.ExcludeLanguages)
	bytes := map[string]int64{}
	count := map[string]int{}
	var total int64
	counted := 0
	for _, r := range repos {
		if r.Fork || skipRepo[r.Name] || (r.Archived && !opts.IncludeArchived) {
			continue
		}
		used := false
		for lang, b := range r.Languages {
			if skipLang[lang] || b <= 0 {
				continue
			}
			bytes[lang] += b
			count[lang]++
			total += b
			used = true
		}
		if used {
			counted++
		}
	}
	rows := make([]LangShare, 0, len(bytes))
	for lang, b := range bytes {
		rows = append(rows, LangShare{Name: lang, Bytes: b, Repos: count[lang]})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Bytes != rows[j].Bytes {
			return rows[i].Bytes > rows[j].Bytes
		}
		return rows[i].Name < rows[j].Name
	})
	top := opts.Top
	if top <= 0 {
		top = 8
	}
	if len(rows) > top {
		other := LangShare{Name: "Other"}
		for _, r := range rows[top-1:] {
			other.Bytes += r.Bytes
		}
		rows = append(rows[:top-1:top-1], other)
	}
	for i := range rows {
		if total > 0 {
			rows[i].Share = 100 * float64(rows[i].Bytes) / float64(total)
		}
	}
	return LanguageSummary{Rows: rows, TotalBytes: total, RepoCount: counted}
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
