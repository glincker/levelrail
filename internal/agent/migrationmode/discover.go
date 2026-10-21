package migrationmode

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
)

// databaseImages maps an image base name to the engine it serves, so databases
// that no platform API lists are still reported.
var databaseImages = map[string]string{
	"postgres": "postgres", "postgis": "postgres", "timescaledb": "postgres", "pgvector": "postgres",
	"mysql": "mysql", "mariadb": "mariadb", "percona": "mysql",
	"mongo": "mongodb", "mongodb": "mongodb",
	"redis": "redis", "valkey": "redis", "keydb": "redis", "dragonfly": "redis",
	"clickhouse": "clickhouse", "clickhouse-server": "clickhouse",
}

// EngineOf returns the database engine and tag for an image reference, or "" when it is not a known database image.
func EngineOf(image string) (engine, version string) {
	ref := image
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	tag := ""
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i+1:]
	}
	base := strings.ToLower(path.Base(ref))
	if e, ok := databaseImages[base]; ok {
		return e, tag
	}
	return "", ""
}

func portStrings(ps []apiPort) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		s := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
		if p.PublicPort != 0 {
			s = fmt.Sprintf("%d->%s", p.PublicPort, s)
		}
		out = append(out, s)
	}
	return out
}

// Discover reads the host's containers, volumes and networks. It never reads
// environment values, only their names.
func Discover(ctx context.Context, d *Docker, requestID string, now time.Time) (*migrationpb.Inventory, error) {
	ver, err := d.version(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := d.containers(ctx)
	if err != nil {
		return nil, err
	}
	vols, err := d.volumes(ctx)
	if err != nil {
		return nil, err
	}
	nets, err := d.networkCount(ctx)
	if err != nil {
		return nil, err
	}
	inv := &migrationpb.Inventory{RequestId: requestID, DockerVersion: ver, NetworkCount: int32(nets), CollectedAt: timestamppb.New(now)} //nolint:gosec // a network count never nears int32 max
	for _, c := range cs {
		keys, err := d.envKeys(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		engine, dbver := EngineOf(c.Image)
		pc := &migrationpb.Container{Id: shortID(c.ID), Name: name, Image: c.Image, State: c.State, Status: c.Status,
			EnvKeys: keys, Ports: portStrings(c.Ports), DatabaseEngine: engine, DatabaseVersion: dbver}
		for _, m := range c.Mounts {
			pc.Mounts = append(pc.Mounts, &migrationpb.Mount{Type: m.Type, Name: m.Name, Source: m.Source, Destination: m.Destination, ReadOnly: !m.RW})
		}
		inv.Containers = append(inv.Containers, pc)
	}
	for _, v := range vols.Volumes {
		inv.Volumes = append(inv.Volumes, &migrationpb.Volume{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint})
	}
	return inv, nil
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
