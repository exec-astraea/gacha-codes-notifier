package main

import "testing"

// The retry cap relies on recordAttempt counting failed posts per code and mark
// clearing that counter, so a code is retried a bounded number of times and a
// fresh code gets its own budget.
func TestRecordAttemptAndMark(t *testing.T) {
	s := &State{
		Sent:     map[string]map[string]bool{},
		Attempts: map[string]map[string]int{},
	}

	if got := s.recordAttempt("genshin", "ABC"); got != 1 {
		t.Fatalf("first attempt = %d, want 1", got)
	}
	if got := s.recordAttempt("genshin", "ABC"); got != 2 {
		t.Fatalf("second attempt = %d, want 2", got)
	}

	// A different code keeps its own independent count.
	if got := s.recordAttempt("genshin", "XYZ"); got != 1 {
		t.Fatalf("other code attempt = %d, want 1", got)
	}

	// Marking a code done clears its counter (and drops the empty game map).
	s.mark("genshin", "ABC")
	if !s.sent("genshin", "ABC") {
		t.Fatal("code should be sent after mark")
	}
	if got := s.recordAttempt("genshin", "ABC"); got != 1 {
		t.Fatalf("attempt after mark did not reset: %d, want 1", got)
	}
}

// recordAttempt must not panic on a zero-value State (nil Attempts map).
func TestRecordAttemptNilMap(t *testing.T) {
	s := &State{Sent: map[string]map[string]bool{}}
	if got := s.recordAttempt("hsr", "CODE"); got != 1 {
		t.Fatalf("attempt on nil map = %d, want 1", got)
	}
}
