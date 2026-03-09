// Package storage provides BoltDB-backed persistence for the AI Shell TUI.
// It saves and restores: conversation messages, command history, working
// directory, and the active theme index.
package storage

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "github.com/boltdb/bolt"

	"tui-start/models"
)

// ─── Bucket names ─────────────────────────────────────────────────────────────

var (
	bucketMessages = []byte("messages") // stores serialised Message records
	bucketHistory  = []byte("history")  // stores command history strings (ordered)
	bucketConfig   = []byte("config")   // stores scalar config values
)

// config keys
var (
	keyThemeIdx = []byte("theme_idx")
	keyCwd      = []byte("cwd")
)

// ─── Wire format ──────────────────────────────────────────────────────────────

// wireMessage is the JSON representation stored in BoltDB.
type wireMessage struct {
	Role      int    `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"ts"` // Unix nano
}

func toWire(m models.Message) wireMessage {
	return wireMessage{
		Role:      int(m.Role),
		Content:   m.Content,
		Timestamp: m.Timestamp.UnixNano(),
	}
}

func fromWire(w wireMessage) models.Message {
	return models.Message{
		Role:      models.Role(w.Role),
		Content:   w.Content,
		Timestamp: time.Unix(0, w.Timestamp),
	}
}

// ─── Store ────────────────────────────────────────────────────────────────────

// Store wraps a BoltDB database with typed helpers.
type Store struct {
	db *bolt.DB
}

// dbPath returns ~/.config/ai-shell/session.db (creates dirs as needed).
func dbPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(cfgDir, "ai-shell")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return filepath.Join(dir, "session.db"), nil
}

// Open opens (or creates) the BoltDB store and initialises all buckets.
func Open() (*Store, error) {
	path, err := dbPath()
	if err != nil {
		return nil, err
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}

	// Create buckets if they don't exist
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketMessages, bucketHistory, bucketConfig, bucketSessions} {
			if _, e := tx.CreateBucketIfNotExists(b); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying BoltDB file.
func (s *Store) Close() error { return s.db.Close() }

// ─── Messages ─────────────────────────────────────────────────────────────────

// SaveMessages replaces all stored messages with the provided slice.
func (s *Store) SaveMessages(msgs []models.Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMessages)
		// Clear existing records
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		// Write new records; key = 8-byte big-endian sequence number
		for i, msg := range msgs {
			key := itob(uint64(i))
			val, err := json.Marshal(toWire(msg))
			if err != nil {
				return err
			}
			if err := b.Put(key, val); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadMessages returns all stored messages in insertion order.
func (s *Store) LoadMessages() ([]models.Message, error) {
	var msgs []models.Message
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMessages)
		return b.ForEach(func(_, v []byte) error {
			var w wireMessage
			if err := json.Unmarshal(v, &w); err != nil {
				return err
			}
			msgs = append(msgs, fromWire(w))
			return nil
		})
	})
	return msgs, err
}

// AppendMessage appends a single message using the next auto-increment key.
func (s *Store) AppendMessage(msg models.Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMessages)
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		val, err := json.Marshal(toWire(msg))
		if err != nil {
			return err
		}
		return b.Put(itob(seq), val)
	})
}

// ─── History ──────────────────────────────────────────────────────────────────

// SaveHistory replaces all stored history entries.
func (s *Store) SaveHistory(history []string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketHistory)
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		for i, h := range history {
			if err := b.Put(itob(uint64(i)), []byte(h)); err != nil {
				return err
			}
		}
		return nil
	})
}

// LoadHistory returns stored history in insertion order.
func (s *Store) LoadHistory() ([]string, error) {
	var history []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketHistory)
		return b.ForEach(func(_, v []byte) error {
			history = append(history, string(v))
			return nil
		})
	})
	return history, err
}

// AppendHistory appends a single history entry.
func (s *Store) AppendHistory(cmd string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketHistory)
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		return b.Put(itob(seq), []byte(cmd))
	})
}

// ─── Config ───────────────────────────────────────────────────────────────────

// SaveThemeIdx persists the active theme index.
func (s *Store) SaveThemeIdx(idx int) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConfig)
		return b.Put(keyThemeIdx, itob(uint64(idx)))
	})
}

// LoadThemeIdx returns the saved theme index, or 0 if not set.
func (s *Store) LoadThemeIdx() (int, error) {
	var idx int
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketConfig)
		v := b.Get(keyThemeIdx)
		if v != nil {
			idx = int(btoi(v))
		}
		return nil
	})
	return idx, err
}

// SaveCwd persists the working directory.
func (s *Store) SaveCwd(cwd string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketConfig).Put(keyCwd, []byte(cwd))
	})
}

// LoadCwd returns the saved working directory, or "" if not set.
func (s *Store) LoadCwd() (string, error) {
	var cwd string
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketConfig).Get(keyCwd)
		if v != nil {
			cwd = string(v)
		}
		return nil
	})
	return cwd, err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// itob encodes a uint64 as an 8-byte big-endian slice.
func itob(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

// btoi decodes an 8-byte big-endian slice to uint64.
func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

// ─── Session types ────────────────────────────────────────────────────────────

// SessionMeta holds metadata about a named chat session.
type SessionMeta struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Session bucket helpers ───────────────────────────────────────────────────

var (
	bucketSessions   = []byte("sessions")
	keyActiveSession = []byte("active_session")
)

// sessionMsgBucket returns the bolt bucket name for a session's messages.
func sessionMsgBucket(id string) []byte { return []byte("sess_msgs_" + id) }

// ─── Session CRUD ─────────────────────────────────────────────────────────────

// CreateSession creates a new session with the given name and returns its metadata.
func (s *Store) CreateSession(name string) (SessionMeta, error) {
	meta := SessionMeta{Name: name, CreatedAt: time.Now()}
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		meta.ID = fmt.Sprintf("%d", seq)
		// Pre-create the message bucket for this session.
		if _, err := tx.CreateBucketIfNotExists(sessionMsgBucket(meta.ID)); err != nil {
			return err
		}
		val, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return b.Put([]byte(meta.ID), val)
	})
	return meta, err
}

// ListSessions returns all sessions ordered by insertion (oldest first).
func (s *Store) ListSessions() ([]SessionMeta, error) {
	var sessions []SessionMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).ForEach(func(_, v []byte) error {
			var meta SessionMeta
			if err := json.Unmarshal(v, &meta); err != nil {
				return err
			}
			sessions = append(sessions, meta)
			return nil
		})
	})
	return sessions, err
}

// RenameSession updates the display name of an existing session.
func (s *Store) RenameSession(id, name string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		v := b.Get([]byte(id))
		if v == nil {
			return fmt.Errorf("session %s not found", id)
		}
		var meta SessionMeta
		if err := json.Unmarshal(v, &meta); err != nil {
			return err
		}
		meta.Name = name
		val, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), val)
	})
}

// DeleteSession removes a session and all its messages.
func (s *Store) DeleteSession(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketSessions).Delete([]byte(id)); err != nil {
			return err
		}
		// Remove message bucket if it exists.
		msgBkt := sessionMsgBucket(id)
		if tx.Bucket(msgBkt) != nil {
			return tx.DeleteBucket(msgBkt)
		}
		return nil
	})
}

// ─── Session-scoped messages ──────────────────────────────────────────────────

// AppendSessionMessage appends a message to the named session's bucket.
func (s *Store) AppendSessionMessage(sessionID string, msg models.Message) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bktName := sessionMsgBucket(sessionID)
		b := tx.Bucket(bktName)
		if b == nil {
			var err error
			b, err = tx.CreateBucket(bktName)
			if err != nil {
				return err
			}
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		val, err := json.Marshal(toWire(msg))
		if err != nil {
			return err
		}
		return b.Put(itob(seq), val)
	})
}

// LoadSessionMessages returns all messages for a session in insertion order.
func (s *Store) LoadSessionMessages(sessionID string) ([]models.Message, error) {
	var msgs []models.Message
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(sessionMsgBucket(sessionID))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var w wireMessage
			if err := json.Unmarshal(v, &w); err != nil {
				return err
			}
			msgs = append(msgs, fromWire(w))
			return nil
		})
	})
	return msgs, err
}

// ClearSessionMessages deletes all messages for a session.
func (s *Store) ClearSessionMessages(sessionID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(sessionMsgBucket(sessionID))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// ─── Active session ───────────────────────────────────────────────────────────

// SaveActiveSession persists the current active session ID.
func (s *Store) SaveActiveSession(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketConfig).Put(keyActiveSession, []byte(id))
	})
}

// LoadActiveSession returns the saved active session ID, or "" if none.
func (s *Store) LoadActiveSession() (string, error) {
	var id string
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketConfig).Get(keyActiveSession)
		if v != nil {
			id = string(v)
		}
		return nil
	})
	return id, err
}
