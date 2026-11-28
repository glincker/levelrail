package database

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

const (
	dynamoDBContainerPort = 8000
	dynamoDBDataPath      = "/home/dynamodblocal/data"
	// dynamoDBUID is the unprivileged user the image runs as.
	dynamoDBUID = 1000
	// dynamoDBHeapShare halves the container limit for the JVM heap, leaving room for metaspace and the SQLite cache.
	dynamoDBHeapShare = 2
)

// dynamoDBCommand runs the engine with a shared database so clients with different dummy keys see the same tables.
// The image entrypoint is java, so this is its argument list.
func dynamoDBCommand(res *store.ServiceResources) []string {
	var cmd []string
	if res != nil && res.MemoryBytes > 0 {
		mb := max(64, int(res.MemoryBytes/(1<<20))/dynamoDBHeapShare)
		cmd = append(cmd, "-Xmx"+strconv.Itoa(mb)+"m")
	}
	return append(cmd, "-jar", "DynamoDBLocal.jar", "-sharedDb", "-dbPath", dynamoDBDataPath)
}

// dynamoDBSpec leaves the image entrypoint alone and drops the SETPCAP grant the other engines need.
func dynamoDBSpec() func(*docker.ContainerSpec) {
	return func(spec *docker.ContainerSpec) {
		spec.CapAdd = nil
	}
}

// prepareDynamoDBVolume chowns a fresh data volume before the first start: a root owned volume makes the engine
// hang requests instead of failing.
func (c *Controller) prepareDynamoDBVolume(ctx context.Context) (reconcile.Result, bool) {
	state, err := c.runtime.InspectByName(ctx, containerName(c.dbName))
	if err != nil || state != nil {
		return reconcile.Result{}, false
	}
	vol := dataVolumeName(c.dbName)
	if err := docker.EnsureVolumeFor(ctx, c.runtime, vol, c.volumeOwner(vol)); err != nil {
		return notReady(reasonVolumeFailed, err), true
	}
	id, err := createCertsHelper(ctx, c.runtime, vol)
	if err != nil {
		return notReady(reasonVolumeFailed, err), true
	}
	defer func() { _ = c.runtime.Remove(context.Background(), id, true) }()
	rc, err := c.runtime.Exec(ctx, id, []string{"chown", "-R", fmt.Sprintf("%d:%d", dynamoDBUID, dynamoDBUID), certsHelperMountPath})
	if err != nil {
		return notReady(reasonVolumeFailed, fmt.Errorf("chown dynamodb volume: %w", err)), true
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return notReady(reasonVolumeFailed, fmt.Errorf("chown dynamodb volume: %w", err)), true
	}
	return reconcile.Result{}, false
}
