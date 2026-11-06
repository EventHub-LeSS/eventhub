package service

import "time"

// SetOrgChangeTimeout overrides orgChangeTimeout for a test and returns a restore func.
func SetOrgChangeTimeout(d time.Duration) (restore func()) {
	previous := orgChangeTimeout
	orgChangeTimeout = d
	return func() { orgChangeTimeout = previous }
}
