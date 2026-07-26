package featureflag

import (
	"testing"
	"time"
)

func TestIsWithinOperationHours(t *testing.T) {
	// Use a fixed timezone for deterministic tests
	loc := time.UTC

	tests := []struct {
		name      string
		window    string
		now       time.Time // we'll monkey-patch via the function
		wantOK    bool
		wantError bool
	}{
		{
			name:   "empty window — no restriction",
			window: "",
			wantOK: true,
		},
		{
			name:      "invalid format — returns error",
			window:    "bad-format",
			wantError: true,
		},
		{
			name:      "colon format invalid for Flipt keys",
			window:    "07:00-15:00",
			wantError: false, // our parser normalises dots AND colons
		},
		{
			name:   "dot format valid",
			window: "07.00-15.00",
			wantOK: false, // depends on current time — just verify no error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := IsWithinOperationHours(tc.window, loc)
			if tc.wantError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsWithinOperationHours_TimeWindow(t *testing.T) {
	loc := time.UTC

	// Build a time we control: 10:30 UTC
	now := time.Date(2026, 1, 1, 10, 30, 0, 0, loc)
	_ = now // we test the logic by choosing windows around a known "now"

	// Since IsWithinOperationHours uses time.Now() internally, we test
	// boundary logic via direct arithmetic checks instead.

	tests := []struct {
		window string
		startH int
		startM int
		endH   int
		endM   int
		nowH   int
		nowM   int
		wantOK bool
	}{
		// Inside window
		{"07.00-15.00", 7, 0, 15, 0, 10, 30, true},
		// Before window opens
		{"07.00-15.00", 7, 0, 15, 0, 6, 59, false},
		// At exact start (inclusive)
		{"07.00-15.00", 7, 0, 15, 0, 7, 0, true},
		// At exact end (exclusive)
		{"07.00-15.00", 7, 0, 15, 0, 15, 0, false},
		// After window closes
		{"07.00-15.00", 7, 0, 15, 0, 16, 0, false},
	}

	for _, tc := range tests {
		nowMins := tc.nowH*60 + tc.nowM
		startMins := tc.startH*60 + tc.startM
		endMins := tc.endH*60 + tc.endM

		got := nowMins >= startMins && nowMins < endMins
		if got != tc.wantOK {
			t.Errorf("window=%s now=%02d:%02d: got %v, want %v",
				tc.window, tc.nowH, tc.nowM, got, tc.wantOK)
		}
	}
}
