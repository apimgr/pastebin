package server

import (
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// captchaTTL bounds how long a built-in simple captcha challenge stays valid.
const captchaTTL = 10 * time.Minute

// simpleCaptcha is a rendered arithmetic challenge: a human-readable question
// plus a signed token that encodes the expected answer and an expiry. The token
// is stateless (HMAC-signed with the server csrfSecret) so no server-side
// session store is required to validate the answer on submission (PART 31).
type simpleCaptcha struct {
	Question string
	Token    string
}

// generateSimpleCaptcha builds an addition challenge with two single-digit
// operands and returns the question and a signed answer token.
func (s *Server) generateSimpleCaptcha() (simpleCaptcha, error) {
	buf := make([]byte, 2)
	if _, err := crand.Read(buf); err != nil {
		return simpleCaptcha{}, err
	}
	a := int(buf[0]%9) + 1
	b := int(buf[1]%9) + 1
	answer := a + b
	expiry := time.Now().Add(captchaTTL).Unix()
	payload := strconv.Itoa(answer) + ":" + strconv.FormatInt(expiry, 10)
	token := s.signCaptchaPayload(payload)
	return simpleCaptcha{
		Question: fmt.Sprintf("What is %d + %d?", a, b),
		Token:    token,
	}, nil
}

// validateSimpleCaptcha reports whether the submitted answer matches the signed
// token and the token has not expired. Comparison is constant-time.
func (s *Server) validateSimpleCaptcha(token, answer string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.csrfSecret)
	mac.Write(payloadBytes)
	if subtle.ConstantTimeCompare(sig, mac.Sum(nil)) != 1 {
		return false
	}
	fields := strings.SplitN(string(payloadBytes), ":", 2)
	if len(fields) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	want := strings.TrimSpace(fields[0])
	got := strings.TrimSpace(answer)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// signCaptchaPayload signs an opaque captcha payload with the server csrfSecret
// and returns "base64(payload).base64(hmac)".
func (s *Server) signCaptchaPayload(payload string) string {
	mac := hmac.New(sha256.New, s.csrfSecret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
