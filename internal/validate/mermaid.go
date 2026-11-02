package validate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MermaidBlock is one fenced mermaid diagram.
type MermaidBlock struct {
	StartLine int // 1-based line of the opening fence
	Lines     []string
}

// ExtractMermaid returns every ```mermaid block. It reports an error for an
// unclosed fence of any kind.
func ExtractMermaid(md string) ([]MermaidBlock, error) {
	var blocks []MermaidBlock
	var cur *MermaidBlock
	inFence := false
	openLine := 0
	for i, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if !fenceTag.MatchString(t) {
			if cur != nil {
				cur.Lines = append(cur.Lines, line)
			}
			continue
		}
		if !inFence {
			inFence, openLine = true, i+1
			if strings.TrimSpace(strings.TrimPrefix(t, "```")) == "mermaid" {
				cur = &MermaidBlock{StartLine: i + 1}
			}
			continue
		}
		if t != "```" {
			return nil, fmt.Errorf("line %d: unexpected fence %q inside a code block", i+1, t)
		}
		inFence = false
		if cur != nil {
			blocks = append(blocks, *cur)
			cur = nil
		}
	}
	if inFence {
		return nil, fmt.Errorf("line %d: code fence is never closed", openLine)
	}
	return blocks, nil
}

var (
	flowHeader = regexp.MustCompile(`^(flowchart|graph)\s+(TB|TD|BT|RL|LR)$`)
	pieHeader  = regexp.MustCompile(`^pie(\s+showData)?$`)
	pieSlice   = regexp.MustCompile(`^"[^"]+"\s*:\s*[0-9]+(\.[0-9]+)?$`)
	otherKinds = regexp.MustCompile(`^(sequenceDiagram|classDiagram|stateDiagram(-v2)?|erDiagram|gantt|journey|gitGraph|mindmap|timeline|quadrantChart)\b`)
	arrowSplit = regexp.MustCompile(`\s*(?:-->|-\.->|==>|---|-\.-|===)(?:\|[^|]*\|)?\s*`)
	leadingID  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*`)
	classDefRe = regexp.MustCompile(`^classDef\s+([A-Za-z][A-Za-z0-9_]*)\s+\S`)
	classUseRe = regexp.MustCompile(`^class\s+([A-Za-z0-9_,]+)\s+([A-Za-z][A-Za-z0-9_]*);?$`)
)

// Mermaid validates every mermaid block in md with basic syntax checks.
func Mermaid(md string) error {
	blocks, err := ExtractMermaid(md)
	if err != nil {
		return fmt.Errorf("mermaid: %w", err)
	}
	var errs []error
	for _, b := range blocks {
		if err := checkBlock(b); err != nil {
			errs = append(errs, fmt.Errorf("mermaid block at line %d: %w", b.StartLine, err))
		}
	}
	return errors.Join(errs...)
}

func checkBlock(b MermaidBlock) error {
	var body []string
	for _, l := range b.Lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		body = append(body, t)
	}
	if len(body) == 0 {
		return errors.New("empty diagram")
	}
	switch head := body[0]; {
	case flowHeader.MatchString(head):
		return checkFlowchart(body[1:])
	case pieHeader.MatchString(head):
		return checkPie(body[1:])
	case otherKinds.MatchString(head):
		return nil
	default:
		return fmt.Errorf("unknown diagram header %q", head)
	}
}

func checkPie(lines []string) error {
	slices := 0
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "title "):
		case pieSlice.MatchString(l):
			slices++
		default:
			return fmt.Errorf("invalid pie line %q", l)
		}
	}
	if slices == 0 {
		return errors.New("pie chart has no slices")
	}
	return nil
}

func checkFlowchart(lines []string) error {
	var errs []error
	nodes := map[string]bool{}
	defs := map[string]bool{}
	type use struct {
		nodes []string
		class string
		line  string
	}
	var uses []use
	for _, l := range lines {
		if err := balanced(l); err != nil {
			errs = append(errs, fmt.Errorf("%w in %q", err, l))
			continue
		}
		keyword := strings.Fields(l)[0]
		switch {
		case keyword == "classDef":
			m := classDefRe.FindStringSubmatch(l)
			if m == nil {
				errs = append(errs, fmt.Errorf("malformed classDef %q", l))
				continue
			}
			defs[m[1]] = true
		case keyword == "class":
			m := classUseRe.FindStringSubmatch(l)
			if m == nil {
				errs = append(errs, fmt.Errorf("malformed class statement %q", l))
				continue
			}
			uses = append(uses, use{nodes: strings.Split(m[1], ","), class: m[2], line: l})
		case keyword == "subgraph", l == "end", keyword == "style", keyword == "linkStyle",
			keyword == "click", keyword == "direction":
		default:
			for _, seg := range arrowSplit.Split(l, -1) {
				id := leadingID.FindString(seg)
				if id == "" {
					errs = append(errs, fmt.Errorf("statement without a node id: %q", l))
					break
				}
				if id == "end" || id == "graph" || id == "flowchart" {
					errs = append(errs, fmt.Errorf("reserved word %q used as a node id", id))
				}
				nodes[id] = true
			}
		}
	}
	for _, u := range uses {
		if !defs[u.class] {
			errs = append(errs, fmt.Errorf("class %q is used but never defined", u.class))
		}
		for _, n := range u.nodes {
			if !nodes[n] {
				errs = append(errs, fmt.Errorf("class statement references unknown node %q", n))
			}
		}
	}
	return errors.Join(errs...)
}

// balanced checks quotes and brackets outside quoted text.
func balanced(l string) error {
	if strings.Count(l, `"`)%2 != 0 {
		return errors.New("unbalanced double quotes")
	}
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	var stack []rune
	inQuote := false
	for _, r := range l {
		switch {
		case r == '"':
			inQuote = !inQuote
		case inQuote:
		case r == '(' || r == '[' || r == '{':
			stack = append(stack, r)
		case r == ')' || r == ']' || r == '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return errors.New("unbalanced brackets")
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return errors.New("unclosed bracket")
	}
	return nil
}
