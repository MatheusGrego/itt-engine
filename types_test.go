package itt

import (
	"testing"
	"time"
)

func TestEventValidation_EmptySource(t *testing.T) {
	e := Event{Source: "", Target: "b"}
	if err := e.Validate(); err != ErrEmptySource {
		t.Fatalf("expected ErrEmptySource, got %v", err)
	}
}

func TestEventValidation_EmptyTarget(t *testing.T) {
	e := Event{Source: "a", Target: ""}
	if err := e.Validate(); err != ErrEmptyTarget {
		t.Fatalf("expected ErrEmptyTarget, got %v", err)
	}
}

func TestEventValidation_NegativeWeight(t *testing.T) {
	e := Event{Source: "a", Target: "b", Weight: -1}
	if err := e.Validate(); err != ErrNegativeWeight {
		t.Fatalf("expected ErrNegativeWeight, got %v", err)
	}
}

func TestEventValidation_Valid(t *testing.T) {
	e := Event{Source: "a", Target: "b", Weight: 1.0}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEventValidation_ZeroWeightIsValid(t *testing.T) {
	e := Event{Source: "a", Target: "b", Weight: 0}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEventNormalize_Defaults(t *testing.T) {
	e := Event{Source: "a", Target: "b"}
	n := e.Normalize()
	if n.Weight != 1.0 {
		t.Fatalf("expected weight 1.0, got %f", n.Weight)
	}
	if n.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestEventNormalize_PreservesExplicit(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Event{Source: "a", Target: "b", Weight: 5.0, Timestamp: ts}
	n := e.Normalize()
	if n.Weight != 5.0 {
		t.Fatalf("expected weight 5.0, got %f", n.Weight)
	}
	if !n.Timestamp.Equal(ts) {
		t.Fatalf("expected preserved timestamp")
	}
}
