package server

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/common/email"
	"github.com/apimgr/pastebin/src/audit"
	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
)

// defaultDisclosureDays is the coordinated-disclosure window offered by default
// (AI.md 14138: "Number of days (default 90)").
const defaultDisclosureDays = 90

// securityComponents populates the affected-component dropdown on the security
// form (AI.md 14130). "other" pairs with the free-text component_other field.
var securityComponents = []string{
	"auth", "api", "frontend", "cli", "paste", "storage",
	"config", "scheduler", "email", "other",
}

// validSeverities is the researcher self-assessment set (AI.md 14132).
var validSeverities = map[string]bool{
	"Critical": true, "High": true, "Medium": true, "Low": true, "Informational": true,
}

// validCreditPrefs is the acknowledgments preference set (AI.md 14139).
var validCreditPrefs = map[string]bool{
	"name": true, "handle": true, "no": true, "anonymous": true,
}

// clientIP extracts the peer IP from RemoteAddr for security.log entries.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// randomHex returns n random hex characters (n/2 random bytes).
func randomHex(n int) (string, error) {
	b := make([]byte, (n+1)/2)
	if _, err := crand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}

// sanitizeComponent reduces an affected-component label to a short, safe token
// for at-rest metadata: it keeps letters, digits, '-', '_', '/' and '.', drops
// everything else, and caps the length. This prevents free-text PII or markup
// from landing in the plaintext metadata columns (AI.md 14151 "sanitized
// affected-component, NO researcher PII").
func sanitizeComponent(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '/', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
		if b.Len() >= 64 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unspecified"
	}
	return out
}

// handleSecurityReport processes a validated coordinated-disclosure submission
// (AI.md 14143-14151). The security_id has already been re-validated by the
// caller. It gathers the security-research fields, encrypts the full report at
// rest, allocates a tracking id, notifies the maintainer and researcher by
// email, logs a PII-free security.log entry, and renders a content-negotiated
// success response. The plaintext report body is NEVER persisted.
func (s *Server) handleSecurityReport(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	pc := cfg.Server.Pages.Contact
	data := s.contactData(r)
	// Force security-mode rendering: contactData only enables it when a valid
	// security_id rides in the query string, but a re-validated id may have
	// arrived via the hidden field. This keeps the form in security mode on any
	// validation-error re-render.
	data["SecurityMode"] = true
	data["SecurityID"] = s.currentSecurityID()
	data["Components"] = securityComponents

	name := strings.TrimSpace(r.PostFormValue("name"))
	fromEmail := strings.TrimSpace(r.PostFormValue("email"))
	gpg := strings.TrimSpace(r.PostFormValue("researcher_gpg"))
	component := strings.TrimSpace(r.PostFormValue("component"))
	componentOther := strings.TrimSpace(r.PostFormValue("component_other"))
	endpoint := strings.TrimSpace(r.PostFormValue("endpoint"))
	severity := strings.TrimSpace(r.PostFormValue("severity"))
	summary := strings.TrimSpace(r.PostFormValue("summary"))
	steps := strings.TrimSpace(r.PostFormValue("steps"))
	impact := strings.TrimSpace(r.PostFormValue("impact"))
	suggestedFix := strings.TrimSpace(r.PostFormValue("suggested_fix"))
	cveRequested := config.IsTruthy(r.PostFormValue("cve_requested"))
	creditPref := strings.TrimSpace(r.PostFormValue("credit_preference"))
	creditName := strings.TrimSpace(r.PostFormValue("credit_name"))
	agreement := config.IsTruthy(r.PostFormValue("agreement"))

	disclosureDays := defaultDisclosureDays
	if v := strings.TrimSpace(r.PostFormValue("disclosure_days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 365 {
			disclosureDays = n
		}
	}

	// Preserve entered values so a validation error does not clear the form.
	data["FormName"] = name
	data["FormEmail"] = fromEmail
	data["FormGPG"] = gpg
	data["FormComponentOther"] = componentOther
	data["FormEndpoint"] = endpoint
	data["FormSummary"] = summary
	data["FormSteps"] = steps
	data["FormImpact"] = impact
	data["FormSuggestedFix"] = suggestedFix
	data["FormDisclosureDays"] = disclosureDays
	data["FormCreditName"] = creditName

	fail := func(status int, msg string) {
		data["ContactError"] = msg
		w.WriteHeader(status)
		s.renderTemplate(w, r, "contact.html", data)
	}

	// Server-side validation — the server is authoritative (never trust client).
	if name == "" || fromEmail == "" || component == "" || severity == "" ||
		summary == "" || steps == "" || impact == "" || creditPref == "" {
		fail(http.StatusBadRequest, "Please complete all required fields.")
		return
	}
	if !strings.Contains(fromEmail, "@") || strings.ContainsAny(fromEmail, " \t\r\n") {
		fail(http.StatusBadRequest, "Please enter a valid email address.")
		return
	}
	if !validSeverities[severity] {
		fail(http.StatusBadRequest, "Please choose a valid severity.")
		return
	}
	if !validCreditPrefs[creditPref] {
		fail(http.StatusBadRequest, "Please choose a valid credit preference.")
		return
	}
	if !agreement {
		fail(http.StatusBadRequest, "You must agree to coordinated disclosure to submit a report.")
		return
	}

	// Validate the built-in simple captcha (provider captchas are not verified
	// server-side here — same policy as the standard contact form).
	if pc.Captcha == "simple" {
		if !s.validateSimpleCaptcha(r.PostFormValue("captcha_token"), r.PostFormValue("captcha")) {
			fail(http.StatusBadRequest, "Captcha answer was incorrect. Please try again.")
			return
		}
	}

	// Resolve the affected component: "other" defers to the free-text field.
	componentLabel := component
	if component == "other" && componentOther != "" {
		componentLabel = componentOther
	}
	sanitized := sanitizeComponent(componentLabel)

	// Encryption at rest is MANDATORY — refuse rather than persist plaintext.
	key, err := cfg.EncryptionKey()
	if err != nil {
		log.Printf("security report: encryption key unavailable: %v", err)
		fail(http.StatusServiceUnavailable, "Security reporting is temporarily unavailable. Please email the security contact directly.")
		return
	}

	trackingID, err := randomHex(16)
	if err != nil {
		fail(http.StatusInternalServerError, "Could not process your report. Please try again.")
		return
	}
	trackingID = "sec_" + trackingID

	rawToken, err := randomHex(32)
	if err != nil {
		fail(http.StatusInternalServerError, "Could not process your report. Please try again.")
		return
	}
	tokenHash := sha256.Sum256([]byte(rawToken))

	timestamp := time.Now().Format(time.RFC3339)
	report := buildReportBody(reportFields{
		trackingID:     trackingID,
		timestamp:      timestamp,
		name:           name,
		email:          fromEmail,
		gpg:            gpg,
		component:      componentLabel,
		endpoint:       endpoint,
		severity:       severity,
		summary:        summary,
		steps:          steps,
		impact:         impact,
		suggestedFix:   suggestedFix,
		cveRequested:   cveRequested,
		disclosureDays: disclosureDays,
		creditPref:     creditPref,
		creditName:     creditName,
		userAgent:      r.UserAgent(),
		remoteIP:       clientIP(r),
		appVersion:     s.version,
		commitHash:     s.commitID,
	})

	// Seal the report at rest: prefer the project PGP key, fall back to
	// AES-256-GCM keyed by server.security.encryption_key (PART 11).
	sealed, encMethod, err := s.encryptSecurityReport([]byte(report), key)
	if err != nil {
		log.Printf("security report: seal failed: %v", err)
		fail(http.StatusInternalServerError, "Could not securely store your report. Please try again.")
		return
	}

	// Credit is only displayed for name/handle; anonymous and no-credit keep the
	// display name out of the stored metadata.
	storedCreditName := creditName
	if creditPref == "anonymous" || creditPref == "no" {
		storedCreditName = ""
	}

	rec := &database.SecurityReport{
		TrackingID:       trackingID,
		Severity:         severity,
		Component:        sanitized,
		EncryptedBody:    sealed,
		EncMethod:        encMethod,
		CreditPreference: creditPref,
		CreditName:       storedCreditName,
		TokenHash:        hex.EncodeToString(tokenHash[:]),
		DisclosureDays:   disclosureDays,
	}
	if err := s.db.CreateSecurityReport(rec); err != nil {
		log.Printf("security report: store failed: %v", err)
		fail(http.StatusInternalServerError, "Could not record your report. Please try again.")
		return
	}

	// Notifications are best-effort: the report is already durably stored, so an
	// email failure must not lose it — log and continue to the success response.
	s.sendSecurityReportEmails(r, cfg, securityEmailCtx{
		trackingID:     trackingID,
		timestamp:      timestamp,
		researcher:     fromEmail,
		severity:       severity,
		summary:        summary,
		component:      componentLabel,
		cveRequested:   cveRequested,
		disclosureDays: disclosureDays,
		encMethod:      "aes-256-gcm",
		statusToken:    rawToken,
	})

	// Admin webhook summary — server-internal event, NEVER user content (PART 17).
	// Only the tracking id, severity, and sanitized component leave the server.
	s.notifyRole(cfg, "admin", "admin.security_report_received",
		"Security report received: "+trackingID,
		"A new security report was received. Severity: "+severity+"; component: "+sanitized+". Decrypt the at-rest report to read the full submission.",
		severity)

	// PII-free audit line — tracking id, severity, sanitized component only.
	s.securityLog("security.report_received",
		"tracking_id", trackingID, "severity", severity, "component", sanitized)
	s.auditLog(r, audit.Entry{
		Event:    "security.report_received",
		Severity: audit.SeverityWarn,
		Target:   &audit.Target{Type: "security_report", ID: trackingID},
		Details:  map[string]any{"severity": severity, "component": sanitized},
	})

	// Content-negotiated success (AI.md 14150).
	if ct := detectClientType(r); ct == "json" || ct == "text" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"data": map[string]string{"tracking_id": trackingID},
		})
		return
	}

	data["ContactSuccess"] = fmt.Sprintf(
		"Thank you — your security report was received and encrypted. Your tracking ID is %s. Check your email for confirmation.",
		trackingID)
	// Clear the preserved fields so the form resets after a successful submit.
	for _, k := range []string{"FormName", "FormEmail", "FormGPG", "FormComponentOther",
		"FormEndpoint", "FormSummary", "FormSteps", "FormImpact", "FormSuggestedFix",
		"FormCreditName"} {
		delete(data, k)
	}
	s.renderTemplate(w, r, "contact.html", data)
}

// reportFields carries the plaintext values assembled into the encrypted report
// body. This struct never leaves this file and is never persisted in the clear.
type reportFields struct {
	trackingID, timestamp                        string
	name, email, gpg                             string
	component, endpoint, severity, summary       string
	steps, impact, suggestedFix                  string
	cveRequested                                 bool
	disclosureDays                               int
	creditPref, creditName                       string
	userAgent, remoteIP, appVersion, commitHash  string
}

// buildReportBody renders the full researcher submission as a plain-text report.
// This is the ONLY representation of the vulnerability detail and is encrypted
// before it touches disk (AI.md 14147).
func buildReportBody(f reportFields) string {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Tracking ID: %s\n", f.trackingID)
	fmt.Fprintf(&b, "Received: %s\n\n", f.timestamp)
	fmt.Fprintf(&b, "Reporter: %s <%s>\n", f.name, f.email)
	if f.gpg != "" {
		fmt.Fprintf(&b, "Reporter PGP: %s\n", f.gpg)
	}
	fmt.Fprintf(&b, "Credit preference: %s", f.creditPref)
	if f.creditName != "" {
		fmt.Fprintf(&b, " (%s)", f.creditName)
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Severity (self-assessed): %s\n", f.severity)
	fmt.Fprintf(&b, "Affected component: %s\n", f.component)
	if f.endpoint != "" {
		fmt.Fprintf(&b, "Affected endpoint: %s\n", f.endpoint)
	}
	fmt.Fprintf(&b, "CVE requested: %s\n", yn(f.cveRequested))
	fmt.Fprintf(&b, "Requested disclosure window: %d days\n\n", f.disclosureDays)
	fmt.Fprintf(&b, "Summary:\n%s\n\n", f.summary)
	fmt.Fprintf(&b, "Steps to reproduce:\n%s\n\n", f.steps)
	fmt.Fprintf(&b, "Impact:\n%s\n\n", f.impact)
	if f.suggestedFix != "" {
		fmt.Fprintf(&b, "Suggested fix:\n%s\n\n", f.suggestedFix)
	}
	b.WriteString("-- triage metadata --\n")
	fmt.Fprintf(&b, "app_version: %s\n", f.appVersion)
	fmt.Fprintf(&b, "commit_hash: %s\n", f.commitHash)
	fmt.Fprintf(&b, "request_user_agent: %s\n", f.userAgent)
	fmt.Fprintf(&b, "request_ip: %s\n", f.remoteIP)
	return b.String()
}

// securityEmailCtx carries the values needed for the two notification emails.
type securityEmailCtx struct {
	trackingID, timestamp, researcher string
	severity, summary, component      string
	cveRequested                      bool
	disclosureDays                    int
	encMethod, statusToken            string
}

// sendSecurityReportEmails dispatches the maintainer notification and the
// researcher acknowledgment (AI.md 14148-14149). Both are best-effort; failures
// are logged and do not affect the already-stored report. Full PGP-encrypted
// delivery and attachment of the encrypted body is completed with the PGP
// subsystem; until then the maintainer email carries triage metadata and the
// encrypted body remains retrievable from the at-rest store.
func (s *Server) sendSecurityReportEmails(r *http.Request, cfg *config.Config, c securityEmailCtx) {
	m := email.New(&cfg.Server.Notifications.Email, cfg.Web.SiteTitle, s.baseURL(r), cfg.Server.FQDN)
	if !m.Enabled() {
		return
	}
	yn := "no"
	if c.cveRequested {
		yn = "yes"
	}

	if to := strings.TrimSpace(cfg.SecurityReportEmail()); to != "" {
		if err := m.Send(to, "security_report_received", map[string]string{
			"severity":        c.severity,
			"summary":         c.summary,
			"timestamp":       c.timestamp,
			"tracking_id":     c.trackingID,
			"component":       c.component,
			"cve_requested":   yn,
			"disclosure_days": strconv.Itoa(c.disclosureDays),
			"enc_method":      c.encMethod,
			"retrieval_note":  "Decrypt the at-rest report with the server encryption key to read the full submission.",
		}); err != nil {
			log.Printf("security report: maintainer notification failed: %v", err)
		}
	}

	statusURL := fmt.Sprintf("%s/server/security/report/%s?token=%s",
		s.baseURL(r), c.trackingID, c.statusToken)
	if err := m.Send(c.researcher, "security_report_ack", map[string]string{
		"timestamp":   c.timestamp,
		"tracking_id": c.trackingID,
		"status_url":  statusURL,
	}); err != nil {
		log.Printf("security report: researcher acknowledgment failed: %v", err)
	}
}
