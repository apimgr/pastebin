package notify

import (
	"crypto/rand"
	"fmt"
	"time"
)

// uuidV7 returns a RFC 9562 version-7 UUID string (48-bit Unix millisecond
// timestamp prefix + random). It is used for the X-Webhook-ID idempotency key
// (PART 13) so receivers can deduplicate retried deliveries. google/uuid v1.3
// predates NewV7, so the layout is assembled here.
func uuidV7() (string, error) {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		return "", err
	}
	// Version 7 in the high nibble of byte 6.
	b[6] = (b[6] & 0x0f) | 0x70
	// RFC 4122 variant (10xx) in the high bits of byte 8.
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
