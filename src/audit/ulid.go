package audit

import (
	"crypto/rand"
	"time"
)

// crockford is the Crockford base32 alphabet used by ULID (no I, L, O, U).
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID returns a 26-character Crockford base32 ULID: a 48-bit millisecond
// timestamp (most significant) followed by 80 bits of cryptographic randomness.
// This is the canonical ULID layout; the identifier is monotonic-ish by time
// and globally unique per entry.
func newULID(t time.Time) (string, error) {
	ms := uint64(t.UnixMilli())
	var id [16]byte
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	if _, err := rand.Read(id[6:]); err != nil {
		return "", err
	}
	return encodeCrockford(id), nil
}

// encodeCrockford renders a 128-bit value as 26 Crockford base32 characters.
// The 128 data bits are left-padded with 2 zero bits to reach 130 bits (26*5),
// then emitted 5 bits per character, most significant first.
func encodeCrockford(id [16]byte) string {
	out := make([]byte, 26)
	for i := 0; i < 26; i++ {
		var v byte
		for b := 0; b < 5; b++ {
			// Absolute bit position within the 130-bit padded value.
			dataPos := i*5 + b - 2
			var bit byte
			if dataPos >= 0 {
				byteIdx := dataPos / 8
				bitInByte := 7 - (dataPos % 8)
				bit = (id[byteIdx] >> uint(bitInByte)) & 1
			}
			v = (v << 1) | bit
		}
		out[i] = crockford[v]
	}
	return string(out)
}
