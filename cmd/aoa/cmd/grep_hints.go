package cmd

import (
	"fmt"
	"strings"
	"unicode"
)

// buildZeroResultHint returns contextual suggestions based on the actual query.
// Analyzes the pattern for common mistakes and suggests specific fixes.
func buildZeroResultHint(pattern string) string {
	if pattern == "" {
		return ""
	}

	var hints []string

	// Detect natural language queries (contains spaces)
	if containsSpaces(pattern) {
		sym := extractKeySymbol(pattern)
		if sym != "" {
			hints = append(hints,
				"  aOa searches by symbol name, not natural language.",
				fmt.Sprintf("  Try: grep %s", sym),
			)
		}
	}

	// Detect overly specific camelCase — suggest truncating
	if !containsSpaces(pattern) && len(pattern) > 15 && hasMixedCase(pattern) {
		short := truncateCamelCase(pattern)
		if short != pattern {
			hints = append(hints,
				fmt.Sprintf("  Try broader term: grep %s", short),
			)
		}
	}

	// Suggest alternation from synonym map
	if !strings.Contains(pattern, "|") {
		if alt := suggestAlternation(pattern); alt != "" {
			hints = append(hints,
				fmt.Sprintf("  Try alternation: egrep '%s'", alt),
			)
		}
	}

	// If AND mode with many terms, suggest fewer
	if strings.Contains(pattern, ",") {
		parts := strings.Split(pattern, ",")
		if len(parts) > 2 {
			hints = append(hints,
				fmt.Sprintf("  Too many AND terms (%d). Try fewer: grep -a '%s'",
					len(parts), strings.Join(parts[:2], ",")),
			)
		}
	}

	// Always suggest locate as a fallback
	locateTarget := pattern
	if containsSpaces(pattern) {
		locateTarget = extractKeySymbol(pattern)
	}
	if len(locateTarget) > 20 {
		locateTarget = locateTarget[:20]
	}
	if locateTarget != "" {
		hints = append(hints,
			fmt.Sprintf("  File search: aoa locate %s", strings.ToLower(locateTarget)),
		)
	}

	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "\n") + "\n"
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

// suggestAlternation creates a simple OR suggestion from a single term.
func suggestAlternation(pattern string) string {
	synonyms := map[string][]string{
		"auth":      {"auth", "login", "session"},
		"config":    {"config", "settings", "options"},
		"error":     {"error", "err", "fail"},
		"handle":    {"handle", "handler", "process"},
		"test":      {"test", "spec", "assert"},
		"create":    {"create", "new", "init"},
		"delete":    {"delete", "remove", "destroy"},
		"update":    {"update", "modify", "patch"},
		"read":      {"read", "get", "fetch", "load"},
		"write":     {"write", "save", "store", "persist"},
		"parse":     {"parse", "decode", "unmarshal"},
		"format":    {"format", "encode", "marshal", "serialize"},
		"connect":   {"connect", "dial", "open"},
		"close":     {"close", "shutdown", "stop", "disconnect"},
		"validate":  {"validate", "check", "verify"},
		"middleware": {"middleware", "interceptor", "handler"},
		"route":     {"route", "endpoint", "handler", "path"},
		"cache":     {"cache", "store", "memo"},
		"log":       {"log", "logger", "logging"},
		"queue":     {"queue", "channel", "buffer", "stream"},
	}

	lower := strings.ToLower(pattern)
	for key, alts := range synonyms {
		if strings.Contains(lower, key) {
			return strings.Join(alts, "|")
		}
	}
	return ""
}
