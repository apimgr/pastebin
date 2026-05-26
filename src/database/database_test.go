package database_test

// Tests for the SQLiteDB implementation.
// Each test spins up a fresh in-process SQLite database so there are no
// external dependencies and tests are safe to run in parallel.

import (
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
