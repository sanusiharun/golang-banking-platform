package featureflag

import (
	"fmt"
	"strings"
	"time"
)

// IsWithinOperationHours checks whether the current time (in loc timezone) falls
// within the operational window defined by a "HH.MM-HH.MM" string.
//
// Flipt variant keys don't allow colons, so we use dots as separator.
//
// Example:
//
//	IsWithinOperationHours("07.00-15.00", time.UTC) // true between 07:00–15:00 UTC
//
// Returns (true, nil) if the current time is within the window.
// Returns (false, nil) if outside the window.
// Returns (false, error) if the format is invalid — caller should treat as open.
func IsWithinOperationHours(window string, loc *time.Location) (bool, error) {
	if window == "" {
		return true, nil // empty = no restriction
	}

	if loc == nil {
		loc = time.UTC
	}

	// Support both "07.00-15.00" (Flipt variant key format, dots)
	// and "07:00-15:00" (human-readable, colons) for flexibility.
	normalized := strings.ReplaceAll(window, ".", ":")

	var startH, startM, endH, endM int
	if _, err := fmt.Sscanf(normalized, "%d:%d-%d:%d", &startH, &startM, &endH, &endM); err != nil {
		return false, fmt.Errorf("invalid operation hours format %q — expected HH.MM-HH.MM", window)
	}

	now := time.Now().In(loc)
	nowMins := now.Hour()*60 + now.Minute()
	startMins := startH*60 + startM
	endMins := endH*60 + endM

	return nowMins >= startMins && nowMins < endMins, nil
}
