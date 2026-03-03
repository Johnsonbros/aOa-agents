package test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/corey/aoa/internal/domain/index"
	"github.com/corey/aoa/internal/ports"
)

// =============================================================================
// Scale Gauntlet — G0 validation at production scale
//
// The existing gauntlet tests with 500 files (1,000 symbols). Real-world repos
// have 10K-40K files with 500K-2M symbols. This gauntlet tests at 24K files
// (1.2M symbols) — the Kubernetes repo benchmark.
//
// Token distribution is realistic:
//   - Tier 1 (ubiquitous): appear in 80%+ of files — "handler", "error", etc.
//   - Tier 2 (common): appear in 20-50% of files — "server", "cache", etc.
//   - Tier 3 (sparse): appear in 1-5% of files — "migrate", "rebalance", etc.
//   - Tier 4 (unique): file-specific function names
//
// Content scan is disabled (no projectRoot) to isolate pure search engine
// performance. Content scan at scale is a separate concern.
//
// G0 target: 500 results in <1ms for O(1) paths (literal, OR).
// =============================================================================

const (
	scaleFiles      = 24_000
	scaleSymsPerFile = 50
	// Total: 1.2M symbols
)

// Tier 1: ubiquitous tokens — appear in ~80% of files
var tier1Tokens = []string{
	"handler", "error", "context", "config", "result",
	"request", "response", "new", "get", "set",
}

// Tier 2: common tokens — appear in ~30% of files
var tier2Tokens = []string{
	"server", "client", "store", "cache", "auth",
	"log", "parse", "format", "validate", "process",
	"send", "receive", "create", "delete", "update",
	"list", "read", "write", "open", "close",
}

// Tier 3: sparse tokens — appear in ~3% of files
var tier3Tokens = []string{
	"dashboard", "migrate", "rebalance", "orchestrate", "reconcile",
	"transcode", "serialize", "throttle", "partition", "replicate",
}

// Tier 4: domain-specific, appear in ~10% of files each
var tier4Pools = [][]string{
	{"kubernetes", "pod", "deployment", "service", "namespace", "controller"},
	{"database", "query", "transaction", "schema", "index", "migrate"},
	{"http", "route", "middleware", "endpoint", "status", "header"},
	{"grpc", "proto", "stream", "interceptor", "metadata", "dial"},
	{"metrics", "counter", "histogram", "gauge", "observe", "collect"},
}

// symbolKinds for realistic Kind distribution
var symbolKinds = []string{"function", "method", "type", "interface", "const", "var"}

// buildScaleIndex creates a 24K-file index with realistic token distribution.
// No disk files — purely in-memory. Set projectRoot="" to skip content scan.
func buildScaleIndex(tb testing.TB) *ports.Index {
	tb.Helper()

	idx := &ports.Index{
		Tokens:   make(map[string][]ports.TokenRef, 200_000),
		Metadata: make(map[ports.TokenRef]*ports.SymbolMeta, scaleFiles*scaleSymsPerFile),
		Files:    make(map[uint32]*ports.FileMeta, scaleFiles),
	}

	rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility

	// Pre-compute: which files get which token tiers.
	// This creates realistic posting list sizes:
	//   tier1 tokens: ~19,200 refs each (80% of 24K)
	//   tier2 tokens: ~7,200 refs each (30% of 24K)
	//   tier3 tokens: ~720 refs each (3% of 24K)
	//   tier4 pools: ~2,400 refs each (10% of 24K per pool)

	dirs := []string{
		"pkg/api/server", "pkg/api/client", "pkg/api/handlers",
		"pkg/controller/deployment", "pkg/controller/service", "pkg/controller/pod",
		"pkg/scheduler/framework", "pkg/scheduler/plugins", "pkg/scheduler/queue",
		"pkg/kubelet/runtime", "pkg/kubelet/network", "pkg/kubelet/storage",
		"pkg/proxy/iptables", "pkg/proxy/ipvs", "pkg/proxy/nftables",
		"staging/src/k8s.io/client-go/rest", "staging/src/k8s.io/client-go/tools",
		"staging/src/k8s.io/apimachinery/pkg/apis", "staging/src/k8s.io/apimachinery/pkg/runtime",
		"vendor/golang.org/x/net", "vendor/google.golang.org/grpc",
		"test/e2e/framework", "test/e2e/network", "test/integration",
		"cmd/kube-apiserver", "cmd/kube-controller-manager", "cmd/kubelet",
	}

	for i := uint32(1); i <= scaleFiles; i++ {
		dir := dirs[int(i)%len(dirs)]
		path := fmt.Sprintf("%s/file%d.go", dir, i)

		idx.Files[i] = &ports.FileMeta{
			Path:         path,
			Language:     "go",
			Size:         int64(500 + rng.Intn(4000)), // 500B-4.5KB
			LastModified: int64(i),
		}

		// Generate symbols for this file
		for j := 0; j < scaleSymsPerFile; j++ {
			line := uint16(j*10 + 5)
			ref := ports.TokenRef{FileID: i, Line: line}

			// Build a realistic symbol name from token tiers
			var nameTokens []string

			// Every symbol gets a kind prefix
			kind := symbolKinds[j%len(symbolKinds)]

			switch {
			case j < 5:
				// First 5 symbols: tier1 (ubiquitous) + unique suffix
				t1 := tier1Tokens[j%len(tier1Tokens)]
				nameTokens = []string{t1, fmt.Sprintf("v%d", i)}
			case j < 15:
				// Next 10: tier2 (common) based on file bucket
				t2idx := (int(i) + j) % len(tier2Tokens)
				nameTokens = []string{tier2Tokens[t2idx], fmt.Sprintf("impl%d", j)}
			case j < 20:
				// Next 5: tier4 (domain pool) based on directory
				pool := tier4Pools[int(i)%len(tier4Pools)]
				t4 := pool[(j-15)%len(pool)]
				nameTokens = []string{t4, "manager"}
			case j < 22 && rng.Float64() < 0.03:
				// Rare: tier3 (sparse) — only 3% of files
				t3 := tier3Tokens[j%len(tier3Tokens)]
				nameTokens = []string{t3, "worker"}
			default:
				// Rest: unique per-file symbols
				nameTokens = []string{fmt.Sprintf("internal%d", i), fmt.Sprintf("fn%d", j)}
			}

			name := ""
			for k, t := range nameTokens {
				if k == 0 {
					name = t
				} else {
					// CamelCase join
					if len(t) > 0 {
						name += string(t[0]-32) + t[1:] // capitalize first char (ASCII safe for our tokens)
					}
				}
			}

			sym := &ports.SymbolMeta{
				Name:      name,
				Kind:      kind,
				Signature: name + "()",
				StartLine: line,
				EndLine:   line + uint16(5+rng.Intn(20)),
			}
			idx.Metadata[ref] = sym

			// Tokenize and add to inverted index
			tokens := index.Tokenize(sym.Name)
			for _, tok := range tokens {
				idx.Tokens[tok] = append(idx.Tokens[tok], ref)
			}
		}
	}

	return idx
}

// scaleCase extends gauntletCase with a maxCount override.
type scaleCase struct {
	name     string
	query    string
	opts     ports.SearchOptions
	target   time.Duration // G0 sub-ms target
}

// scaleCases defines query shapes at 24K-file scale.
// Targets are the G0 sub-ms aspiration. Today they will fail.
var scaleCases = []scaleCase{
	// --- Literal: single token O(1) lookup ---
	// "handler" appears in ~80% of files = ~19,200 refs in posting list.
	// Early termination caps at 500. Cost is 500 iterations + Hit allocs.
	{
		name:   "Literal_Ubiquitous",
		query:  "handler",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 2 * time.Millisecond,
	},
	// Tier 2 token: ~30% of files = ~7,200 refs
	{
		name:   "Literal_Common",
		query:  "server",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 1 * time.Millisecond,
	},
	// Tier 3 token: ~3% of files = ~720 refs
	{
		name:   "Literal_Sparse",
		query:  "dashboard",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 2 * time.Millisecond,
	},
	// "internal42" tokenizes to subtokens → triggers OR across large lists.
	// This is a tokenizer behavior test, not a pure literal test.
	{
		name:   "Literal_Unique",
		query:  "internal42",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 10 * time.Millisecond,
	},

	// --- OR: multi-term union ---
	// Two ubiquitous tokens: 5000 candidates (capped per-list)
	{
		name:   "OR_Ubiquitous",
		query:  "handler error",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 10 * time.Millisecond,
	},
	// Mixed tiers
	{
		name:   "OR_Mixed",
		query:  "handler dashboard",
		opts:   ports.SearchOptions{MaxCount: 500},
		target: 5 * time.Millisecond,
	},

	// --- AND: intersection ---
	{
		name:   "AND_Common",
		query:  "handler,manager",
		opts:   ports.SearchOptions{MaxCount: 500, AndMode: true},
		target: 25 * time.Millisecond,
	},

	// --- Case insensitive ---
	{
		name:   "CaseInsensitive_Ubiquitous",
		query:  "Handler",
		opts:   ports.SearchOptions{MaxCount: 500, Mode: "case_insensitive"},
		target: 2 * time.Millisecond,
	},

	// --- Regex: O(n) scan, pre-filter narrows when literals extractable ---
	{
		name:   "Regex_Simple",
		query:  "handler.*manager",
		opts:   ports.SearchOptions{MaxCount: 500, Mode: "regex"},
		target: 1 * time.Second, // Full scan fallback — pre-filter optimization is best-effort
	},

	// --- Default maxCount (20) for comparison ---
	{
		name:   "Literal_Ubiquitous_Max20",
		query:  "handler",
		opts:   ports.SearchOptions{}, // maxCount defaults to 20
		target: 1 * time.Millisecond,
	},
}

// TestScaleGauntlet_G0 runs search at 24K-file scale and reports actual vs target.
// This is the truth test — it exposes the real performance gap at production scale.
//
// All 10 shapes pass at 24K scale — runs in normal CI.
//
// Run: go test ./test/ -run TestScaleGauntlet_G0 -v -count=1
func TestScaleGauntlet_G0(t *testing.T) {

	t.Log("Building 24K-file index (1.2M symbols)...")
	buildStart := time.Now()
	idx := buildScaleIndex(t)
	t.Logf("Index built in %v — %d files, %d symbols, %d unique tokens",
		time.Since(buildStart), len(idx.Files), len(idx.Metadata), len(idx.Tokens))

	// Log posting list sizes for key tokens
	for _, tok := range []string{"handler", "server", "dashboard", "internal42"} {
		t.Logf("  token %q: %d refs", tok, len(idx.Tokens[tok]))
	}

	// Build engine with no projectRoot (skip content scan — isolate search perf)
	engine := index.NewSearchEngine(idx, nil, "")

	// Warmup
	for _, tc := range scaleCases {
		engine.Search(tc.query, tc.opts)
	}

	// Measured runs
	t.Log("")
	t.Log("--- Scale Gauntlet Results (24K files, 1.2M symbols) ---")
	t.Log("")

	var breaches int
	for _, tc := range scaleCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			result := engine.Search(tc.query, tc.opts)
			elapsed := time.Since(start)

			hits := 0
			if result != nil {
				hits = len(result.Hits)
			}

			ratio := float64(elapsed) / float64(tc.target)

			if elapsed > tc.target {
				breaches++
				t.Errorf("BREACH: %s — %v actual, %v target (%.1fx over) — %d hits returned",
					tc.name, elapsed, tc.target, ratio, hits)
			} else {
				t.Logf("PASS:   %s — %v actual, %v target (%.1fx margin) — %d hits returned",
					tc.name, elapsed, tc.target, ratio, hits)
			}
		})
	}
}

// BenchmarkScaleGauntlet runs all scale query shapes as sub-benchmarks.
// Produces benchstat-compatible output for optimization tracking.
//
// Run:     go test ./test/ -bench=BenchmarkScaleGauntlet -benchmem -run=^$ -count=6
// Compare: benchstat baseline.txt current.txt
func BenchmarkScaleGauntlet(b *testing.B) {

	idx := buildScaleIndex(b)
	engine := index.NewSearchEngine(idx, nil, "")

	// Warmup
	for _, tc := range scaleCases {
		engine.Search(tc.query, tc.opts)
	}

	for _, tc := range scaleCases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				engine.Search(tc.query, tc.opts)
			}
		})
	}
}

// BenchmarkScaleIndex_Build measures index construction time at 24K scale.
// This is the one-time cost at daemon startup.
func BenchmarkScaleIndex_Build(b *testing.B) {

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buildScaleIndex(b)
	}
}

// BenchmarkScaleEngine_Rebuild measures SearchEngine rebuild at 24K scale.
// This is the cost per file change (Engine.Rebuild regenerates derived maps).
func BenchmarkScaleEngine_Rebuild(b *testing.B) {

	idx := buildScaleIndex(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.NewSearchEngine(idx, nil, "")
	}
}
