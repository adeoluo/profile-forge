package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Entities are built by concatenation so this file never contains them literally.
const (
	mdash = "&" + "mdash;"
	ndash = "&" + "ndash;"
	upper = "&" + "MDASH;"
	dec   = "&" + "#8211;"
	hex   = "&" + "#x2014;"
)

const minimal = `user: tester
name: Test Person
categories:
  - id: model
    title: Modeling
    color: "#dbeafe"
    repos: [alpha, beta]
flagships:
  - repo: alpha
    result: ok
`

func TestParseMinimalAppliesDefaults(t *testing.T) {
	c, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Sections, ","); got != strings.Join(DefaultSections, ",") {
		t.Errorf("sections = %s", got)
	}
	if c.Languages.Top != 8 || c.Path.MaxOthers != 3 || c.Ecosystem.Direction != "LR" || c.Path.Direction != "TD" {
		t.Errorf("defaults not applied: %+v %+v %q", c.Languages, c.Path, c.Ecosystem.Direction)
	}
	cat := c.Category[0]
	if cat.Stroke != "#dbeafe" || cat.Text != "#111827" || cat.ColorName != "#dbeafe" {
		t.Errorf("category defaults: %+v", cat)
	}
	if c.CategoryOf("beta").ID != "model" || c.CategoryOf("nope") != nil {
		t.Error("CategoryOf")
	}
	if c.FlagshipOf("alpha") == nil || c.FlagshipOf("beta") != nil {
		t.Error("FlagshipOf")
	}
	ix := c.Index()
	if ix.CategoryOf("beta") != c.CategoryOf("beta") || ix.CategoryOf("nope") != nil {
		t.Error("Index.CategoryOf")
	}
	if ix.FlagshipOf("alpha") != c.FlagshipOf("alpha") || ix.FlagshipOf("beta") != nil {
		t.Error("Index.FlagshipOf")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	base := minimal
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"bad user", strings.Replace(base, "user: tester", "user: -bad-", 1), "valid GitHub login"},
		{"empty name", strings.Replace(base, "name: Test Person", `name: ""`, 1), "name: must not be empty"},
		{"unknown key", base + "colour: red\n", "field colour not found"},
		{"unknown section", base + "sections: [header, footer]\n", `unknown section "footer"`},
		{"duplicate section", base + "sections: [header, header]\n", "listed twice"},
		{"bad color", strings.Replace(base, `"#dbeafe"`, `"blue"`, 1), "hex color"},
		{"bad id", strings.Replace(base, "id: model", "id: Model-1", 1), "must match"},
		{"reserved id", strings.Replace(base, "id: model", "id: start", 1), "reserved"},
		{"repo twice", strings.Replace(base, "[alpha, beta]", "[alpha, alpha]", 1), "listed in both"},
		{"bad repo name", strings.Replace(base, "[alpha, beta]", "[alpha, \"a b\"]", 1), "invalid repo name"},
		{"flagship without result", strings.Replace(base, "result: ok", `result: ""`, 1), "repo and result are required"},
		{"flagship not categorized", strings.Replace(base, "repo: alpha", "repo: zeta", 1), "not in any category"},
		{"bad direction", base + "ecosystem:\n  direction: XY\n", "direction"},
		{"bad path direction", base + "path:\n  direction: XY\n", "path.direction"},
		{"self edge", base + "ecosystem:\n  edges:\n    - {from: alpha, to: alpha}\n", "self edge"},
		{"bad edge style", base + "ecosystem:\n  edges:\n    - {from: alpha, to: beta, style: wavy}\n", "style"},
		{"pipe in label", base + "ecosystem:\n  edges:\n    - {from: alpha, to: beta, label: \"a|b\"}\n", "quotes or pipes"},
		{"badge missing color", base + "badges:\n  - {label: a, message: b}\n", "message and color"},
		{"stack without items", base + "stack:\n  - {title: Languages}\n", "at least one item"},
		{"stack item missing color", base + "stack:\n  - title: Languages\n    items:\n      - {message: Go}\n", "stack[0].items[0]: message and color"},
		{"http link", base + "links:\n  - {label: x, url: \"http://x\"}\n", "https://"},
		{"em dash", strings.Replace(base, "Test Person", "Test \u2014 Person", 1), "em dash (U+2014)"},
		{"en dash", strings.Replace(base, "Test Person", "2020\u20132024", 1), "en dash (U+2013)"},
		{"entity", strings.Replace(base, "Test Person", "a "+mdash+" b", 1), "dash entity " + mdash},
		{"not yaml", "user: [", "parse config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestDashErrorType(t *testing.T) {
	_, err := Parse([]byte("name: a\u2014b\n"))
	var de *DashError
	if !errors.As(err, &de) || len(de.Hits) != 1 || de.Hits[0].Line != 1 || de.Hits[0].Column != 8 {
		t.Fatalf("got %v", err)
	}
}

func TestFindDashes(t *testing.T) {
	cases := []struct {
		in   string
		want []DashHit
	}{
		{"plain - hyphen and -- double", nil},
		{"a\u2014b", []DashHit{{1, 2, "\u2014"}}},
		{"x\n\u00e9\u2013y", []DashHit{{2, 2, "\u2013"}}},
		{upper + " and " + dec + " and " + hex, []DashHit{{1, 1, upper}, {1, 13, dec}, {1, 25, hex}}},
		{"&mdash without semicolon", nil},
	}
	for _, tc := range cases {
		got := FindDashes(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("%q: got %v want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: hit %d = %+v want %+v", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestReplaceDashes(t *testing.T) {
	cases := map[string]string{
		"x " + mdash + " y":        "x, y",
		"mixed" + ndash + "entity": "mixed-entity",
		"fast \u2014 small":        "fast, small",
		"2020\u20132024":           "2020-2024",
		"a\u2014b":                 "a-b",
		"no dashes - at all":       "no dashes - at all",
		"one \u2013 two":           "one, two",
	}
	for in, want := range cases {
		if got := ReplaceDashes(in); got != want {
			t.Errorf("ReplaceDashes(%q) = %q want %q", in, got, want)
		}
		if hits := FindDashes(ReplaceDashes(in)); len(hits) != 0 {
			t.Errorf("dashes remain in %q", ReplaceDashes(in))
		}
	}
}

func TestReposOrderAndDedup(t *testing.T) {
	c, err := Parse([]byte(minimal + "ecosystem:\n  edges:\n    - {from: beta, to: gamma}\nlanguages:\n  exclude_repos: [delta, alpha]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Repos(), ","); got != "alpha,beta,gamma,delta" {
		t.Errorf("Repos() = %s", got)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(p, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("missing file: %v", err)
	}
}

func TestExamplesParse(t *testing.T) {
	for _, p := range []string{"../../examples/adeoluo.yaml", "../cli/example.yaml"} {
		if _, err := Load(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
