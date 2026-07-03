package server

import (
	"testing"
	"time"
)

func newSecurityIDTestServer() *Server {
	return &Server{installSecret: []byte("security-id-unit-test-installation-secret")}
}

func TestSecurityIDCurrentValidates(t *testing.T) {
	s := newSecurityIDTestServer()
	id := s.currentSecurityID()
	if len(id) != 16 {
		t.Fatalf("expected 16-char id, got %d: %q", len(id), id)
	}
	if !s.validSecurityID(id) {
		t.Errorf("current security_id %q rejected by validSecurityID", id)
	}
}

func TestSecurityIDDeterministic(t *testing.T) {
	s := newSecurityIDTestServer()
	w := time.Now().Unix() / securityIDWindow
	if a, b := s.securityIDForWindow(w), s.securityIDForWindow(w); a != b {
		t.Errorf("same window produced different ids: %q vs %q", a, b)
	}
}

func TestSecurityIDPreviousWindowAccepted(t *testing.T) {
	s := newSecurityIDTestServer()
	now := time.Now().Unix() / securityIDWindow
	prev := s.securityIDForWindow(now - 1)
	if !s.validSecurityID(prev) {
		t.Errorf("previous-window id %q should be accepted", prev)
	}
}

func TestSecurityIDStaleWindowRejected(t *testing.T) {
	s := newSecurityIDTestServer()
	now := time.Now().Unix() / securityIDWindow
	// Two windows back (96h) must not validate.
	stale := s.securityIDForWindow(now - 2)
	if s.validSecurityID(stale) {
		t.Errorf("stale-window id %q should be rejected", stale)
	}
}

func TestSecurityIDRejectsEmptyAndForeign(t *testing.T) {
	s := newSecurityIDTestServer()
	if s.validSecurityID("") {
		t.Error("empty id accepted")
	}
	// An id derived from a different installation secret must not validate.
	other := &Server{installSecret: []byte("a-different-installation-secret")}
	foreign := other.currentSecurityID()
	if s.validSecurityID(foreign) {
		t.Errorf("foreign id %q accepted", foreign)
	}
}

func TestSecurityIDNoSecret(t *testing.T) {
	s := &Server{}
	if got := s.currentSecurityID(); got != "" {
		t.Errorf("expected empty id with no secret, got %q", got)
	}
	if s.validSecurityID("anything") {
		t.Error("validSecurityID accepted an id with no installation secret")
	}
}

func TestItoa64(t *testing.T) {
	cases := map[int64]string{0: "0", 7: "7", 42: "42", 172800: "172800", -5: "-5"}
	for in, want := range cases {
		if got := itoa64(in); got != want {
			t.Errorf("itoa64(%d) = %q, want %q", in, got, want)
		}
	}
}
