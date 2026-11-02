package config

import (
	"fmt"
	"regexp"
	"strings"
)

// DashHit records one forbidden dash found in text.
type DashHit struct {
	Line   int    // 1-based line number
	Column int    // 1-based rune column
	Found  string // the offending character or entity
}

func (h DashHit) String() string {
	return fmt.Sprintf("line %d col %d: %s", h.Line, h.Column, describe(h.Found))
}

func describe(s string) string {
	switch s {
	case "\u2014":
		return "em dash (U+2014)"
	case "\u2013":
		return "en dash (U+2013)"
	default:
		return "dash entity " + s
	}
}

// DashError reports forbidden dashes in config text.
type DashError struct{ Hits []DashHit }

func (e *DashError) Error() string {
	parts := make([]string, 0, len(e.Hits))
	for _, h := range e.Hits {
		parts = append(parts, h.String())
	}
	return "config contains em or en dashes; use commas, colons, or hyphens instead: " + strings.Join(parts, "; ")
}

var dashEntity = regexp.MustCompile(`(?i)&(?:mdash|ndash|#8212|#8211|#x2014|#x2013);`)

// FindDashes returns every em dash, en dash, or equivalent HTML entity in s.
func FindDashes(s string) []DashHit {
	var hits []DashHit
	for i, line := range strings.Split(s, "\n") {
		col := 0
		for _, r := range line {
			col++
			if r == '\u2014' || r == '\u2013' {
				hits = append(hits, DashHit{Line: i + 1, Column: col, Found: string(r)})
			}
		}
		for _, loc := range dashEntity.FindAllStringIndex(line, -1) {
			hits = append(hits, DashHit{
				Line:   i + 1,
				Column: len([]rune(line[:loc[0]])) + 1,
				Found:  line[loc[0]:loc[1]],
			})
		}
	}
	return hits
}

// ReplaceDashes rewrites dashes in text that comes from outside the config
// (for example repository descriptions fetched from GitHub) so the generated
// output stays clean. Spaced dashes become commas; unspaced ones hyphens.
func ReplaceDashes(s string) string {
	if !strings.ContainsAny(s, "\u2014\u2013&") {
		return s // nothing to rewrite: skip the regexp and replacers
	}
	s = dashEntity.ReplaceAllString(s, "\u2014")
	s = spacedDashes.Replace(s)
	return bareDashes.Replace(s)
}

var (
	spacedDashes = strings.NewReplacer(" \u2014 ", ", ", " \u2013 ", ", ")
	bareDashes   = strings.NewReplacer("\u2014", "-", "\u2013", "-")
)
