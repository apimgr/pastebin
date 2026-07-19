package maintenance

// Tests for RunCompliance and its strictest-requirement-wins resolution
// helpers (AI.md Compliance Requirements Matrix).

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCompliance_UnknownActionRejected(t *testing.T) {
	dir := t.TempDir()
	if err := RunCompliance("purge", ComplianceOptions{ConfigDir: dir, LogDir: dir}); err == nil {
		t.Error("expected error for an unknown action")
	}
}

func TestRunCompliance_ReportNoStandardsEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := RunCompliance("report", ComplianceOptions{ConfigDir: dir, LogDir: dir}); err != nil {
		t.Fatalf("RunCompliance: %v", err)
	}
}

func TestRunCompliance_ReportWithStandardsEnabled(t *testing.T) {
	dir := t.TempDir()
	yaml := "server:\n  compliance:\n    gdpr: true\n    hipaa: true\n"
	if err := os.WriteFile(filepath.Join(dir, "server.yml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunCompliance("report", ComplianceOptions{ConfigDir: dir, LogDir: dir}); err != nil {
		t.Fatalf("RunCompliance: %v", err)
	}
}

// ─── strictest-wins resolution helpers ──────────────────────────────────────

func TestStrictestRetention(t *testing.T) {
	got := strictestRetention([]standardRequirement{
		{retentionYears: 1},
		{retentionYears: 6},
		{retentionYears: 3},
	})
	if got != 6 {
		t.Errorf("strictestRetention = %d, want 6", got)
	}
}

func TestStrictestRetention_Empty(t *testing.T) {
	if got := strictestRetention(nil); got != 0 {
		t.Errorf("strictestRetention(nil) = %d, want 0", got)
	}
}

func TestStrictestBreachNotification(t *testing.T) {
	got := strictestBreachNotification([]standardRequirement{
		{breachNotifyHours: 72},
		{breachNotifyHours: 0}, // SOC2: not specified, must be ignored
		{breachNotifyHours: 60 * 24},
	})
	if got != 72 {
		t.Errorf("strictestBreachNotification = %d, want 72 (shortest wins)", got)
	}
}

func TestStrictestBreachNotification_NoneSpecified(t *testing.T) {
	got := strictestBreachNotification([]standardRequirement{{breachNotifyHours: 0}})
	if got != 0 {
		t.Errorf("strictestBreachNotification = %d, want 0", got)
	}
}

func TestStrictestSessionTimeout(t *testing.T) {
	got := strictestSessionTimeout([]standardRequirement{
		{sessionTimeoutMins: 15},
		{sessionTimeoutMins: 0},
		{sessionTimeoutMins: 30},
	})
	if got != 15 {
		t.Errorf("strictestSessionTimeout = %d, want 15 (shortest wins)", got)
	}
}

func TestAnyRightToErasure(t *testing.T) {
	if !anyRightToErasure([]standardRequirement{{rightToErasure: false}, {rightToErasure: true}}) {
		t.Error("expected true when any standard grants right to erasure")
	}
	if anyRightToErasure([]standardRequirement{{rightToErasure: false}}) {
		t.Error("expected false when no standard grants right to erasure")
	}
}

func TestAnyDataPortability(t *testing.T) {
	if !anyDataPortability([]standardRequirement{{dataPortability: false}, {dataPortability: true}}) {
		t.Error("expected true when any standard grants data portability")
	}
	if anyDataPortability([]standardRequirement{{dataPortability: false}}) {
		t.Error("expected false when no standard grants data portability")
	}
}

func TestQuantifiedRequirements_GDPRHIPAAOverlap(t *testing.T) {
	// Documents the AI.md example: GDPR (1yr/72hr) + HIPAA (6yr/60hr) enabled
	// together resolves to the strictest per-field values, not one standard
	// winning outright.
	active := []standardRequirement{quantifiedRequirements["GDPR"], quantifiedRequirements["HIPAA"]}
	if got := strictestRetention(active); got != 6 {
		t.Errorf("retention = %d, want 6 (HIPAA strictest)", got)
	}
	if got := strictestBreachNotification(active); got != 72 {
		t.Errorf("breach notification = %d, want 72 (GDPR strictest/shortest)", got)
	}
	if got := strictestSessionTimeout(active); got != 15 {
		t.Errorf("session timeout = %d, want 15 (HIPAA specifies, GDPR doesn't)", got)
	}
	if !anyRightToErasure(active) {
		t.Error("expected right to erasure true (GDPR grants it)")
	}
}
