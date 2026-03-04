// Package hints generates contextual zero-result suggestions for search queries.
// Pure domain logic — no CLI, no I/O. Uses the enricher atlas for synonym resolution.
package hints

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/corey/aoa/internal/domain/enricher"
)

// HintContext provides the query context for hint generation.
type HintContext struct {
	Query   string
	AndMode bool
}

// Generator produces zero-result hints using the enricher atlas.
type Generator struct {
	enricher *enricher.Enricher
	fallback map[string][]string // keywords not in atlas
}

// New creates a Generator backed by the given enricher.
// If enr is nil, only structural hints (NL detection, camelCase, AND mode) are generated.
func New(enr *enricher.Enricher) *Generator {
	return &Generator{
		enricher: enr,
		fallback: map[string][]string{
			"err":    {"err", "error", "fail"},
			"fail":   {"fail", "error", "err"},
			"modify": {"modify", "update", "patch"},
			"new":    {"new", "create", "init"},
			"save":   {"save", "write", "store", "persist"},
		},
	}
}

// Generate produces hint strings for a zero-result query.
// Returns nil for empty queries. Each hint is a single display line.
func (g *Generator) Generate(ctx HintContext) []string {
	if ctx.Query == "" {
		return nil
	}

	var hints []string

	// 1. Natural language detection — extract key symbol
	if containsSpaces(ctx.Query) {
		sym := extractKeySymbol(ctx.Query)
		if sym != "" {
			hints = append(hints,
				"  aOa searches by symbol name, not natural language.",
				fmt.Sprintf("  Try: grep %s", sym),
			)
		}
	}

	// 2. Overly specific camelCase — truncate to first segment
	if !containsSpaces(ctx.Query) && len(ctx.Query) > 15 && hasMixedCase(ctx.Query) {
		short := truncateCamelCase(ctx.Query)
		if short != ctx.Query {
			hints = append(hints,
				fmt.Sprintf("  Try broader term: grep %s", short),
			)
		}
	}

	// 3. Enricher-based alternation (exact key match, not substring)
	if !strings.Contains(ctx.Query, "|") {
		if alt := g.suggestAlternation(ctx.Query); alt != "" {
			hints = append(hints,
				fmt.Sprintf("  Try alternation: egrep '%s'", alt),
			)
		}
	}

	// 4. Too many AND terms — suggest first two
	if ctx.AndMode || strings.Contains(ctx.Query, ",") {
		parts := strings.Split(ctx.Query, ",")
		if len(parts) > 2 {
			hints = append(hints,
				fmt.Sprintf("  Too many AND terms (%d). Try fewer: grep -a '%s'",
					len(parts), strings.Join(parts[:2], ",")),
			)
		}
	}

	// 5. Locate fallback — always
	locateTarget := ctx.Query
	if containsSpaces(ctx.Query) {
		locateTarget = extractKeySymbol(ctx.Query)
	}
	if len(locateTarget) > 20 {
		locateTarget = locateTarget[:20]
	}
	if locateTarget != "" {
		hints = append(hints,
			fmt.Sprintf("  File search: aoa locate %s", strings.ToLower(locateTarget)),
		)
	}

	return hints
}

// suggestAlternation uses the enricher atlas for exact-match keyword lookup,
// then collects sibling keywords from the same term as alternation suggestions.
// Falls back to a small built-in map for common keywords not in the atlas.
func (g *Generator) suggestAlternation(pattern string) string {
	lower := strings.ToLower(pattern)

	// Try enricher first (exact key match — no substring)
	if g.enricher != nil {
		matches := g.enricher.Lookup(lower)
		if len(matches) > 0 {
			// Use the first match's domain+term to find siblings
			m := matches[0]
			terms := g.enricher.DomainTerms(m.Domain)
			if terms != nil {
				if siblings, ok := terms[m.Term]; ok && len(siblings) > 1 {
					// Limit to 4 siblings for readability
					alts := siblings
					if len(alts) > 4 {
						alts = alts[:4]
					}
					return strings.Join(alts, "|")
				}
			}
		}
	}

	// Fallback for common keywords not in atlas
	if alts, ok := g.fallback[lower]; ok {
		return strings.Join(alts, "|")
	}

	return ""
}

// containsSpaces returns true if the string has whitespace.
func containsSpaces(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// hasMixedCase returns true if string has both upper and lower case letters.
func hasMixedCase(s string) bool {
	hasUpper, hasLower := false, false
	for _, r := range s {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	return hasUpper && hasLower
}

// extractKeySymbol pulls the most likely symbol name from a natural language query.
func extractKeySymbol(query string) string {
	stopWords := map[string]bool{
		"how": true, "does": true, "the": true, "what": true, "where": true,
		"is": true, "are": true, "this": true, "that": true, "for": true,
		"with": true, "from": true, "work": true, "find": true, "get": true,
		"can": true, "and": true, "not": true, "all": true, "use": true,
		"why": true, "when": true, "which": true, "implement": true, "do": true,
	}
	words := strings.Fields(query)
	best := ""
	for _, w := range words {
		w = strings.Trim(w, "\"'`?.,!;:")
		if len(w) > len(best) && !stopWords[strings.ToLower(w)] && len(w) > 2 {
			best = w
		}
	}
	if best == "" && len(words) > 0 {
		best = strings.Trim(words[len(words)-1], "\"'`?.,!;:")
	}
	return best
}

// truncateCamelCase shortens a camelCase identifier to its first segment.
func truncateCamelCase(s string) string {
	var parts []string
	start := 0
	for i := 1; i < len(s); i++ {
		if unicode.IsUpper(rune(s[i])) && unicode.IsLower(rune(s[i-1])) {
			parts = append(parts, s[start:i])
			start = i
			if len(parts) >= 2 {
				break
			}
		}
	}
	if len(parts) == 0 {
		return s
	}
	return strings.ToLower(parts[0])
}
