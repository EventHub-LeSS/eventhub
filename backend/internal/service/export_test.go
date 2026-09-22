package service

import "time"

// SetOrgRoleChangeTimeout overrides orgRoleChangeTimeout for a test and returns a restore func.
func SetOrgRoleChangeTimeout(d time.Duration) (restore func()) {
	previous := orgRoleChangeTimeout
	orgRoleChangeTimeout = d
	return func() { orgRoleChangeTimeout = previous }
}
