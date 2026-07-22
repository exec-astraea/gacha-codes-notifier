package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State records which codes have already been sent to Discord, per game key.
// Persisted as JSON in the bind-mounted data dir so restarts never re-send.
type State struct {
	path string
	// Sent maps game key -> set of code strings already announced (or, for a
	// game's first-run backlog / a code we gave up retrying, recorded as done).
	Sent map[string]map[string]bool `json:"sent"`
	// Attempts maps game key -> code -> number of failed announce attempts. A
	// code lives here only while it's being retried; it's cleared once the code
	// is marked sent (success or give-up).
	Attempts map[string]map[string]int `json:"attempts,omitempty"`
}

func loadState(path string) (*State, error) {
	s := &State{
		path:     path,
		Sent:     map[string]map[string]bool{},
		Attempts: map[string]map[string]int{},
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil // fresh start
	}
	if err != nil {
		return s, err
	}

	// Tolerate an empty/corrupt file by starting clean rather than crashing.
	var on struct {
		Sent     map[string]map[string]bool `json:"sent"`
		Attempts map[string]map[string]int  `json:"attempts"`
	}
	if err := json.Unmarshal(data, &on); err != nil {
		return s, nil
	}
	if on.Sent != nil {
		s.Sent = on.Sent
	}
	if on.Attempts != nil {
		s.Attempts = on.Attempts
	}
	return s, nil
}

// sent reports whether a code was already announced for a game.
func (s *State) sent(game, code string) bool {
	return s.Sent[game][code]
}

// hasGame reports whether we have ever recorded state for a game (used to
// detect a first run and seed silently).
func (s *State) hasGame(game string) bool {
	_, ok := s.Sent[game]
	return ok
}

// mark records a code as sent for a game and clears any retry counter for it.
func (s *State) mark(game, code string) {
	if s.Sent[game] == nil {
		s.Sent[game] = map[string]bool{}
	}
	s.Sent[game][code] = true
	if s.Attempts[game] != nil {
		delete(s.Attempts[game], code)
		if len(s.Attempts[game]) == 0 {
			delete(s.Attempts, game)
		}
	}
}

// recordAttempt increments and returns the failed-announce attempt count for a
// code. Used to cap retries so a persistently failing post can't retry forever.
func (s *State) recordAttempt(game, code string) int {
	if s.Attempts == nil {
		s.Attempts = map[string]map[string]int{}
	}
	if s.Attempts[game] == nil {
		s.Attempts[game] = map[string]int{}
	}
	s.Attempts[game][code]++
	return s.Attempts[game][code]
}

// seed records that a game has been seen without marking any specific code, so
// an empty first fetch still flips hasGame. Without it, a game that has no
// active codes on its first run stays "first run" and the next fetch that does
// find codes is silently suppressed as backlog instead of announced.
func (s *State) seed(game string) {
	if s.Sent[game] == nil {
		s.Sent[game] = map[string]bool{}
	}
}

// save writes state atomically (temp file + rename).
func (s *State) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Sent     map[string]map[string]bool `json:"sent"`
		Attempts map[string]map[string]int  `json:"attempts,omitempty"`
	}{Sent: s.Sent, Attempts: s.Attempts}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
