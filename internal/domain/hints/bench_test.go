package hints

import (
	"testing"

	"github.com/corey/aoa/atlas"
	"github.com/corey/aoa/internal/domain/enricher"
	"github.com/stretchr/testify/require"
)

func BenchmarkGenerate(b *testing.B) {
	enr, err := enricher.NewFromFS(atlas.FS, "v1")
	require.NoError(b, err)
	g := New(enr)

	queries := []HintContext{
		{Query: "how does authentication work"},
		{Query: "zqxjkFrobnicatePlughWazzle"},
		{Query: "auth,login,session,verify", AndMode: true},
		{Query: "connect"},
		{Query: "foobar"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Generate(queries[i%len(queries)])
	}
}
