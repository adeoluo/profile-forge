// Package render turns config plus repository metadata into Markdown and JSON.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"text/template"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
)

//go:embed templates/readme.md.tmpl
var templates embed.FS

// DefaultTemplate returns the built-in README template source.
func DefaultTemplate() string {
	b, _ := templates.ReadFile("templates/readme.md.tmpl")
	return string(b)
}

// BadgeView is one rendered shields.io static badge.
type BadgeView struct {
	Alt  string `json:"alt"`
	URL  string `json:"url"`
	Link string `json:"link,omitempty"`
}

// Markdown renders the badge as an image, optionally wrapped in a link.
func (b BadgeView) Markdown() string {
	img := fmt.Sprintf("![%s](%s)", b.Alt, b.URL)
	if b.Link != "" {
		return fmt.Sprintf("[%s](%s)", img, b.Link)
	}
	return img
}

// RepoView is one repository as presented in the README and JSON.
type RepoView struct {
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Description   string     `json:"description"`
	Language      string     `json:"language,omitempty"`
	Topics        []string   `json:"topics,omitempty"`
	Stars         int        `json:"stars"`
	PushedAt      string     `json:"pushed_at,omitempty"`
	Archived      bool       `json:"archived,omitempty"`
	Category      string     `json:"category"`
	Flagship      bool       `json:"flagship"`
	Summary       string     `json:"summary,omitempty"`
	Result        string     `json:"result,omitempty"`
	LatestRelease string     `json:"latest_release,omitempty"`
	ReleaseURL    string     `json:"release_url,omitempty"`
	ReleaseBadge  *BadgeView `json:"-"`
}

// CategoryView is one category with its resolved repositories.
type CategoryView struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	ColorName   string     `json:"color_name"`
	Color       string     `json:"color"`
	Stroke      string     `json:"stroke"`
	Description string     `json:"description"`
	Swatch      BadgeView  `json:"-"`
	Repos       []RepoView `json:"repos"`
}

// View is the data passed to the template and serialized as portfolio JSON.
type View struct {
	User           string          `json:"user"`
	Name           string          `json:"name"`
	Headline       string          `json:"headline"`
	Bio            string          `json:"bio"`
	Links          []config.Link   `json:"links,omitempty"`
	Badges         []BadgeView     `json:"-"`
	StatBadges     []BadgeView     `json:"-"`
	Intro          config.Intro    `json:"-"`
	Footer         string          `json:"-"`
	PathChart      string          `json:"-"`
	EcosystemChart string          `json:"-"`
	Flagships      []RepoView      `json:"flagships"`
	Categories     []CategoryView  `json:"categories"`
	Languages      LanguageSummary `json:"languages"`
	LanguagePie    string          `json:"-"`
	Releases       []RepoView      `json:"-"`
	PublicRepos    int             `json:"public_repos"`
}

// StaticBadge builds a shields.io static badge URL. Only static badges are
// used because dynamic endpoints are rate limited and render unreliably.
func StaticBadge(label, message, color, style, logo string) string {
	esc := func(s string) string {
		s = strings.NewReplacer("-", "--", "_", "__").Replace(s)
		return url.PathEscape(s)
	}
	color = strings.TrimPrefix(color, "#")
	var path string
	if label == "" {
		path = esc(message) + "-" + esc(color)
	} else {
		path = esc(label) + "-" + esc(message) + "-" + esc(color)
	}
	q := url.Values{}
	if style != "" {
		q.Set("style", style)
	}
	if logo != "" {
		q.Set("logo", logo)
		q.Set("logoColor", "white")
	}
	u := "https://img.shields.io/badge/" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Build assembles the view from config and fetched repositories. It returns
// an error when the config references a repository that was not fetched.
func Build(c *config.Config, repos []github.Repo) (*View, error) {
	byName := make(map[string]github.Repo, len(repos))
	for _, r := range repos {
		byName[r.Name] = r
	}
	var missing []string
	for _, name := range c.Repos() {
		if _, ok := byName[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config references %d repositories that are not public repos of %s: %s",
			len(missing), c.User, strings.Join(missing, ", "))
	}

	v := &View{
		User:     c.User,
		Name:     c.Name,
		Headline: c.Headline,
		Bio:      strings.TrimSpace(c.Bio),
		Links:    c.Links,
		Intro:    c.Intro,
		Footer:   strings.TrimSpace(c.Footer),
	}
	for _, b := range c.Badges {
		style := b.Style
		if style == "" {
			style = "for-the-badge"
		}
		alt := b.Message
		if b.Label != "" {
			alt = b.Label + ": " + b.Message
		}
		v.Badges = append(v.Badges, BadgeView{Alt: alt, URL: StaticBadge(b.Label, b.Message, b.Color, style, b.Logo)})
	}

	repoView := func(name string, cat *config.Category) RepoView {
		r := byName[name]
		rv := RepoView{
			Name:          r.Name,
			URL:           r.HTMLURL,
			Description:   config.ReplaceDashes(strings.TrimSpace(r.Description)),
			Language:      r.Language,
			Topics:        r.Topics,
			Stars:         r.Stars,
			Archived:      r.Archived,
			LatestRelease: r.LatestRelease,
			ReleaseURL:    r.ReleaseURL,
		}
		if rv.URL == "" {
			rv.URL = "https://github.com/" + c.User + "/" + r.Name
		}
		if !r.PushedAt.IsZero() {
			rv.PushedAt = r.PushedAt.UTC().Format("2006-01-02")
		}
		if cat != nil {
			rv.Category = cat.ID
		}
		if f := c.FlagshipOf(name); f != nil {
			rv.Flagship = true
			rv.Summary = strings.TrimSpace(f.Summary)
			rv.Result = strings.TrimSpace(f.Result)
		}
		if rv.LatestRelease != "" {
			color := "2ea44f"
			if cat != nil {
				color = cat.Stroke
			}
			link := rv.ReleaseURL
			if link == "" {
				link = rv.URL + "/releases/latest"
			}
			rv.ReleaseBadge = &BadgeView{
				Alt:  name + " " + rv.LatestRelease,
				URL:  StaticBadge("release", rv.LatestRelease, color, "flat-square", ""),
				Link: link,
			}
		}
		return rv
	}

	for _, cat := range c.Category {
		cv := CategoryView{
			ID:          cat.ID,
			Title:       cat.Title,
			ColorName:   cat.ColorName,
			Color:       cat.Color,
			Stroke:      cat.Stroke,
			Description: strings.TrimSpace(cat.Description),
			Swatch:      BadgeView{Alt: cat.ColorName, URL: StaticBadge("", cat.ColorName, cat.Color, "flat-square", "")},
		}
		for _, r := range cat.Repos {
			cv.Repos = append(cv.Repos, repoView(r, &cat))
		}
		v.Categories = append(v.Categories, cv)
	}
	for _, f := range c.Flagships {
		v.Flagships = append(v.Flagships, repoView(f.Repo, c.CategoryOf(f.Repo)))
	}

	// Releases cover every public repository with a release, not only the
	// categorized ones, ordered by name.
	var withRelease []string
	public := 0
	for _, r := range repos {
		if !r.Fork {
			public++
		}
		if r.LatestRelease != "" && !r.Fork {
			withRelease = append(withRelease, r.Name)
		}
	}
	sort.Strings(withRelease)
	for _, name := range withRelease {
		v.Releases = append(v.Releases, repoView(name, c.CategoryOf(name)))
	}
	v.PublicRepos = public

	v.Languages = AggregateLanguages(repos, c.Languages)
	v.LanguagePie = LanguagePie(v.Languages)
	v.PathChart = PathChart(c)
	v.EcosystemChart = EcosystemChart(c)

	stat := func(label, msg string) BadgeView {
		return BadgeView{Alt: label + ": " + msg, URL: StaticBadge(label, msg, "1f6feb", "flat-square", "")}
	}
	v.StatBadges = []BadgeView{
		stat("public repos", fmt.Sprint(public)),
		stat("flagships", fmt.Sprint(len(c.Flagships))),
		stat("releases", fmt.Sprint(len(v.Releases))),
	}
	if len(v.Languages.Rows) > 0 {
		top := v.Languages.Rows[0]
		v.StatBadges = append(v.StatBadges, stat("top language", fmt.Sprintf("%s %.0f%%", top.Name, top.Share)))
	}
	return v, nil
}

// Renderer executes the README template.
type Renderer struct {
	tmpl *template.Template
}

// NewRenderer parses a template source; pass DefaultTemplate() for the built-in.
func NewRenderer(src string) (*Renderer, error) {
	t, err := template.New("readme").Funcs(funcs).Option("missingkey=error").Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	for _, s := range config.DefaultSections {
		if t.Lookup(s) == nil {
			return nil, fmt.Errorf("template is missing section %q", s)
		}
	}
	return &Renderer{tmpl: t}, nil
}

// Markdown renders the configured sections in order, separated by rules.
func (r *Renderer) Markdown(v *View, sections []string) (string, error) {
	var parts []string
	for _, s := range sections {
		var buf bytes.Buffer
		if err := r.tmpl.ExecuteTemplate(&buf, s, v); err != nil {
			return "", fmt.Errorf("render section %s: %w", s, err)
		}
		if out := strings.TrimSpace(buf.String()); out != "" {
			parts = append(parts, out)
		}
	}
	var out strings.Builder
	out.WriteString("<!-- Generated by profile-forge. Edit the config file, not this file. -->\n\n")
	out.WriteString(strings.Join(parts, "\n\n---\n\n"))
	if v.Footer != "" {
		out.WriteString("\n\n---\n\n<div align=\"center\">\n\n<sub>" + v.Footer + "</sub>\n\n</div>")
	}
	out.WriteString("\n")
	return out.String(), nil
}

// JSON serializes the view as an indented portfolio document.
func JSON(v *View) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

var funcs = template.FuncMap{
	"cell": func(s string) string {
		s = strings.ReplaceAll(s, "\n", " ")
		return strings.ReplaceAll(s, "|", `\|`)
	},
	"join": strings.Join,
	"pct":  func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
	"human": func(n int64) string {
		switch {
		case n >= 1<<20:
			return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
		case n >= 1<<10:
			return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
		default:
			return fmt.Sprintf("%d B", n)
		}
	},
	"badges": func(bs []BadgeView) string {
		parts := make([]string, len(bs))
		for i, b := range bs {
			parts[i] = b.Markdown()
		}
		return strings.Join(parts, "\n")
	},
	"links": func(ls []config.Link) string {
		parts := make([]string, len(ls))
		for i, l := range ls {
			parts[i] = fmt.Sprintf("[%s](%s)", l.Label, l.URL)
		}
		return strings.Join(parts, " | ")
	},
	"orDefault": func(def, s string) string {
		if s == "" {
			return def
		}
		return s
	},
}
