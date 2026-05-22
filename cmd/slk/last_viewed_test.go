package main

import "testing"

func TestMostRecentlyVisitedChannelID(t *testing.T) {
	visits := map[string]int64{
		"C1": 10,
		"D1": 30,
		"C2": 20,
	}

	if got := mostRecentlyVisitedChannelID(visits); got != "D1" {
		t.Fatalf("mostRecentlyVisitedChannelID() = %q, want D1", got)
	}
}

func TestMostRecentlyVisitedChannelIDTieBreaksDeterministically(t *testing.T) {
	visits := map[string]int64{
		"C2": 10,
		"C1": 10,
	}

	if got := mostRecentlyVisitedChannelID(visits); got != "C1" {
		t.Fatalf("mostRecentlyVisitedChannelID() = %q, want C1", got)
	}
}

func TestMostRecentlyVisitedChannelIDEmpty(t *testing.T) {
	if got := mostRecentlyVisitedChannelID(nil); got != "" {
		t.Fatalf("mostRecentlyVisitedChannelID(nil) = %q, want empty", got)
	}
}
