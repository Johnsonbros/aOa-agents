package fsnotify

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// S-04: fsnotify Watcher Adapter — Detect file changes, trigger re-index
// Goals: G1 (O(1) performance)
// Expectation: File changes detected and callback fired within <100ms
// =============================================================================

// waitForCallback waits up to timeout for the callback channel to receive a value.
func waitForCallback(ch <-chan string, timeout time.Duration) (string, bool) {
	select {
	case v := <-ch:
		return v, true
	case <-time.After(timeout):
		return "", false
	}
}

func TestWatcher_DetectsFileChange(t *testing.T) {
	// S-04, G1: Create a temp file, start watching, modify file.
	// onChange callback fires with the modified file path.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.py")
	require.NoError(t, os.WriteFile(testFile, []byte("# original"), 0644))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	changed := make(chan string, 10)
	err = w.Watch(dir, func(path string) {
		changed <- path
	})
	require.NoError(t, err)

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	require.NoError(t, os.WriteFile(testFile, []byte("# modified"), 0644))

	path, ok := waitForCallback(changed, 2*time.Second)
	assert.True(t, ok, "expected callback for file change")
	assert.Equal(t, testFile, path)
}

func TestWatcher_DetectsNewFile(t *testing.T) {
	// S-04, G1: Create a new file in watched directory.
	// onChange fires with the new file path.
	dir := t.TempDir()

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	changed := make(chan string, 10)
	err = w.Watch(dir, func(path string) {
		changed <- path
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	newFile := filepath.Join(dir, "new_file.py")
	require.NoError(t, os.WriteFile(newFile, []byte("# new"), 0644))

	path, ok := waitForCallback(changed, 2*time.Second)
	assert.True(t, ok, "expected callback for new file")
	assert.Equal(t, newFile, path)
}

func TestWatcher_DetectsDeletedFile(t *testing.T) {
	// S-04, G1: Delete a file in watched directory.
	// onChange fires so index can remove stale entries.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "to_delete.py")
	require.NoError(t, os.WriteFile(testFile, []byte("# delete me"), 0644))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	changed := make(chan string, 10)
	err = w.Watch(dir, func(path string) {
		changed <- path
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, os.Remove(testFile))

	path, ok := waitForCallback(changed, 2*time.Second)
	assert.True(t, ok, "expected callback for deleted file")
	assert.Equal(t, testFile, path)
}

func TestWatcher_IgnoresNonCodeFiles(t *testing.T) {
	// S-04, G7: Changes to .git/, node_modules/, .DS_Store, etc.
	// do NOT trigger onChange. Only code files trigger re-index.
	dir := t.TempDir()

	// Create ignored directories
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))
	nmDir := filepath.Join(dir, "node_modules")
	require.NoError(t, os.MkdirAll(nmDir, 0755))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	changed := make(chan string, 10)
	err = w.Watch(dir, func(path string) {
		changed <- path
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Write to ignored locations
	os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref"), 0644)
	os.WriteFile(filepath.Join(nmDir, "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "test.swp"), []byte("x"), 0644)

	// None of these should trigger callback
	_, ok := waitForCallback(changed, 500*time.Millisecond)
	assert.False(t, ok, "should not have received callback for ignored files")

	// But a real code file should trigger
	codeFile := filepath.Join(dir, "main.py")
	require.NoError(t, os.WriteFile(codeFile, []byte("# code"), 0644))

	path, ok := waitForCallback(changed, 2*time.Second)
	assert.True(t, ok, "expected callback for code file")
	assert.Equal(t, codeFile, path)
}

func TestWatcher_ReindexLatency(t *testing.T) {
	// S-04, G1: Time from file change to onChange callback < 100ms.
	// Measures fsnotify event delivery, not re-indexing cost.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "latency.py")
	require.NoError(t, os.WriteFile(testFile, []byte("# initial"), 0644))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	var callbackTime time.Time
	var mu sync.Mutex
	err = w.Watch(dir, func(path string) {
		mu.Lock()
		callbackTime = time.Now()
		mu.Unlock()
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	writeTime := time.Now()
	require.NoError(t, os.WriteFile(testFile, []byte("# changed"), 0644))

	// Wait for callback
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	latency := callbackTime.Sub(writeTime)
	mu.Unlock()

	assert.Less(t, latency, 100*time.Millisecond, "callback latency %v exceeds 100ms", latency)
	t.Logf("Callback latency: %v", latency)
}

func TestWatcher_ExcludeDropsMatchingPaths(t *testing.T) {
	// L15.3: Events for excluded paths (e.g. .aoa/, .claude/settings.local.json)
	// are silently dropped before callback. Non-excluded paths still fire.
	dir := t.TempDir()

	// Create subdirectories that simulate .aoa/ and .claude/
	aoaDir := filepath.Join(dir, ".aoa")
	require.NoError(t, os.MkdirAll(aoaDir, 0755))
	claudeDir := filepath.Join(dir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	// Exclude .aoa/ subtree and .claude/settings.local.json (+ .tmp variants)
	w.Exclude([]string{
		aoaDir + string(filepath.Separator),
		filepath.Join(claudeDir, "settings.local.json"),
	})

	// Must manually add the directories since .aoa is in ignoreDirs
	// and won't be walked by Watch.
	require.NoError(t, w.fw.Add(aoaDir))
	require.NoError(t, w.fw.Add(claudeDir))

	changed := make(chan string, 10)
	err = w.Watch(dir, func(path string) {
		changed <- path
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Write to excluded paths — none should trigger callback
	os.WriteFile(filepath.Join(aoaDir, "aoa.db"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(aoaDir, "status.json"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json.tmp.12345"), []byte("{}"), 0644)

	_, ok := waitForCallback(changed, 500*time.Millisecond)
	assert.False(t, ok, "should not have received callback for excluded paths")

	// Write to a non-excluded .claude/ file — should trigger
	otherFile := filepath.Join(claudeDir, "CLAUDE.md")
	require.NoError(t, os.WriteFile(otherFile, []byte("# test"), 0644))

	path, ok := waitForCallback(changed, 2*time.Second)
	assert.True(t, ok, "expected callback for non-excluded .claude/ file")
	assert.Equal(t, otherFile, path)
}

func TestWatcher_ExcludeIsExcluded(t *testing.T) {
	// Unit test for isExcluded — no filesystem needed
	w := &Watcher{}
	w.Exclude([]string{
		"/project/.aoa/",
		"/project/.claude/settings.local.json",
	})

	assert.True(t, w.isExcluded("/project/.aoa/aoa.db"))
	assert.True(t, w.isExcluded("/project/.aoa/hook/context.jsonl"))
	assert.True(t, w.isExcluded("/project/.aoa/status.json"))
	assert.True(t, w.isExcluded("/project/.claude/settings.local.json"))
	assert.True(t, w.isExcluded("/project/.claude/settings.local.json.tmp.98765"))

	assert.False(t, w.isExcluded("/project/.claude/CLAUDE.md"))
	assert.False(t, w.isExcluded("/project/src/main.go"))
	assert.False(t, w.isExcluded("/project/.aoa")) // dir itself, not under .aoa/
}

func TestWatcher_DebounceCoalescesAtomicWrite(t *testing.T) {
	// L15.4: An atomic write (write tmp + rename to target) should produce
	// exactly one callback, not two. The 500ms debounce window coalesces
	// both the Create/Write on the tmp path AND the Rename that lands on
	// the target path. Because rename delivers an event for the target file
	// name, we verify the target fires exactly once within the window.
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"v":1}`), 0644))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	var mu sync.Mutex
	counts := make(map[string]int)
	err = w.Watch(dir, func(path string) {
		mu.Lock()
		counts[path]++
		mu.Unlock()
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Simulate atomic write: write tmp, rename to target.
	// Both operations generate events for the target path within ~10ms.
	tmp := target + ".tmp.99999"
	require.NoError(t, os.WriteFile(tmp, []byte(`{"v":2}`), 0644))
	require.NoError(t, os.Rename(tmp, target))

	// Wait long enough for any duplicate to have arrived (>debounce window)
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	targetCount := counts[target]
	mu.Unlock()

	assert.Equal(t, 1, targetCount,
		"expected exactly 1 callback for target after atomic write, got %d", targetCount)
}

func TestWatcher_DebounceWindowValue(t *testing.T) {
	// L15.4: The debounce window must be >= 500ms so that events 200-300ms
	// apart (observed in production with 24k-file K8s deployments) are
	// coalesced. We verify by writing the same file twice with a 250ms gap
	// and asserting only one callback fires.
	dir := t.TempDir()
	testFile := filepath.Join(dir, "rapid.py")
	require.NoError(t, os.WriteFile(testFile, []byte("# v0"), 0644))

	w, err := NewWatcher()
	require.NoError(t, err)
	defer w.Stop()

	var mu sync.Mutex
	callCount := 0
	err = w.Watch(dir, func(path string) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// First write
	require.NoError(t, os.WriteFile(testFile, []byte("# v1"), 0644))
	// Wait 250ms — inside the 500ms debounce window
	time.Sleep(250 * time.Millisecond)
	// Second write — should be suppressed
	require.NoError(t, os.WriteFile(testFile, []byte("# v2"), 0644))

	// Wait for debounce window to fully expire
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	assert.Equal(t, 1, count,
		"expected 1 callback (second write within debounce window should be suppressed), got %d", count)
}

func TestWatcher_StopCleanup(t *testing.T) {
	// S-04, G5: After Stop(), no more callbacks fire.
	// Resources cleaned up, no goroutine leaks.
	dir := t.TempDir()

	w, err := NewWatcher()
	require.NoError(t, err)

	callCount := 0
	var mu sync.Mutex
	err = w.Watch(dir, func(path string) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Stop the watcher
	err = w.Stop()
	require.NoError(t, err)

	// Record count after stop
	mu.Lock()
	countAfterStop := callCount
	mu.Unlock()

	// Write file after stop — should NOT trigger callback
	os.WriteFile(filepath.Join(dir, "after_stop.py"), []byte("# nope"), 0644)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	countAfterWrite := callCount
	mu.Unlock()

	assert.Equal(t, countAfterStop, countAfterWrite, "callbacks fired after Stop()")

	// Double-stop should be safe
	err = w.Stop()
	assert.NoError(t, err)
}
