package itt

import (
	"math"
	"testing"
)

func TestAggMean(t *testing.T) {
	tests := []struct {
		input    []float64
		expected float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3, 4, 5}, 3},
		{[]float64{0, 10}, 5},
	}
	for _, tt := range tests {
		got := AggMean(tt.input)
		if math.Abs(got-tt.expected) > 1e-10 {
			t.Errorf("AggMean(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestAggMax(t *testing.T) {
	tests := []struct {
		input    []float64
		expected float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 3, 2}, 3},
		{[]float64{-1, -5, -2}, -1},
	}
	for _, tt := range tests {
		got := AggMax(tt.input)
		if math.Abs(got-tt.expected) > 1e-10 {
			t.Errorf("AggMax(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestAggMedian(t *testing.T) {
	tests := []struct {
		input    []float64
		expected float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 3}, 2},
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
	}
	for _, tt := range tests {
		got := AggMedian(tt.input)
		if math.Abs(got-tt.expected) > 1e-10 {
			t.Errorf("AggMedian(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestAggSum(t *testing.T) {
	tests := []struct {
		input    []float64
		expected float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3}, 6},
	}
	for _, tt := range tests {
		got := AggSum(tt.input)
		if math.Abs(got-tt.expected) > 1e-10 {
			t.Errorf("AggSum(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}
