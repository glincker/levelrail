package main

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// appAction runs the plain call, or the queue-while-offline variant when asked.
func appAction(ctx context.Context, client *apiclient.Client, name, action string, queue bool, plain func() (apiclient.AppResource, error)) (apiclient.AppResource, error) {
	if queue {
		return client.AppActionQueued(ctx, name, action)
	}
	return plain()
}
