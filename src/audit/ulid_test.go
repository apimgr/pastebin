package audit

import (
	"strings"
	"testing"
	"time"
)

func TestNewULID_Format(t *testing.T) {
	ts := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	id, err := newULID(ts)
	if err != nil {
		t.Fatalf("newULID: %v", err)
	}
	if len(id) != 26 {
		t.Fatalf("ULID length = %d, want 26", len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune(crockford, c) {
			t.Errorf("ULID contains non-Crockford char %q in %q", c, id)
		}
	}
}

func TestNewULID_Unique(t *testing.T) {
	ts := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id, err := newULID(ts)
		if err != nil {
			t.Fatalf("newULID: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate ULID at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestNewULID_TimeOrdering(t *testing.T) {
	early, _ := newULID(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	late, _ := newULID(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	// The 48-bit timestamp is most-significant, so a later time sorts after an
	// earlier one lexically (10-char timestamp prefix).
	if early[:10] >= late[:10] {
		t.Errorf("timestamp prefix not monotonic: early=%s late=%s", early, late)
	}
}
