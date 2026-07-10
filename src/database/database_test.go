package database_test

// Tests for the SQLiteDB implementation.
// Each test spins up a fresh in-process SQLite database so there are no
// external dependencies and tests are safe to run in parallel.

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/model"
)

// newTestDB creates an isolated SQLite database in a temp directory and
// registers cleanup to close and remove it when the test finishes.
func newTestDB(t *testing.T) database.DB {
	t.Helper()
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "pastebin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := database.NewDatabase("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// samplePaste builds a minimal valid Paste for insertion.
func samplePaste(id string) *model.Paste {
	return &model.Paste{
		ID:              id,
		Title:           "Test Paste",
		Content:         "hello world",
		Language:        "text",
		Visibility:      model.VisibilityPublic,
		BurnAfter:       0,
		DeleteTokenHash: "",
		Views:           0,
	}
}

// ─── Paste CRUD ───────────────────────────────────────────────────────────────

// TestCreateAndGetPaste verifies a round-trip: insert, retrieve, check every field.
func TestCreateAndGetPaste(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("abc12345")
	p.Title = "Round-trip"
	p.Content = "some content"
	p.Language = "go"
	p.Visibility = model.VisibilityUnlisted

	if err := db.CreatePaste(p); err != nil {
		t.Fatalf("CreatePaste: %v", err)
	}

	got, err := db.GetPasteByID("abc12345")
	if err != nil {
		t.Fatalf("GetPasteByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetPasteByID: got nil, want paste")
	}
	if got.ID != p.ID {
		t.Errorf("ID: got %q, want %q", got.ID, p.ID)
	}
	if got.Title != p.Title {
		t.Errorf("Title: got %q, want %q", got.Title, p.Title)
	}
	if got.Content != p.Content {
		t.Errorf("Content: got %q, want %q", got.Content, p.Content)
	}
	if got.Language != p.Language {
		t.Errorf("Language: got %q, want %q", got.Language, p.Language)
	}
	if got.Visibility != p.Visibility {
		t.Errorf("Visibility: got %d, want %d", got.Visibility, p.Visibility)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

// TestGetPaste_NotFound expects nil,nil for an ID that was never inserted.
func TestGetPaste_NotFound(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetPasteByID("doesnotexist")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil paste, got %+v", got)
	}
}

// TestDeletePaste inserts a paste, deletes it, and confirms it is gone.
func TestDeletePaste(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("del00001")
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}
	if err := db.DeletePaste("del00001"); err != nil {
		t.Fatalf("DeletePaste: %v", err)
	}
	got, err := db.GetPasteByID("del00001")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil after delete, paste still exists")
	}
}

// TestDeletePasteByToken_WrongToken confirms that a mismatched token returns an error.
func TestDeletePasteByToken_WrongToken(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("tok00001")
	p.DeleteTokenHash = "abc"
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}

	err := db.DeletePasteByToken("tok00001", "def")
	if err == nil {
		t.Fatal("expected error with wrong token, got nil")
	}
}

// TestDeletePasteByToken_Correct confirms deletion succeeds with the correct hash.
func TestDeletePasteByToken_Correct(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("tok00002")
	p.DeleteTokenHash = "correcthash"
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}

	if err := db.DeletePasteByToken("tok00002", "correcthash"); err != nil {
		t.Fatalf("DeletePasteByToken with correct token: %v", err)
	}

	got, _ := db.GetPasteByID("tok00002")
	if got != nil {
		t.Error("paste should be gone after token deletion")
	}
}

// TestDeleteExpiredPastes inserts a paste with an expiry in the past and verifies
// DeleteExpiredPastes removes it.
func TestDeleteExpiredPastes(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("exp00001")
	past := time.Now().Add(-time.Hour)
	p.ExpiresAt = &past
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}

	n, err := db.DeleteExpiredPastes()
	if err != nil {
		t.Fatalf("DeleteExpiredPastes: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}

	got, _ := db.GetPasteByID("exp00001")
	if got != nil {
		t.Error("expired paste should be gone")
	}
}

// TestDeleteBurnedPastes inserts a paste that has reached its burn limit and verifies
// DeleteBurnedPastes removes it.
func TestDeleteBurnedPastes(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("burn0001")
	p.BurnAfter = 2
	p.Views = 2
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}

	n, err := db.DeleteBurnedPastes()
	if err != nil {
		t.Fatalf("DeleteBurnedPastes: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 deleted, got %d", n)
	}

	got, _ := db.GetPasteByID("burn0001")
	if got != nil {
		t.Error("burned paste should be gone")
	}
}

// TestGetPublicPastes inserts 3 public pastes and confirms all are returned on page 1.
func TestGetPublicPastes(t *testing.T) {
	db := newTestDB(t)

	ids := []string{"pub00001", "pub00002", "pub00003"}
	for _, id := range ids {
		p := samplePaste(id)
		p.Visibility = model.VisibilityPublic
		if err := db.CreatePaste(p); err != nil {
			t.Fatalf("CreatePaste %s: %v", id, err)
		}
	}

	items, total, err := db.GetPublicPastes(1, 10)
	if err != nil {
		t.Fatalf("GetPublicPastes: %v", err)
	}
	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}
	if len(items) != 3 {
		t.Errorf("len(items): got %d, want 3", len(items))
	}
}

// TestIncrementViews verifies the view counter increments correctly.
func TestIncrementViews(t *testing.T) {
	db := newTestDB(t)

	p := samplePaste("views001")
	if err := db.CreatePaste(p); err != nil {
		t.Fatal(err)
	}

	if err := db.IncrementPasteViews("views001"); err != nil {
		t.Fatalf("IncrementPasteViews: %v", err)
	}

	got, err := db.GetPasteByID("views001")
	if err != nil || got == nil {
		t.Fatalf("GetPasteByID after increment: %v / nil=%v", err, got == nil)
	}
	if got.Views != 1 {
		t.Errorf("Views: got %d, want 1", got.Views)
	}
}

// ─── Scheduler ────────────────────────────────────────────────────────────────

// TestSchedulerUpsertAndGet upserts a TaskState and retrieves it, verifying fields.
func TestSchedulerUpsertAndGet(t *testing.T) {
	db := newTestDB(t)

	now := time.Now().Truncate(time.Second)
	ts := &database.TaskState{
		TaskID:     "task-001",
		TaskName:   "ssl_renewal",
		Schedule:   "0 3 * * *",
		LastRun:    now,
		LastStatus: "success",
		LastError:  "",
		NextRun:    now.Add(24 * time.Hour),
		RunCount:   5,
		FailCount:  1,
		Enabled:    true,
	}

	if err := db.UpsertSchedulerTask(ts); err != nil {
		t.Fatalf("UpsertSchedulerTask: %v", err)
	}

	got, err := db.GetSchedulerTask("task-001")
	if err != nil {
		t.Fatalf("GetSchedulerTask: %v", err)
	}
	if got == nil {
		t.Fatal("GetSchedulerTask: got nil")
	}
	if got.TaskID != ts.TaskID {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, ts.TaskID)
	}
	if got.TaskName != ts.TaskName {
		t.Errorf("TaskName: got %q, want %q", got.TaskName, ts.TaskName)
	}
	if got.Schedule != ts.Schedule {
		t.Errorf("Schedule: got %q, want %q", got.Schedule, ts.Schedule)
	}
	if got.LastStatus != ts.LastStatus {
		t.Errorf("LastStatus: got %q, want %q", got.LastStatus, ts.LastStatus)
	}
	if got.Enabled != ts.Enabled {
		t.Errorf("Enabled: got %v, want %v", got.Enabled, ts.Enabled)
	}
}

// TestSchedulerListTasks upserts 2 tasks and verifies ListSchedulerTasks returns both.
func TestSchedulerListTasks(t *testing.T) {
	db := newTestDB(t)

	for _, id := range []string{"task-a", "task-b"} {
		ts := &database.TaskState{
			TaskID:     id,
			TaskName:   id,
			Schedule:   "* * * * *",
			LastStatus: "pending",
			Enabled:    true,
		}
		if err := db.UpsertSchedulerTask(ts); err != nil {
			t.Fatalf("UpsertSchedulerTask %s: %v", id, err)
		}
	}

	list, err := db.ListSchedulerTasks()
	if err != nil {
		t.Fatalf("ListSchedulerTasks: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len: got %d, want 2", len(list))
	}
}

// TestUpdateTaskRun upserts a task then updates its run metadata, verifying retrieval.
func TestUpdateTaskRun(t *testing.T) {
	db := newTestDB(t)

	ts := &database.TaskState{
		TaskID:     "task-upd",
		TaskName:   "token_cleanup",
		Schedule:   "0 * * * *",
		LastStatus: "pending",
		Enabled:    true,
	}
	if err := db.UpsertSchedulerTask(ts); err != nil {
		t.Fatal(err)
	}

	ran := time.Now().Truncate(time.Second)
	next := ran.Add(time.Hour)
	if err := db.UpdateTaskRun("task-upd", ran, "success", "", 1, 0, next); err != nil {
		t.Fatalf("UpdateTaskRun: %v", err)
	}

	got, err := db.GetSchedulerTask("task-upd")
	if err != nil || got == nil {
		t.Fatalf("GetSchedulerTask: %v / nil=%v", err, got == nil)
	}
	if got.LastStatus != "success" {
		t.Errorf("LastStatus: got %q, want %q", got.LastStatus, "success")
	}
	if got.RunCount != 1 {
		t.Errorf("RunCount: got %d, want 1", got.RunCount)
	}
	if got.FailCount != 0 {
		t.Errorf("FailCount: got %d, want 0", got.FailCount)
	}
}

// TestRecordTaskHistory verifies that RecordTaskHistory does not return an error.
// We trust the underlying INSERT; a separate query would just re-test the driver.
func TestRecordTaskHistory(t *testing.T) {
	db := newTestDB(t)

	// Upsert the parent task first to satisfy any FK constraints.
	ts := &database.TaskState{
		TaskID:     "task-hist",
		TaskName:   "healthcheck_self",
		Schedule:   "*/5 * * * *",
		LastStatus: "pending",
		Enabled:    true,
	}
	if err := db.UpsertSchedulerTask(ts); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	h := &database.TaskHistory{
		TaskID:     "task-hist",
		StartedAt:  now,
		FinishedAt: now.Add(200 * time.Millisecond),
		Status:     "success",
		ErrorMsg:   "",
		DurationMS: 200,
	}
	if err := db.RecordTaskHistory(h); err != nil {
		t.Fatalf("RecordTaskHistory: %v", err)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// makeToken generates a random 32-byte token, returning the base64url-encoded
// raw string, its SHA-256 hash as [32]byte, the hex digest, and the 12-char prefix.
func makeToken(t *testing.T) (raw string, hashBytes [32]byte, hashHex string, prefix string) {
	t.Helper()
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		t.Fatal(err)
	}
	raw = base64.URLEncoding.EncodeToString(b)
	hashBytes = sha256.Sum256([]byte(raw))
	hashHex = hex.EncodeToString(hashBytes[:])
	prefix = raw[:12]
	return raw, hashBytes, hashHex, prefix
}

// minimalTask returns a TaskState suitable for prerequisite upserts.
func minimalTask(id string) *database.TaskState {
	return &database.TaskState{
		TaskID:     id,
		TaskName:   id,
		Schedule:   "* * * * *",
		LastStatus: "pending",
		Enabled:    true,
	}
}

// ─── Type and Ping ────────────────────────────────────────────────────────────

// TestDB_TypeAndPing verifies Type returns "sqlite" and Ping returns nil.
func TestDB_TypeAndPing(t *testing.T) {
	db := newTestDB(t)

	cases := []struct {
		name string
		fn   func() error
	}{
		{"Type", func() error {
			if got := db.Type(); got != "sqlite" {
				t.Errorf("Type: got %q, want %q", got, "sqlite")
			}
			return nil
		}},
		{"Ping", func() error { return db.Ping() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
		})
	}
}

// ─── CountPastes ──────────────────────────────────────────────────────────────

// TestCountPastes verifies that CountPastes returns 0 on an empty DB and 1 after insert.
func TestCountPastes(t *testing.T) {
	db := newTestDB(t)

	cases := []struct {
		name  string
		setup func()
		want  int64
	}{
		{"empty", func() {}, 0},
		{"after_insert", func() {
			if err := db.CreatePaste(samplePaste("cnt00001")); err != nil {
				t.Fatal(err)
			}
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			got, err := db.CountPastes()
			if err != nil {
				t.Fatalf("CountPastes: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// ─── SetTaskEnabled ───────────────────────────────────────────────────────────

// TestSetTaskEnabled upserts a task, disables it, confirms Enabled==false,
// then re-enables it and confirms Enabled==true.
func TestSetTaskEnabled(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertSchedulerTask(minimalTask("task-en")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		enabled bool
	}{
		{"disable", false},
		{"re_enable", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.SetTaskEnabled("task-en", tc.enabled); err != nil {
				t.Fatalf("SetTaskEnabled(%v): %v", tc.enabled, err)
			}
			got, err := db.GetSchedulerTask("task-en")
			if err != nil || got == nil {
				t.Fatalf("GetSchedulerTask: %v / nil=%v", err, got == nil)
			}
			if got.Enabled != tc.enabled {
				t.Errorf("Enabled: got %v, want %v", got.Enabled, tc.enabled)
			}
		})
	}
}

// ─── ListTaskHistory ──────────────────────────────────────────────────────────

// TestListTaskHistory_DefaultLimit inserts 25 history entries, then verifies that
// a limit of 0 returns 20 (the default) and a limit of 5 returns exactly 5.
func TestListTaskHistory_DefaultLimit(t *testing.T) {
	db := newTestDB(t)

	if err := db.UpsertSchedulerTask(minimalTask("task-lh")); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for i := 0; i < 25; i++ {
		h := &database.TaskHistory{
			TaskID:     "task-lh",
			StartedAt:  now.Add(time.Duration(i) * time.Second),
			FinishedAt: now.Add(time.Duration(i)*time.Second + 100*time.Millisecond),
			Status:     "success",
			DurationMS: 100,
		}
		if err := db.RecordTaskHistory(h); err != nil {
			t.Fatalf("RecordTaskHistory #%d: %v", i, err)
		}
	}

	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{"default_zero_gives_20", 0, 20},
		{"explicit_5", 5, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.ListTaskHistory("task-lh", tc.limit)
			if err != nil {
				t.Fatalf("ListTaskHistory(limit=%d): %v", tc.limit, err)
			}
			if len(got) != tc.want {
				t.Errorf("len: got %d, want %d", len(got), tc.want)
			}
		})
	}
}

// ─── EnsureAppSecret ──────────────────────────────────────────────────────────

// TestEnsureAppSecret_Idempotent verifies that EnsureAppSecret returns 32 bytes
// on first call and the identical bytes on a second call for the same key.
func TestEnsureAppSecret_Idempotent(t *testing.T) {
	db := newTestDB(t)

	first, err := db.EnsureAppSecret("test-secret")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(first) != 32 {
		t.Errorf("first call: got %d bytes, want 32", len(first))
	}

	second, err := db.EnsureAppSecret("test-secret")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(first) != string(second) {
		t.Error("second call returned different bytes than the first")
	}
}

// TestEnsureAppSecret_DifferentKeys verifies that two different keys each return
// their own independent 32-byte secret.
func TestEnsureAppSecret_DifferentKeys(t *testing.T) {
	db := newTestDB(t)

	a, err := db.EnsureAppSecret("key-alpha")
	if err != nil {
		t.Fatalf("key-alpha: %v", err)
	}
	b, err := db.EnsureAppSecret("key-beta")
	if err != nil {
		t.Fatalf("key-beta: %v", err)
	}
	if string(a) == string(b) {
		t.Error("different keys unexpectedly returned identical secrets")
	}
}

// ─── API Tokens ───────────────────────────────────────────────────────────────

// TestCreateAndVerifyAPIToken creates a token then verifies it succeeds.
func TestCreateAndVerifyAPIToken(t *testing.T) {
	db := newTestDB(t)

	_, hashBytes, hashHex, prefix := makeToken(t)
	if err := db.CreateAPIToken(hashHex, prefix, "paste", "paste-001", nil); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if err := db.VerifyAPIToken(hashBytes, "paste", "paste-001"); err != nil {
		t.Errorf("VerifyAPIToken: unexpected error: %v", err)
	}
}

// TestVerifyAPIToken_WrongResource verifies that VerifyAPIToken returns an error
// when the resource ID does not match the stored row.
func TestVerifyAPIToken_WrongResource(t *testing.T) {
	db := newTestDB(t)

	_, hashBytes, hashHex, prefix := makeToken(t)
	if err := db.CreateAPIToken(hashHex, prefix, "paste", "paste-001", nil); err != nil {
		t.Fatal(err)
	}
	err := db.VerifyAPIToken(hashBytes, "paste", "paste-WRONG")
	if err == nil {
		t.Error("expected error for wrong resourceID, got nil")
	}
}

// TestValidateAPIToken_Valid creates a token, then validates it by resource type only.
func TestValidateAPIToken_Valid(t *testing.T) {
	db := newTestDB(t)

	_, hashBytes, hashHex, prefix := makeToken(t)
	if err := db.CreateAPIToken(hashHex, prefix, "paste", "paste-001", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.ValidateAPIToken(hashBytes, "paste"); err != nil {
		t.Errorf("ValidateAPIToken: unexpected error: %v", err)
	}
}

// TestValidateAPIToken_Unknown confirms ValidateAPIToken returns an error when no
// matching token exists.
func TestValidateAPIToken_Unknown(t *testing.T) {
	db := newTestDB(t)

	_, hashBytes, _, _ := makeToken(t)
	if err := db.ValidateAPIToken(hashBytes, "paste"); err == nil {
		t.Error("expected error for unknown token, got nil")
	}
}

// TestRevokeAPIToken verifies that a token can be revoked once, and that a second
// revocation attempt on the same prefix returns an error.
func TestRevokeAPIToken(t *testing.T) {
	db := newTestDB(t)

	_, _, hashHex, prefix := makeToken(t)
	if err := db.CreateAPIToken(hashHex, prefix, "paste", "paste-001", nil); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		wantErr bool
	}{
		{"first_revoke", false},
		{"second_revoke_already_revoked", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := db.RevokeAPIToken(prefix, "test reason")
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestListAPITokens creates 2 tokens, revokes one, and asserts only 1 is returned.
func TestListAPITokens(t *testing.T) {
	db := newTestDB(t)

	_, _, hashHex1, prefix1 := makeToken(t)
	_, _, hashHex2, prefix2 := makeToken(t)

	if err := db.CreateAPIToken(hashHex1, prefix1, "paste", "paste-001", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAPIToken(hashHex2, prefix2, "paste", "paste-002", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeAPIToken(prefix1, "cleanup"); err != nil {
		t.Fatal(err)
	}

	tokens, err := db.ListAPITokens()
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("len: got %d, want 1", len(tokens))
	}
	if len(tokens) == 1 && tokens[0].TokenPrefix != prefix2 {
		t.Errorf("remaining token prefix: got %q, want %q", tokens[0].TokenPrefix, prefix2)
	}
}

// TestDeleteExpiredAPITokens creates a token with an expiry in the past and
// verifies DeleteExpiredAPITokens removes it and returns a count of 1.
func TestDeleteExpiredAPITokens(t *testing.T) {
	db := newTestDB(t)

	_, _, hashHex, prefix := makeToken(t)
	past := time.Now().Add(-time.Hour)
	if err := db.CreateAPIToken(hashHex, prefix, "paste", "paste-001", &past); err != nil {
		t.Fatal(err)
	}

	n, err := db.DeleteExpiredAPITokens()
	if err != nil {
		t.Fatalf("DeleteExpiredAPITokens: %v", err)
	}
	if n != 1 {
		t.Errorf("rows deleted: got %d, want 1", n)
	}
}

// ─── NewDatabase default type ─────────────────────────────────────────────────

// TestNewDatabase_DefaultType verifies that passing an empty type string to
// NewDatabase falls back to SQLite and returns a working DB.
func TestNewDatabase_DefaultType(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewDatabase("", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDatabase(\"\", ...): %v", err)
	}
	if db == nil {
		t.Fatal("NewDatabase returned nil")
	}
	t.Cleanup(func() { db.Close() })

	if got := db.Type(); got != "sqlite" {
		t.Errorf("Type: got %q, want %q", got, "sqlite")
	}
	if err := db.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}
