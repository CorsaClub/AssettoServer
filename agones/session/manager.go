// Package session manages the lifecycle of game sessions.
package session

import (
	"agones/types"
	"sync"
	"time"
)

// SessionManager manages the lifecycle of sessions.
type SessionManager struct {
	sync.RWMutex
	current     *types.Session         // The current active session
	history     []*types.Session       // History of previous sessions
	maxHistory  int                    // Maximum number of sessions to keep in history
	transitions chan SessionTransition // Channel to handle session transitions

}

// SessionTransition represents a transition between sessions.
type SessionTransition struct {
	From string    // The previous session type
	To   string    // The new session type
	Time time.Time // The time the transition occurred
}

// NewSessionManager creates a new SessionManager with a specified maximum history size.
func NewSessionManager(maxHistory int) *SessionManager {
	return &SessionManager{
		history:     make([]*types.Session, 0, maxHistory),
		maxHistory:  maxHistory,
		transitions: make(chan SessionTransition, 50),
	}
}

// StartNewSession initiates a new session of the given type.
// It archives the current session if one exists and records the transition.
func (sm *SessionManager) StartNewSession(sessionType string) error {
	sm.Lock()
	defer sm.Unlock()

	if sm.current != nil {
		sm.archiveCurrentSession()
	}

	sm.current = &types.Session{
		Type:      sessionType,
		StartTime: time.Now(),
	}

	sm.transitions <- SessionTransition{
		From: "none",
		To:   sessionType,
		Time: time.Now(),
	}

	return nil
}

// archiveCurrentSession archives the current session to the history.
// If the history exceeds maxHistory, it removes the oldest session.
func (sm *SessionManager) archiveCurrentSession() {
	if len(sm.history) >= sm.maxHistory {
		// Remove the oldest session
		sm.history = sm.history[1:]
	}
	sm.current.EndTime = time.Now()
	sm.history = append(sm.history, sm.current)
}

// GetCurrentSession returns the current active session.
func (sm *SessionManager) GetCurrentSession() *types.Session {
	sm.RLock()
	defer sm.RUnlock()
	return sm.current
}

// GetSessionHistory returns a defensive copy of the session history.
func (sm *SessionManager) GetSessionHistory() []*types.Session {
	sm.RLock()
	defer sm.RUnlock()
	return append([]*types.Session{}, sm.history...)
}

// Close closes the session transitions channel.
func (sm *SessionManager) Close() error {
	close(sm.transitions)
	return nil
}
