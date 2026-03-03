package app

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeGo_NormalExecution(t *testing.T) {
	var wg sync.WaitGroup
	var ran int32
	safeGo(&wg, "test-normal", nil, func() {
		atomic.StoreInt32(&ran, 1)
	})
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&ran))
}

func TestSafeGo_PanicRecovery(t *testing.T) {
	var wg sync.WaitGroup
	var panicName string
	var panicVal interface{}
	var mu sync.Mutex

	safeGo(&wg, "test-panic", func(name string, r interface{}) {
		mu.Lock()
		panicName = name
		panicVal = r
		mu.Unlock()
	}, func() {
		panic("boom")
	})
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "test-panic", panicName)
	assert.Equal(t, "boom", panicVal)
}

func TestSafeGo_WaitGroupDrained(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		safeGo(&wg, "test-drain", nil, func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines drained — success.
	case <-time.After(5 * time.Second):
		t.Fatal("WaitGroup not drained within timeout")
	}
}

func TestSafeGo_PanicStillDrainsWaitGroup(t *testing.T) {
	var wg sync.WaitGroup

	safeGo(&wg, "test-panic-drain", nil, func() {
		panic("test panic")
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// WaitGroup drained despite panic — success.
	case <-time.After(5 * time.Second):
		t.Fatal("WaitGroup not drained after panic")
	}
}

func TestApp_SafeGo_TrackedByBgWg(t *testing.T) {
	// Verify SafeGo goroutines are tracked and drained.
	a := &App{
		stopCh: make(chan struct{}),
	}

	var count int32
	for i := 0; i < 5; i++ {
		a.SafeGo("test-tracked", func() {
			atomic.AddInt32(&count, 1)
			time.Sleep(20 * time.Millisecond)
		})
	}

	a.bgWg.Wait()
	require.Equal(t, int32(5), atomic.LoadInt32(&count))
}
