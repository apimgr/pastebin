package secretbox

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := testKey()
	plaintext := []byte("coordinated disclosure report body")
	sealed, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Errorf("round trip mismatch: got %q", opened)
	}
}

func TestSealUniqueNonce(t *testing.T) {
	key := testKey()
	a, _ := Seal(key, []byte("same"))
	b, _ := Seal(key, []byte("same"))
	if bytes.Equal(a, b) {
		t.Error("two seals of identical plaintext produced identical ciphertext")
	}
}

func TestOpenWrongKeyFails(t *testing.T) {
	sealed, _ := Seal(testKey(), []byte("secret"))
	wrong := make([]byte, 32)
	if _, err := Open(wrong, sealed); err == nil {
		t.Error("Open with wrong key should fail authentication")
	}
}

func TestOpenTamperedFails(t *testing.T) {
	key := testKey()
	sealed, _ := Seal(key, []byte("secret"))
	sealed[len(sealed)-1] ^= 0xff
	if _, err := Open(key, sealed); err == nil {
		t.Error("Open of tampered ciphertext should fail")
	}
}

func TestKeySizeValidation(t *testing.T) {
	if _, err := Seal([]byte("short"), []byte("x")); err != ErrKeySize {
		t.Errorf("Seal: expected ErrKeySize, got %v", err)
	}
	if _, err := Open([]byte("short"), []byte("x")); err != ErrKeySize {
		t.Errorf("Open: expected ErrKeySize, got %v", err)
	}
}

func TestOpenShortCiphertext(t *testing.T) {
	if _, err := Open(testKey(), []byte("tiny")); err != ErrShortCiphertext {
		t.Errorf("expected ErrShortCiphertext, got %v", err)
	}
}
