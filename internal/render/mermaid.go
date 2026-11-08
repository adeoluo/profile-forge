package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/adeoluo/profile-forge/internal/config"
)

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9]+`)

// nodeID turns a repository or category name into a safe mermaid identifier.
// A prefix avoids collisions with mermaid keywords such as "end" or "graph".
func nodeID(prefix, name string) string {
	return prefix + "_" + strings.Trim(nonIdent.ReplaceAllString(name, "_"), "_")
}

// mermaidText escapes text for use inside a double-quoted mermaid label.
func mermaidText(s string) string {
	return mermaidEscaper.Replace(s)
}

var mermaidEscaper = strings.NewReplacer(`"`, "#quot;", "\n", " ")

func classDef(c config.Category) string {
	return fmt.Sprintf("  classDef %s fill:%s,stroke:%s,color:%s;", nodeID("c", c.ID), c.Color, c.Stroke, c.Text)
}

const rootClass = "  classDef c_root fill:#1f2937,stroke:#111827,color:#f9fafb;"

// flowInit widens Mermaid's label wrapping so hyphenated repository names stay on one
// line instead of breaking at a hyphen (the default wraps at 200px).
const flowInit = `%%{init: {"flowchart": {"wrappingWidth": 320}}}%%`

// bullet prefixes each repository in a path chart list.
const bullet = "\u2022 "

// PathChart builds the "find your path" flowchart. Each category is a box whose
// centered title is the box heading; inside it, the repositories are a
// left-aligned bulleted list with flagships in bold. GitHub's Mermaid sanitizer
// strips <u> and inline styles, so the title is set apart by being the subgraph
// heading and the list alignment comes from a classDef.
func PathChart(c *config.Config) string {
	var b strings.Builder
	b.WriteString(flowInit + "\n")
	grid := c.Path.Columns > 0
	dir := c.Path.Direction
	if dir == "" || grid {
		dir = "TD"
	}
	fmt.Fprintf(&b, "flowchart %s\n", dir)
	if !grid {
		fmt.Fprintf(&b, "  n_root([\"%s\"])\n", mermaidText(c.Path.Prompt))
	}
	ix := c.Index()
	blocks := make([]string, len(c.Category))
	for i, cat := range c.Category {
		var flags, others []string
		for _, r := range cat.Repos {
			if ix.FlagshipOf(r) != nil {
				flags = append(flags, r)
			} else {
				others = append(others, r)
			}
		}
		var lines []string
		for _, f := range flags {
			lines = append(lines, bullet+"<b>"+f+"</b>")
		}
		shown := others
		more := 0
		if len(shown) > c.Path.MaxOthers {
			more = len(shown) - c.Path.MaxOthers
			shown = shown[:c.Path.MaxOthers]
		}
		for _, r := range shown {
			lines = append(lines, bullet+r)
		}
		if more > 0 {
			lines = append(lines, fmt.Sprintf("+%d more", more))
		}
		var cb strings.Builder
		fmt.Fprintf(&cb, "  subgraph %s[\"%s\"]\n", nodeID("s", cat.ID), mermaidText(cat.Title))
		fmt.Fprintf(&cb, "    %s[\"%s\"]\n", nodeID("n", cat.ID), strings.Join(lines, "<br/>"))
		cb.WriteString("  end\n")
		if !grid {
			fmt.Fprintf(&cb, "  n_root --> %s\n", nodeID("s", cat.ID))
		}
		blocks[i] = cb.String()
	}
	if grid {
		// Mermaid lays out the boxes of one rank right to left in declaration order, so each
		// row is declared in reverse; invisible links stack row r+1 under row r.
		cols := c.Path.Columns
		for row := 0; row < len(blocks); row += cols {
			end := min(row+cols, len(blocks))
			for i := end - 1; i >= row; i-- {
				b.WriteString(blocks[i])
			}
		}
		for i := 0; i+cols < len(c.Category); i++ {
			fmt.Fprintf(&b, "  %s ~~~ %s\n", nodeID("s", c.Category[i].ID), nodeID("s", c.Category[i+cols].ID))
		}
	} else {
		for _, blk := range blocks {
			b.WriteString(blk)
		}
	}
	b.WriteString("\n")
	if !grid {
		b.WriteString(rootClass + "\n")
	}
	for _, cat := range c.Category {
		fmt.Fprintf(&b, "  classDef %s fill:%s,stroke-width:0px,color:%s,text-align:left;\n",
			nodeID("c", cat.ID), cat.Color, cat.Text)
	}
	b.WriteString("\n")
	if !grid {
		b.WriteString("  class n_root c_root;\n")
	}
	for _, cat := range c.Category {
		fmt.Fprintf(&b, "  class %s %s;\n", nodeID("n", cat.ID), nodeID("c", cat.ID))
		fmt.Fprintf(&b, "  style %s fill:%s,stroke:%s,color:%s\n", nodeID("s", cat.ID), cat.Color, cat.Stroke, cat.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// EcosystemChart builds the directed graph of configured edges. Nodes are
// colored by the category that owns the repository.
func EcosystemChart(c *config.Config) string {
	if len(c.Ecosystem.Edges) == 0 {
		return ""
	}
	var order []string
	seen := map[string]bool{}
	for _, e := range c.Ecosystem.Edges {
		for _, r := range []string{e.From, e.To} {
			if !seen[r] {
				seen[r] = true
				order = append(order, r)
			}
		}
	}
	var b strings.Builder
	b.WriteString(flowInit + "\n")
	fmt.Fprintf(&b, "flowchart %s\n", c.Ecosystem.Direction)
	for _, r := range order {
		label := r
		if a, ok := c.Ecosystem.Aliases[r]; ok && a != "" {
			label = a
		}
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", nodeID("r", r), mermaidText(label))
	}
	b.WriteString("\n")
	for _, e := range c.Ecosystem.Edges {
		arrow := "-->"
		switch e.Style {
		case "dotted":
			arrow = "-.->"
		case "thick":
			arrow = "==>"
		}
		if e.Label != "" {
			arrow += "|" + mermaidText(e.Label) + "|"
		}
		fmt.Fprintf(&b, "  %s %s %s\n", nodeID("r", e.From), arrow, nodeID("r", e.To))
	}
	ix := c.Index()
	members := map[string][]string{}
	for _, r := range order {
		if owner := ix.CategoryOf(r); owner != nil {
			members[owner.ID] = append(members[owner.ID], nodeID("r", r))
		}
	}
	var used []config.Category
	for _, cat := range c.Category {
		if len(members[cat.ID]) > 0 {
			used = append(used, cat)
		}
	}
	if len(used) > 0 {
		b.WriteString("\n")
		for _, cat := range used {
			b.WriteString(classDef(cat) + "\n")
		}
		b.WriteString("\n")
		for _, cat := range used {
			fmt.Fprintf(&b, "  class %s %s;\n", strings.Join(members[cat.ID], ","), nodeID("c", cat.ID))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// LanguagePie builds a mermaid pie chart of bytes per language.
func LanguagePie(s LanguageSummary) string {
	if len(s.Rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("pie\n  title Code by language (bytes)\n")
	for _, r := range s.Rows {
		fmt.Fprintf(&b, "  \"%s\" : %d\n", mermaidText(r.Name), r.Bytes)
	}
	return strings.TrimRight(b.String(), "\n")
}
