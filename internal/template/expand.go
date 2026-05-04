// Package template implements ${VAR:-default} extraction and rendering for
// Kubernetes Job manifests. It replaces both the regex-based extractor and the
// shelled-out envsubst step from the legacy code path.
package template

import (
	"sort"
	"strings"
)

// Var is a single ${VAR:-default} occurrence found in a template.
type Var struct {
	Name    string
	Default string
}

// Extract walks the raw template text and returns every variable, deduped by
// name. The first occurrence wins for the default value, which matches legacy
// behaviour.
func Extract(text string) []Var {
	seen := map[string]string{}
	order := []string{}
	scan(text, func(name, def string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = def
		order = append(order, name)
	})
	sort.Strings(order)
	out := make([]Var, 0, len(order))
	for _, n := range order {
		out = append(out, Var{Name: n, Default: seen[n]})
	}
	return out
}

// Render substitutes ${VAR:-default} with values from env. When a variable is
// absent from env its default is used. Unknown shapes such as ${VAR} (no
// default) and $VAR are left untouched so YAML payloads using shell snippets
// in container args survive intact.
func Render(text string, env map[string]string) string {
	var b strings.Builder
	b.Grow(len(text))
	scanReplace(text, &b, env)
	return b.String()
}

// scan invokes fn for each ${NAME:-DEFAULT} occurrence. It tolerates `}`
// characters inside the default by tracking brace depth.
func scan(text string, fn func(name, def string)) {
	for i := 0; i < len(text); {
		j := strings.Index(text[i:], "${")
		if j < 0 {
			return
		}
		start := i + j
		name, def, end, ok := parseVar(text, start)
		if !ok {
			i = start + 2
			continue
		}
		fn(name, def)
		i = end
	}
}

// scanReplace mirrors scan but writes the substituted text to b.
func scanReplace(text string, b *strings.Builder, env map[string]string) {
	i := 0
	for i < len(text) {
		j := strings.Index(text[i:], "${")
		if j < 0 {
			b.WriteString(text[i:])
			return
		}
		b.WriteString(text[i : i+j])
		start := i + j
		name, def, end, ok := parseVar(text, start)
		if !ok {
			b.WriteString("${")
			i = start + 2
			continue
		}
		if v, present := env[name]; present {
			b.WriteString(v)
		} else {
			b.WriteString(def)
		}
		i = end
	}
}

// parseVar reads a ${NAME:-DEFAULT} starting at text[start:] (which must point
// at the literal "${"). It returns the captured name, default value, and the
// index just past the closing brace. ok is false when the syntax doesn't
// match — callers should leave the source text untouched in that case.
func parseVar(text string, start int) (name, def string, end int, ok bool) {
	if start+2 > len(text) || text[start] != '$' || text[start+1] != '{' {
		return "", "", 0, false
	}
	// Find the closing brace at depth zero. Default values may contain `}`
	// only when paired with a `{`; otherwise the first `}` ends the variable.
	depth := 1
	colonDash := -1
	for k := start + 2; k < len(text); k++ {
		c := text[k]
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if colonDash < 0 {
					return "", "", 0, false
				}
				name = text[start+2 : colonDash]
				def = text[colonDash+2 : k]
				if name == "" {
					return "", "", 0, false
				}
				return name, def, k + 1, true
			}
		case ':':
			if colonDash < 0 && k+1 < len(text) && text[k+1] == '-' {
				colonDash = k
			}
		}
	}
	return "", "", 0, false
}
