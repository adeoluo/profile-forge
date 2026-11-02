package validate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
)

func block(body string) string { return "# t\n\n```mermaid\n" + body + "\n```\n" }

func TestMermaid(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string // empty means valid
	}{
		{"flowchart ok", block("flowchart LR\n  a[\"A\"] --> b\n  b -.->|uses| c((C))\n  classDef x fill:#fff;\n  class a,b x;"), ""},
		{"graph with comment and subgraph", block("graph TD\n  %% note\n  subgraph s\n  a --> b\n  end\n  style a fill:#f00"), ""},
		{"pie ok", block("pie showData\n  title T\n  \"Go\" : 10\n  \"Py\" : 2.5"), ""},
		{"other kind", block("sequenceDiagram\n  A->>B: hi"), ""},
		{"no mermaid at all", "# plain\n\n```go\nfmt.Println()\n```\n", ""},
		{"empty", block(""), "empty diagram"},
		{"unknown header", block("flowchart XY\n a --> b"), "unknown diagram header"},
		{"unbalanced quotes", block("flowchart LR\n a[\"A] --> b"), "unbalanced double quotes"},
		{"unbalanced brackets", block("flowchart LR\n a[A --> b"), "unclosed bracket"},
		{"mismatched brackets", block("flowchart LR\n a[A) --> b"), "unbalanced brackets"},
		{"undefined class", block("flowchart LR\n a --> b\n class a nope;"), `class "nope" is used but never defined`},
		{"unknown node in class", block("flowchart LR\n a --> b\n classDef x fill:#fff;\n class z x;"), `unknown node "z"`},
		{"malformed classDef", block("flowchart LR\n a --> b\n classDef"), "malformed classDef"},
		{"malformed classDef 2", block("flowchart LR\n a --> b\n classDef 9x fill"), "malformed classDef"},
		{"malformed class", block("flowchart LR\n a --> b\n class a"), "malformed class statement"},
		{"malformed class 2", block("flowchart LR\n a --> b\n class a b c d"), "malformed class statement"},
		{"reserved id", block("flowchart LR\n a --> end"), `reserved word "end"`},
		{"reserved graph", block("flowchart LR\n graph --> a"), `reserved word "graph"`},
		{"bad edge", block("flowchart LR\n a --> "), "statement without a node id"},
		{"pie no slices", block("pie\n  title T"), "no slices"},
		{"pie bad line", block("pie\n  Go : x"), "invalid pie line"},
		{"unclosed fence", "```mermaid\nflowchart LR\n a --> b\n", "never closed"},
		{"nested fence", "```mermaid\nflowchart LR\n```go\n```\n", "unexpected fence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Mermaid(tc.md)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

func TestExtractMermaidLines(t *testing.T) {
	md := "x\n```mermaid\nflowchart LR\n a --> b\n```\n\n```mermaid\npie\n \"a\" : 1\n```\n"
	blocks, err := ExtractMermaid(md)
	if err != nil || len(blocks) != 2 || blocks[0].StartLine != 2 || blocks[1].StartLine != 7 || len(blocks[0].Lines) != 2 {
		t.Fatalf("%+v %v", blocks, err)
	}
}

// Built by concatenation so this source file never contains the entity itself.
const ndash = "&" + "ndash;"

func TestOutputDashes(t *testing.T) {
	if err := Output("fine - text\n"); err != nil {
		t.Fatal(err)
	}
	err := Output("bad \u2014 text and " + ndash + "\n" + block("pie"))
	if err == nil || !strings.Contains(err.Error(), "em or en dashes") || !strings.Contains(err.Error(), ndash) || !strings.Contains(err.Error(), "no slices") {
		t.Fatalf("got %v", err)
	}
}

func TestImageURLs(t *testing.T) {
	md := `![a](https://img.shields.io/badge/a-b-c) text [![b](https://x.test/b.svg)](https://link)
<img alt="c" src="https://x.test/c.png" width="10"> ![a again](https://img.shields.io/badge/a-b-c)
![relative](docs/x.png) [not image](https://example.com)`
	got := strings.Join(ImageURLs(md), " ")
	want := "https://img.shields.io/badge/a-b-c https://x.test/b.svg https://x.test/c.png"
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestCheckLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("<svg/>"))
	}))
	defer srv.Close()
	ok := []string{srv.URL + "/a", srv.URL + "/b"}
	res, err := CheckLinks(context.Background(), srv.Client(), ok, 0)
	if err != nil || len(res) != 2 || res[0].Status != 200 {
		t.Fatalf("%+v %v", res, err)
	}
	bad := append(ok, srv.URL+"/missing", "http://127.0.0.1:1/refused", "://bad")
	_, err = CheckLinks(context.Background(), nil, bad, 4)
	if err == nil || !strings.Contains(err.Error(), "3 of 5") || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("got %v", err)
	}
}

func TestCoverage(t *testing.T) {
	cfg, err := config.Parse([]byte(`user: u
name: n
categories:
  - {id: a, title: A, color: "#ffffff", repos: [one]}
ecosystem:
  edges: [{from: one, to: two}]
ignore: [three]
`))
	if err != nil {
		t.Fatal(err)
	}
	repos := []github.Repo{{Name: "one"}, {Name: "two"}, {Name: "three"}, {Name: "four"}, {Name: "five", Fork: true}, {Name: "six", Archived: true}, {Name: "alpha"}}
	if got := strings.Join(Coverage(cfg, repos), ","); got != "alpha,four" {
		t.Fatalf("got %s", got)
	}
}
