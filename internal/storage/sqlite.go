// Package storage provides a SQLite-backed session service that wraps the ADK
// in-memory session service with persistent event storage.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const maxRecentEvents = 20

// SQLiteService wraps the ADK in-memory session service and persists
// conversation events to a SQLite database so they survive restarts.
type SQLiteService struct {
	inner    session.Service
	db       *sql.DB
	mu       sync.Mutex
	sessions map[string]string // "appName:userID" -> sessionID
}

// NewSQLiteService opens (or creates) a SQLite database at dbPath and returns
// a session.Service that persists events across restarts.
func NewSQLiteService(dbPath string) (*SQLiteService, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &SQLiteService{
		inner:    session.InMemoryService(),
		db:       db,
		sessions: make(map[string]string),
	}, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		app_name TEXT NOT NULL,
		user_id TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		event_id TEXT NOT NULL,
		author TEXT NOT NULL,
		role TEXT NOT NULL,
		content_json TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
	`
	_, err := db.Exec(schema)
	return err
}

func sessionKey(appName, userID string) string {
	return appName + ":" + userID
}

// GetOrCreateSession returns an existing session for the user or creates a new
// one, replaying persisted events into the in-memory session.
func (s *SQLiteService) GetOrCreateSession(ctx context.Context, appName, userID string) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(appName, userID)

	if sessionID, ok := s.sessions[key]; ok {
		resp, err := s.inner.Get(ctx, &session.GetRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err == nil {
			return resp.Session, nil
		}
	}

	var sessionID string
	err := s.db.QueryRow(
		"SELECT id FROM sessions WHERE app_name = ? AND user_id = ? ORDER BY updated_at DESC LIMIT 1",
		appName, userID,
	).Scan(&sessionID)

	if err == sql.ErrNoRows {
		return s.createNewSession(ctx, appName, userID, key)
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	return s.restoreSession(ctx, appName, userID, sessionID, key)
}

func (s *SQLiteService) createNewSession(ctx context.Context, appName, userID, key string) (session.Session, error) {
	resp, err := s.inner.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	sid := resp.Session.ID()
	s.sessions[key] = sid

	_, err = s.db.Exec(
		"INSERT INTO sessions (id, app_name, user_id) VALUES (?, ?, ?)",
		sid, appName, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("persist session: %w", err)
	}

	return resp.Session, nil
}

func (s *SQLiteService) restoreSession(ctx context.Context, appName, userID, sessionID, key string) (session.Session, error) {
	resp, err := s.inner.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, fmt.Errorf("recreate session: %w", err)
	}

	s.sessions[key] = sessionID

	rows, err := s.db.Query(
		"SELECT event_id, author, role, content_json FROM events WHERE session_id = ? ORDER BY id DESC LIMIT ?",
		sessionID, maxRecentEvents,
	)
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	defer rows.Close()

	var stored []storedEvent
	for rows.Next() {
		var se storedEvent
		if err := rows.Scan(&se.EventID, &se.Author, &se.Role, &se.ContentJSON); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		stored = append(stored, se)
	}

	// Reverse to chronological order (we queried DESC)
	for i, j := 0, len(stored)-1; i < j; i, j = i+1, j-1 {
		stored[i], stored[j] = stored[j], stored[i]
	}

	for _, se := range stored {
		var content genai.Content
		if err := json.Unmarshal([]byte(se.ContentJSON), &content); err != nil {
			continue
		}

		event := session.NewEvent("restored")
		event.ID = se.EventID
		event.Author = se.Author
		event.Content = &content

		if err := s.inner.AppendEvent(ctx, resp.Session, event); err != nil {
			continue
		}
	}

	return resp.Session, nil
}

type storedEvent struct {
	EventID     string
	Author      string
	Role        string
	ContentJSON string
}

// AppendEvent appends an event to the in-memory session and persists it to SQLite.
func (s *SQLiteService) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if err := s.inner.AppendEvent(ctx, sess, event); err != nil {
		return err
	}

	if event.Partial || event.Content == nil {
		return nil
	}

	contentJSON, err := json.Marshal(event.Content)
	if err != nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = s.db.Exec(
		"INSERT INTO events (session_id, event_id, author, role, content_json) VALUES (?, ?, ?, ?, ?)",
		sess.ID(), event.ID, event.Author, event.Content.Role, string(contentJSON),
	)
	if err != nil {
		return fmt.Errorf("persist event: %w", err)
	}

	_, _ = s.db.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?", time.Now(), sess.ID())
	return nil
}

// DeleteUserSessions removes all sessions and events for a user.
func (s *SQLiteService) DeleteUserSessions(ctx context.Context, appName, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(appName, userID)

	if sessionID, ok := s.sessions[key]; ok {
		_ = s.inner.Delete(ctx, &session.DeleteRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		delete(s.sessions, key)
	}

	_, err := s.db.Exec(
		`DELETE FROM events WHERE session_id IN (SELECT id FROM sessions WHERE app_name = ? AND user_id = ?)`,
		appName, userID,
	)
	if err != nil {
		return fmt.Errorf("delete events: %w", err)
	}

	_, err = s.db.Exec("DELETE FROM sessions WHERE app_name = ? AND user_id = ?", appName, userID)
	if err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}

	return nil
}

// DeleteAllSessions removes all sessions and events from the database and
// replaces the in-memory service with a fresh instance.
func (s *SQLiteService) DeleteAllSessions(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.db.Exec("DELETE FROM events"); err != nil {
		return fmt.Errorf("delete all events: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM sessions"); err != nil {
		return fmt.Errorf("delete all sessions: %w", err)
	}

	s.sessions = make(map[string]string)
	s.inner = session.InMemoryService()
	return nil
}

// InnerService returns the underlying in-memory session.Service for use with
// the ADK runner.
func (s *SQLiteService) InnerService() session.Service {
	return s.inner
}

// Close closes the SQLite database.
func (s *SQLiteService) Close() error {
	return s.db.Close()
}
