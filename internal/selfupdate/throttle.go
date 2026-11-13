package selfupdate

import "time"

// CheckInterval is how often MyPack looks for a new release on its own.
const CheckInterval = 24 * time.Hour

// Due reports whether an automatic check is due at now, the last one
// having been at last (zero: never). A last check in the future, from a
// clock that was set back, does not hold checks off.
func Due(last, now time.Time, interval time.Duration) bool {
	if last.IsZero() || last.After(now) {
		return true
	}
	return now.Sub(last) >= interval
}
