package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	userRolePageConcurrency = 4
	userRolePageTimeout     = 15 * time.Second
)

// GetUsersGlobalRoles returns role sets in the same order as userIDs.
func (k *KeycloakService) GetUsersGlobalRoles(ctx context.Context, userIDs []string) (result [][]string, err error) {
	result = make([][]string, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, userRolePageTimeout)
	defer cancel()
	defer func() { k.dropAdminTokenIfRejected(err) }()

	token, err := k.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	clientID, err := k.backendClientID(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("resolve backend client id: %w", err)
	}

	jobs := make(chan int, len(userIDs))
	for i := range userIDs {
		jobs <- i
	}
	close(jobs)

	var workers sync.WaitGroup
	var firstError sync.Once
	var lookupErr error
	for range min(userRolePageConcurrency, len(userIDs)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				roles, err := k.userGlobalRoles(ctx, token, clientID, userIDs[i])
				if err != nil {
					k.dropAdminTokenIfRejected(err)
					firstError.Do(func() {
						lookupErr = err
						cancel()
					})
					return
				}
				result[i] = roles
			}
		})
	}
	workers.Wait()
	if lookupErr != nil {
		return nil, lookupErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
