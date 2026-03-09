package tuistart_test

import (
	"testing"
	"time"

	"tui-start/models"
)

func TestRoleConstants(t *testing.T) {
	if models.RoleSystem != 0 {
		t.Errorf("RoleSystem = %d, want 0", models.RoleSystem)
	}
	if models.RoleUser != 1 {
		t.Errorf("RoleUser = %d, want 1", models.RoleUser)
	}
	if models.RoleAI != 2 {
		t.Errorf("RoleAI = %d, want 2", models.RoleAI)
	}
	if models.RoleShellCmd != 3 {
		t.Errorf("RoleShellCmd = %d, want 3", models.RoleShellCmd)
	}
	if models.RoleShellOut != 4 {
		t.Errorf("RoleShellOut = %d, want 4", models.RoleShellOut)
	}
	if models.RoleError != 5 {
		t.Errorf("RoleError = %d, want 5", models.RoleError)
	}
	if models.RoleAutoTask != 6 {
		t.Errorf("RoleAutoTask = %d, want 6", models.RoleAutoTask)
	}
	if models.RoleAutoStep != 7 {
		t.Errorf("RoleAutoStep = %d, want 7", models.RoleAutoStep)
	}
	if models.RoleAutoOut != 8 {
		t.Errorf("RoleAutoOut = %d, want 8", models.RoleAutoOut)
	}
	if models.RoleAutoDone != 9 {
		t.Errorf("RoleAutoDone = %d, want 9", models.RoleAutoDone)
	}
}

func TestNew_RoleAndContent(t *testing.T) {
	cases := []struct {
		role    models.Role
		content string
	}{
		{models.RoleUser, "hello"},
		{models.RoleAI, "hi there"},
		{models.RoleShellCmd, "ls -la"},
		{models.RoleError, "something went wrong"},
	}
	for _, tc := range cases {
		msg := models.New(tc.role, tc.content)
		if msg.Role != tc.role {
			t.Errorf("New(%v, %q).Role = %v, want %v", tc.role, tc.content, msg.Role, tc.role)
		}
		if msg.Content != tc.content {
			t.Errorf("New(%v, %q).Content = %q, want %q", tc.role, tc.content, msg.Content, tc.content)
		}
	}
}

func TestNew_TimestampIsRecent(t *testing.T) {
	before := time.Now()
	msg := models.New(models.RoleUser, "test")
	after := time.Now()

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Errorf("Timestamp %v is not between %v and %v", msg.Timestamp, before, after)
	}
}

func TestNew_EmptyContent(t *testing.T) {
	msg := models.New(models.RoleSystem, "")
	if msg.Content != "" {
		t.Errorf("expected empty content, got %q", msg.Content)
	}
	if msg.Role != models.RoleSystem {
		t.Errorf("expected RoleSystem, got %v", msg.Role)
	}
}
