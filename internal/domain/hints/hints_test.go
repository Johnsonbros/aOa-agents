package hints

import (
	"strings"
	"testing"

	"github.com/corey/aoa/atlas"
	"github.com/corey/aoa/internal/domain/enricher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestGenerator(t *testing.T) *Generator {
	t.Helper()
	enr, err := enricher.NewFromFS(atlas.FS, "v1")
	require.NoError(t, err)
	return New(enr)
}

func TestGenerate_EmptyQuery(t *testing.T) {
	g := newTestGenerator(t)
	assert.Nil(t, g.Generate(HintContext{Query: ""}))
}

func TestNaturalLanguageHint(t *testing.T) {
	g := newTestGenerator(t)
	hints := g.Generate(HintContext{Query: "how does authentication work"})
	require.NotNil(t, hints)

	joined := strings.Join(hints, "\n")
	assert.Contains(t, joined, "symbol name, not natural language")
	assert.Contains(t, joined, "grep authentication")
}

func TestCamelCaseHint(t *testing.T) {
	g := newTestGenerator(t)
	hints := g.Generate(HintContext{Query: "zqxjkFrobnicatePlughWazzle"})
	require.NotNil(t, hints)

	joined := strings.Join(hints, "\n")
	assert.Contains(t, joined, "broader term")
	assert.Contains(t, joined, "grep zqxjk")
}

func TestAndModeHint(t *testing.T) {
	g := newTestGenerator(t)
	hints := g.Generate(HintContext{Query: "auth,login,session,verify", AndMode: true})
	require.NotNil(t, hints)

	joined := strings.Join(hints, "\n")
	assert.Contains(t, joined, "Too many AND terms")
	assert.Contains(t, joined, "auth,login")
}

func TestAlternationHint_NoSubstringMatch(t *testing.T) {
	// "login" must NOT match "log" family — exact key match only
	g := newTestGenerator(t)
	hints := g.Generate(HintContext{Query: "login"})

	joined := strings.Join(hints, "\n")
	// Must not suggest log|logger|logging (the old substring bug)
	assert.NotContains(t, joined, "log|logger|logging")
}

func TestAlternationHint_FallbackKeywords(t *testing.T) {
	g := newTestGenerator(t)

	// "err" is in the fallback map (not in atlas)
	hints := g.Generate(HintContext{Query: "err"})
	joined := strings.Join(hints, "\n")
	assert.Contains(t, joined, "alternation")
	assert.Contains(t, joined, "err")
	assert.Contains(t, joined, "error")
}

func TestLocateFallback_AlwaysPresent(t *testing.T) {
	g := newTestGenerator(t)
	hints := g.Generate(HintContext{Query: "foobar"})
	require.NotNil(t, hints)

	last := hints[len(hints)-1]
	assert.Contains(t, last, "aoa locate")
}

func TestGenerate_NilEnricher(t *testing.T) {
	g := New(nil)
	hints := g.Generate(HintContext{Query: "auth"})
	// Should still produce locate fallback, just no atlas-based alternation
	require.NotNil(t, hints)
	joined := strings.Join(hints, "\n")
	assert.Contains(t, joined, "aoa locate")
}
