// Package config loads and validates the profile-forge configuration file.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Section names accepted in the sections list, in their default order.
var DefaultSections = []string{
	"header", "path", "ecosystem", "flagships", "categories", "languages", "releases", "stack",
}

// Config is the full profile description.
type Config struct {
	User      string       `yaml:"user" json:"user"`
	Name      string       `yaml:"name" json:"name"`
	Headline  string       `yaml:"headline" json:"headline"`
	Bio       string       `yaml:"bio" json:"bio"`
	Links     []Link       `yaml:"links" json:"links,omitempty"`
	Badges    []Badge      `yaml:"badges" json:"badges,omitempty"`
	Stack     []StackGroup `yaml:"stack" json:"stack,omitempty"`
	Sections  []string     `yaml:"sections" json:"sections"`
	Intro     Intro        `yaml:"intro" json:"intro"`
	Footer    string       `yaml:"footer" json:"footer,omitempty"`
	Category  []Category   `yaml:"categories" json:"categories"`
	Flagships []Flagship   `yaml:"flagships" json:"flagships"`
	Ecosystem Ecosystem    `yaml:"ecosystem" json:"ecosystem"`
	Languages Languages    `yaml:"languages" json:"languages"`
	Path      PathChart    `yaml:"path" json:"path"`
	// Ignore lists public repositories that are intentionally left out of
	// every category (for example the profile repository itself).
	Ignore []string `yaml:"ignore" json:"ignore,omitempty"`
}

// Intro holds optional prose shown above individual sections.
type Intro struct {
	Path       string `yaml:"path" json:"path,omitempty"`
	Ecosystem  string `yaml:"ecosystem" json:"ecosystem,omitempty"`
	Flagships  string `yaml:"flagships" json:"flagships,omitempty"`
	Categories string `yaml:"categories" json:"categories,omitempty"`
	Languages  string `yaml:"languages" json:"languages,omitempty"`
	Releases   string `yaml:"releases" json:"releases,omitempty"`
	Stack      string `yaml:"stack" json:"stack,omitempty"`
}

// Link is a labeled URL shown in the header.
type Link struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
}

// Badge is a shields.io static badge.
type Badge struct {
	Label   string `yaml:"label" json:"label"`
	Message string `yaml:"message" json:"message"`
	Color   string `yaml:"color" json:"color"`
	Logo    string `yaml:"logo" json:"logo,omitempty"`
	Style   string `yaml:"style" json:"style,omitempty"`
}

// StackGroup is one titled row of static badges in the stack section, for example
// "Languages" or "Infrastructure". Items are badges whose label is usually empty.
type StackGroup struct {
	Title string  `yaml:"title" json:"title"`
	Items []Badge `yaml:"items" json:"items"`
}

// Category groups repositories and owns a color used in every diagram.
type Category struct {
	ID          string   `yaml:"id" json:"id"`
	Title       string   `yaml:"title" json:"title"`
	ColorName   string   `yaml:"color_name" json:"color_name,omitempty"`
	Color       string   `yaml:"color" json:"color"`
	Stroke      string   `yaml:"stroke" json:"stroke,omitempty"`
	Text        string   `yaml:"text" json:"text,omitempty"`
	Description string   `yaml:"description" json:"description"`
	Repos       []string `yaml:"repos" json:"repos"`
}

// Flagship marks a repository as a highlighted project.
type Flagship struct {
	Repo    string `yaml:"repo" json:"repo"`
	Summary string `yaml:"summary" json:"summary,omitempty"`
	Result  string `yaml:"result" json:"result"`
}

// Ecosystem describes the directed graph between repositories.
type Ecosystem struct {
	Direction string            `yaml:"direction" json:"direction,omitempty"`
	Aliases   map[string]string `yaml:"aliases" json:"aliases,omitempty"`
	Edges     []Edge            `yaml:"edges" json:"edges,omitempty"`
}

// Edge connects two repositories in the ecosystem diagram.
type Edge struct {
	From  string `yaml:"from" json:"from"`
	To    string `yaml:"to" json:"to"`
	Label string `yaml:"label" json:"label,omitempty"`
	Style string `yaml:"style" json:"style,omitempty"`
}

// Languages controls language aggregation.
type Languages struct {
	Top              int      `yaml:"top" json:"top"`
	ExcludeRepos     []string `yaml:"exclude_repos" json:"exclude_repos,omitempty"`
	ExcludeLanguages []string `yaml:"exclude_languages" json:"exclude_languages,omitempty"`
	IncludeArchived  bool     `yaml:"include_archived" json:"include_archived,omitempty"`
}

// PathChart controls the "find your path" flowchart.
type PathChart struct {
	Prompt    string `yaml:"prompt" json:"prompt,omitempty"`
	MaxOthers int    `yaml:"max_others" json:"max_others"`
	// Direction is the flowchart direction; LR stacks the categories in one column,
	// which stays readable when there are many of them.
	Direction string `yaml:"direction" json:"direction,omitempty"`
}

// Load reads, decodes, defaults, and statically validates a config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(data)
}

// Parse decodes a YAML document. Unknown keys are rejected so typos surface.
func Parse(data []byte) (*Config, error) {
	if hits := FindDashes(string(data)); len(hits) > 0 {
		return nil, &DashError{Hits: hits}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if len(c.Sections) == 0 {
		c.Sections = append([]string(nil), DefaultSections...)
	}
	if c.Languages.Top <= 0 {
		c.Languages.Top = 8
	}
	if c.Path.MaxOthers <= 0 {
		c.Path.MaxOthers = 3
	}
	if c.Path.Prompt == "" {
		c.Path.Prompt = "What do you want to explore?"
	}
	if c.Ecosystem.Direction == "" {
		c.Ecosystem.Direction = "LR"
	}
	if c.Path.Direction == "" {
		c.Path.Direction = "TD"
	}
	for i := range c.Category {
		cat := &c.Category[i]
		if cat.Stroke == "" {
			cat.Stroke = cat.Color
		}
		if cat.Text == "" {
			cat.Text = "#111827"
		}
		if cat.ColorName == "" {
			cat.ColorName = cat.Color
		}
	}
}

var (
	idPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	userPattern  = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
)

// Validate checks the config for internal consistency. It does not contact
// GitHub; repository existence is checked separately against fetched data.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !userPattern.MatchString(c.User) {
		add("user: %q is not a valid GitHub login", c.User)
	}
	if strings.TrimSpace(c.Name) == "" {
		add("name: must not be empty")
	}
	known := map[string]bool{}
	for _, s := range DefaultSections {
		known[s] = true
	}
	seenSection := map[string]bool{}
	for _, s := range c.Sections {
		if !known[s] {
			add("sections: unknown section %q (allowed: %s)", s, strings.Join(DefaultSections, ", "))
		}
		if seenSection[s] {
			add("sections: %q listed twice", s)
		}
		seenSection[s] = true
	}
	for i, b := range c.Badges {
		if b.Message == "" || b.Color == "" {
			add("badges[%d]: message and color are required", i)
		}
	}
	for i, g := range c.Stack {
		if g.Title == "" || len(g.Items) == 0 {
			add("stack[%d]: title and at least one item are required", i)
		}
		for j, b := range g.Items {
			if b.Message == "" || b.Color == "" {
				add("stack[%d].items[%d]: message and color are required", i, j)
			}
		}
	}
	for i, l := range c.Links {
		if l.Label == "" || !strings.HasPrefix(l.URL, "https://") {
			add("links[%d]: label is required and url must start with https://", i)
		}
	}

	seenCat := map[string]bool{}
	owner := map[string]string{}
	for i, cat := range c.Category {
		if !idPattern.MatchString(cat.ID) {
			add("categories[%d]: id %q must match %s", i, cat.ID, idPattern)
		}
		if seenCat[cat.ID] {
			add("categories[%d]: duplicate id %q", i, cat.ID)
		}
		seenCat[cat.ID] = true
		if cat.ID == "start" {
			add("categories[%d]: id %q is reserved", i, cat.ID)
		}
		if cat.Title == "" {
			add("categories[%d]: title is required", i)
		}
		for _, field := range []struct{ name, v string }{{"color", cat.Color}, {"stroke", cat.Stroke}, {"text", cat.Text}} {
			if !colorPattern.MatchString(field.v) {
				add("categories[%d] (%s): %s %q must be a #rrggbb hex color", i, cat.ID, field.name, field.v)
			}
		}
		for _, r := range cat.Repos {
			if !repoPattern.MatchString(r) {
				add("categories[%d] (%s): invalid repo name %q", i, cat.ID, r)
			}
			if prev, ok := owner[r]; ok {
				add("repo %q is listed in both %q and %q", r, prev, cat.ID)
			}
			owner[r] = cat.ID
		}
	}
	if seenSection["path"] && len(c.Category) == 0 {
		add("sections: path requires at least one category")
	}

	seenFlag := map[string]bool{}
	for i, f := range c.Flagships {
		if f.Repo == "" || f.Result == "" {
			add("flagships[%d]: repo and result are required", i)
		}
		if seenFlag[f.Repo] {
			add("flagships[%d]: duplicate repo %q", i, f.Repo)
		}
		seenFlag[f.Repo] = true
		if _, ok := owner[f.Repo]; f.Repo != "" && !ok && len(c.Category) > 0 {
			add("flagships[%d]: repo %q is not in any category", i, f.Repo)
		}
	}

	switch c.Ecosystem.Direction {
	case "LR", "RL", "TD", "TB", "BT":
	default:
		add("ecosystem.direction: %q must be one of LR, RL, TD, TB, BT", c.Ecosystem.Direction)
	}
	switch c.Path.Direction {
	case "LR", "RL", "TD", "TB", "BT":
	default:
		add("path.direction: %q must be one of LR, RL, TD, TB, BT", c.Path.Direction)
	}
	for i, e := range c.Ecosystem.Edges {
		if e.From == "" || e.To == "" {
			add("ecosystem.edges[%d]: from and to are required", i)
		}
		if e.From == e.To && e.From != "" {
			add("ecosystem.edges[%d]: self edge on %q", i, e.From)
		}
		switch e.Style {
		case "", "solid", "dotted", "thick":
		default:
			add("ecosystem.edges[%d]: style %q must be solid, dotted, or thick", i, e.Style)
		}
		if strings.ContainsAny(e.Label, `"|`) {
			add("ecosystem.edges[%d]: label must not contain quotes or pipes", i)
		}
	}
	return errors.Join(errs...)
}

// Repos returns every repository the config references, deduplicated, in a
// stable order: categories, flagships, ecosystem edges, language exclusions.
func (c *Config) Repos() []string {
	seen := map[string]bool{}
	var out []string
	add := func(r string) {
		if r != "" && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, cat := range c.Category {
		for _, r := range cat.Repos {
			add(r)
		}
	}
	for _, f := range c.Flagships {
		add(f.Repo)
	}
	for _, e := range c.Ecosystem.Edges {
		add(e.From)
		add(e.To)
	}
	for _, r := range c.Languages.ExcludeRepos {
		add(r)
	}
	return out
}

// Index maps repository names to their category and flagship entries, so
// renderers that look up every repository do it in constant time. Build it once
// per render; it does not follow later edits to the Config.
type Index struct {
	category map[string]*Category
	flagship map[string]*Flagship
}

// Index builds the lookup maps. The first category or flagship listing a repo
// wins, matching CategoryOf and FlagshipOf.
func (c *Config) Index() Index {
	ix := Index{category: map[string]*Category{}, flagship: map[string]*Flagship{}}
	for i := range c.Category {
		for _, r := range c.Category[i].Repos {
			if _, seen := ix.category[r]; !seen {
				ix.category[r] = &c.Category[i]
			}
		}
	}
	for i := range c.Flagships {
		if _, seen := ix.flagship[c.Flagships[i].Repo]; !seen {
			ix.flagship[c.Flagships[i].Repo] = &c.Flagships[i]
		}
	}
	return ix
}

// CategoryOf returns the category that owns repo, or nil.
func (ix Index) CategoryOf(repo string) *Category { return ix.category[repo] }

// FlagshipOf returns the flagship entry for repo, or nil.
func (ix Index) FlagshipOf(repo string) *Flagship { return ix.flagship[repo] }

// CategoryOf returns the category that owns repo, or nil.
func (c *Config) CategoryOf(repo string) *Category {
	for i := range c.Category {
		for _, r := range c.Category[i].Repos {
			if r == repo {
				return &c.Category[i]
			}
		}
	}
	return nil
}

// FlagshipOf returns the flagship entry for repo, or nil.
func (c *Config) FlagshipOf(repo string) *Flagship {
	for i := range c.Flagships {
		if c.Flagships[i].Repo == repo {
			return &c.Flagships[i]
		}
	}
	return nil
}
