// Package models defines shared data types for the AI Shell TUI.
package models

import "time"

// Role identifies who authored a message in the conversation history.
type Role int

const (
	RoleSystem   Role = iota // informational text from the app itself
	RoleUser                 // text the user typed in AI mode
	RoleAI                   // reply from the AI agent
	RoleShellCmd             // shell command entered by the user
	RoleShellOut             // stdout of a shell command
	RoleError                // stderr / error output
	RoleAutoTask             // top-level task entered in auto mode
	RoleAutoStep             // a step planned/announced by the auto agent
	RoleAutoOut              // output captured during an auto step
	RoleAutoDone             // final summary emitted by the auto agent
)

// Message is a single entry in the conversation history.
type Message struct {
	Role      Role
	Content   string
	Timestamp time.Time
}

// New creates a Message with the current time.
func New(role Role, content string) Message {
	return Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}
