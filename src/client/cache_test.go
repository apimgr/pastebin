package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResponseCachePutGetAndInvalidate(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := responseCacheConfig{enabled: true, ttl: time.Minute, maxSize: 1024}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/plain"}}}
	cfg.put("https://example.test/raw/abc", resp, []byte("cached paste"))
	cached, ok := cfg.get("https://example.test/raw/abc")
	if !ok {
		t.Fatal("expected cached response")
	}
	body, err := io.ReadAll(cached.Body)
	if err != nil {
		t.Fatalf("read cached response: %v", err)
	}
	if string(body) != "cached paste" {
		t.Fatalf("cached body = %q, want %q", body, "cached paste")
	}
	if cached.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("cached content type = %q", cached.Header.Get("Content-Type"))
	}
	cfg.invalidate()
	if _, ok := cfg.get("https://example.test/raw/abc"); ok {
		t.Fatal("cache entry remained after invalidation")
	}
}

func TestResponseCacheRejectsErrorsAndOversizedBodies(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := responseCacheConfig{enabled: true, ttl: time.Minute, maxSize: 3}
	cfg.put("error", &http.Response{StatusCode: http.StatusInternalServerError}, []byte("bad"))
	cfg.put("large", &http.Response{StatusCode: http.StatusOK}, []byte("large"))
	for _, key := range []string{"error", "large"} {
		if _, ok := cfg.get(key); ok {
			t.Errorf("unexpected cache entry for %s", key)
		}
	}
}

func TestResponseCacheHonorsNoStore(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := responseCacheConfig{enabled: true, ttl: time.Minute, maxSize: 1024}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{"private, no-store"}}}
	cfg.put("private", resp, []byte(strings.Repeat("x", 1)))
	if _, ok := cfg.get("private"); ok {
		t.Fatal("no-store response was cached")
	}
}
