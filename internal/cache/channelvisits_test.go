package cache

import (
	"testing"
	"time"
)

func TestRecordAndGetChannelVisit(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	if err := db.RecordChannelVisit("T1", "C1", now); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := db.RecordChannelVisit("T1", "C2", now+1); err != nil {
		t.Fatalf("record: %v", err)
	}

	visits, err := db.GetChannelVisits("T1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(visits) != 2 {
		t.Fatalf("want 2 entries, got %d", len(visits))
	}
	if visits["C1"] == 0 || visits["C2"] == 0 {
		t.Fatalf("expected non-zero last_visited, got %+v", visits)
	}
}

// TestRecordChannelVisitPreservesCallerOrder verifies that consecutive
// visits within the same millisecond keep the caller-supplied ordering
// when distinct timestamps are passed. This is the property the restore
// path relies on so the last viewed channel is unambiguous even when
// async DB writes complete out of order.
func TestRecordChannelVisitPreservesCallerOrder(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	if err := db.RecordChannelVisit("T1", "C1", 100); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := db.RecordChannelVisit("T1", "C1", 200); err != nil {
		t.Fatalf("record: %v", err)
	}
	visits, _ := db.GetChannelVisits("T1")
	if visits["C1"] != 200 {
		t.Fatalf("expected later caller timestamp to win; got %d", visits["C1"])
	}
}

// TestRecordChannelVisitIsMonotonic verifies that an older timestamp
// landing after a newer one cannot roll the stored value back. Without
// this guarantee, two goroutines racing into SQLite for the same channel
// can leave the row stamped with the earlier visit even though the user
// actually selected it later -- breaking both finder recency and
// restart restoration.
func TestRecordChannelVisitIsMonotonic(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	if err := db.RecordChannelVisit("T1", "C1", 200); err != nil {
		t.Fatalf("record newer: %v", err)
	}
	if err := db.RecordChannelVisit("T1", "C1", 100); err != nil {
		t.Fatalf("record older: %v", err)
	}
	visits, _ := db.GetChannelVisits("T1")
	if visits["C1"] != 200 {
		t.Fatalf("expected stored timestamp to stay at the newer value 200; got %d", visits["C1"])
	}
}

func TestGetChannelVisitsIsolatesWorkspaces(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	if err := db.RecordChannelVisit("T1", "C1", now); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := db.RecordChannelVisit("T2", "C2", now); err != nil {
		t.Fatalf("record: %v", err)
	}

	t1, _ := db.GetChannelVisits("T1")
	t2, _ := db.GetChannelVisits("T2")

	if _, ok := t1["C1"]; !ok {
		t.Errorf("expected T1 to contain C1, got %+v", t1)
	}
	if _, ok := t1["C2"]; ok {
		t.Errorf("expected T1 to NOT contain C2, got %+v", t1)
	}
	if _, ok := t2["C2"]; !ok {
		t.Errorf("expected T2 to contain C2, got %+v", t2)
	}
	if _, ok := t2["C1"]; ok {
		t.Errorf("expected T2 to NOT contain C1, got %+v", t2)
	}
}
