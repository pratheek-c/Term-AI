package tuistart_test

import (
	"path/filepath"
	"testing"

	"tui-start/models"
	"tui-start/storage"
)

// newTestStore opens a fresh BoltDB in a temp directory and registers cleanup.
func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := storage.OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ─── Messages ─────────────────────────────────────────────────────────────────

func TestSaveAndLoadMessages_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	msgs := []models.Message{
		models.New(models.RoleUser, "hello"),
		models.New(models.RoleAI, "hi there"),
	}
	if err := s.SaveMessages(msgs); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}
	got, err := s.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(got) != len(msgs) {
		t.Fatalf("len(LoadMessages) = %d, want %d", len(got), len(msgs))
	}
	for i, m := range msgs {
		if got[i].Role != m.Role || got[i].Content != m.Content {
			t.Errorf("[%d] got {%v,%q}, want {%v,%q}", i, got[i].Role, got[i].Content, m.Role, m.Content)
		}
	}
}

func TestSaveMessages_ReplacesExisting(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveMessages([]models.Message{models.New(models.RoleUser, "old")})
	newMsgs := []models.Message{models.New(models.RoleAI, "new")}
	if err := s.SaveMessages(newMsgs); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}
	got, _ := s.LoadMessages()
	if len(got) != 1 || got[0].Content != "new" {
		t.Errorf("expected [new], got %v", got)
	}
}

func TestLoadMessages_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestAppendMessage(t *testing.T) {
	s := newTestStore(t)
	m1 := models.New(models.RoleUser, "first")
	m2 := models.New(models.RoleAI, "second")
	if err := s.AppendMessage(m1); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := s.AppendMessage(m2); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	got, _ := s.LoadMessages()
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("unexpected contents: %v", got)
	}
}

// ─── History ──────────────────────────────────────────────────────────────────

func TestSaveAndLoadHistory_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	h := []string{"ls", "pwd", "git status"}
	if err := s.SaveHistory(h); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}
	got, err := s.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(got) != len(h) {
		t.Fatalf("len = %d, want %d", len(got), len(h))
	}
	for i := range h {
		if got[i] != h[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], h[i])
		}
	}
}

func TestSaveHistory_ReplacesExisting(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveHistory([]string{"old"})
	if err := s.SaveHistory([]string{"new1", "new2"}); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}
	got, _ := s.LoadHistory()
	if len(got) != 2 || got[0] != "new1" {
		t.Errorf("unexpected history: %v", got)
	}
}

func TestAppendHistory(t *testing.T) {
	s := newTestStore(t)
	_ = s.AppendHistory("cmd1")
	_ = s.AppendHistory("cmd2")
	got, _ := s.LoadHistory()
	if len(got) != 2 || got[0] != "cmd1" || got[1] != "cmd2" {
		t.Errorf("unexpected history: %v", got)
	}
}

// ─── Config: ThemeIdx ─────────────────────────────────────────────────────────

func TestSaveAndLoadThemeIdx(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveThemeIdx(3); err != nil {
		t.Fatalf("SaveThemeIdx: %v", err)
	}
	got, err := s.LoadThemeIdx()
	if err != nil {
		t.Fatalf("LoadThemeIdx: %v", err)
	}
	if got != 3 {
		t.Errorf("LoadThemeIdx = %d, want 3", got)
	}
}

func TestLoadThemeIdx_DefaultZero(t *testing.T) {
	s := newTestStore(t)
	got, err := s.LoadThemeIdx()
	if err != nil {
		t.Fatalf("LoadThemeIdx: %v", err)
	}
	if got != 0 {
		t.Errorf("default ThemeIdx = %d, want 0", got)
	}
}

// ─── Config: Cwd ──────────────────────────────────────────────────────────────

func TestSaveAndLoadCwd(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveCwd("/home/user/projects"); err != nil {
		t.Fatalf("SaveCwd: %v", err)
	}
	got, err := s.LoadCwd()
	if err != nil {
		t.Fatalf("LoadCwd: %v", err)
	}
	if got != "/home/user/projects" {
		t.Errorf("LoadCwd = %q, want %q", got, "/home/user/projects")
	}
}

func TestLoadCwd_DefaultEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.LoadCwd()
	if err != nil {
		t.Fatalf("LoadCwd: %v", err)
	}
	if got != "" {
		t.Errorf("default cwd = %q, want \"\"", got)
	}
}

// ─── Sessions CRUD ────────────────────────────────────────────────────────────

func TestCreateAndListSessions(t *testing.T) {
	s := newTestStore(t)
	meta, err := s.CreateSession("session one")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if meta.ID == "" {
		t.Error("expected non-empty ID")
	}
	if meta.Name != "session one" {
		t.Errorf("Name = %q, want %q", meta.Name, "session one")
	}

	list, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 1 || list[0].ID != meta.ID {
		t.Errorf("unexpected sessions: %v", list)
	}
}

func TestRenameSession(t *testing.T) {
	s := newTestStore(t)
	meta, _ := s.CreateSession("old name")
	if err := s.RenameSession(meta.ID, "new name"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	list, _ := s.ListSessions()
	if len(list) != 1 || list[0].Name != "new name" {
		t.Errorf("unexpected sessions after rename: %v", list)
	}
}

func TestRenameSession_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.RenameSession("9999", "x")
	if err == nil {
		t.Error("expected error renaming non-existent session")
	}
}

func TestDeleteSession(t *testing.T) {
	s := newTestStore(t)
	meta, _ := s.CreateSession("to delete")
	if err := s.DeleteSession(meta.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	list, _ := s.ListSessions()
	if len(list) != 0 {
		t.Errorf("expected empty list after delete, got %v", list)
	}
}

// ─── Session messages ─────────────────────────────────────────────────────────

func TestAppendAndLoadSessionMessages(t *testing.T) {
	s := newTestStore(t)
	meta, _ := s.CreateSession("chat")
	m1 := models.New(models.RoleUser, "hi")
	m2 := models.New(models.RoleAI, "hello")
	_ = s.AppendSessionMessage(meta.ID, m1)
	_ = s.AppendSessionMessage(meta.ID, m2)

	got, err := s.LoadSessionMessages(meta.ID)
	if err != nil {
		t.Fatalf("LoadSessionMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Content != "hi" || got[1].Content != "hello" {
		t.Errorf("unexpected messages: %v", got)
	}
}

func TestLoadSessionMessages_Empty(t *testing.T) {
	s := newTestStore(t)
	meta, _ := s.CreateSession("empty")
	got, err := s.LoadSessionMessages(meta.ID)
	if err != nil {
		t.Fatalf("LoadSessionMessages: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestClearSessionMessages(t *testing.T) {
	s := newTestStore(t)
	meta, _ := s.CreateSession("chat")
	_ = s.AppendSessionMessage(meta.ID, models.New(models.RoleUser, "msg"))
	if err := s.ClearSessionMessages(meta.ID); err != nil {
		t.Fatalf("ClearSessionMessages: %v", err)
	}
	got, _ := s.LoadSessionMessages(meta.ID)
	if len(got) != 0 {
		t.Errorf("expected empty after clear, got %v", got)
	}
}

// ─── Active session ───────────────────────────────────────────────────────────

func TestSaveAndLoadActiveSession(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveActiveSession("42"); err != nil {
		t.Fatalf("SaveActiveSession: %v", err)
	}
	got, err := s.LoadActiveSession()
	if err != nil {
		t.Fatalf("LoadActiveSession: %v", err)
	}
	if got != "42" {
		t.Errorf("LoadActiveSession = %q, want %q", got, "42")
	}
}

func TestLoadActiveSession_DefaultEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.LoadActiveSession()
	if err != nil {
		t.Fatalf("LoadActiveSession: %v", err)
	}
	if got != "" {
		t.Errorf("default active session = %q, want \"\"", got)
	}
}
