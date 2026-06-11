package cache

// Internal tests for the cache package — covers unexported implementation
// details that cannot be reached from the external test file.

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestReaper_EvictsExpiredEntries verifies the background reaper goroutine
// removes expired entries from the memory store.
func TestReaper_EvictsExpiredEntries(t *testing.T) {
	orig := reaperTickInterval
	reaperTickInterval = 5 * time.Millisecond
	defer func() { reaperTickInterval = orig }()

	c := newMemoryCache("rpr:", time.Hour)

	ctx := context.Background()
	// Store an entry that expires almost immediately.
	if err := c.Set(ctx, "k", "v", 10*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for the entry to expire and the reaper to sweep.
	time.Sleep(80 * time.Millisecond)

	c.mu.RLock()
	_, ok := c.data["rpr:k"]
	c.mu.RUnlock()
	if ok {
		t.Error("reaper should have evicted the expired entry from the internal map")
	}
}

// TestRedisCache_Close verifies Close releases the underlying client pool.
// go-redis uses lazy connections — NewClient does not open a TCP socket, so
// Close is safe to call without a live server.
func TestRedisCache_Close(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	rc := &redisCache{client: client, prefix: "c:", defaultTTL: time.Hour}
	if err := rc.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// TestRedisCache_Key verifies the internal key helper prepends the prefix.
func TestRedisCache_Key(t *testing.T) {
	rc := &redisCache{prefix: "pfx:", defaultTTL: time.Hour}
	if got := rc.key("foo"); got != "pfx:foo" {
		t.Errorf("key() = %q, want %q", got, "pfx:foo")
	}
}

// TestReaper_LeavesNonExpiredEntries confirms the reaper does not remove
// entries whose TTL has not yet elapsed.
func TestReaper_LeavesNonExpiredEntries(t *testing.T) {
	orig := reaperTickInterval
	reaperTickInterval = 5 * time.Millisecond
	defer func() { reaperTickInterval = orig }()

	c := newMemoryCache("rpr2:", time.Hour)

	ctx := context.Background()
	// Entry with a long TTL — must survive the reaper sweep.
	if err := c.Set(ctx, "live", "value", 10*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Let the reaper run at least once.
	time.Sleep(30 * time.Millisecond)

	c.mu.RLock()
	_, ok := c.data["rpr2:live"]
	c.mu.RUnlock()
	if !ok {
		t.Error("reaper must not evict an entry whose TTL has not elapsed")
	}
}
