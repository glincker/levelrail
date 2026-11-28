package docker

import (
	"context"
	"io"

	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/api/types/volume"
	dockerclient "github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// engine keeps the call shapes of the previous Docker SDK so the moby module
// move touches types, not every call site. Methods shadow the embedded client.
type engine struct{ *dockerclient.Client }

func (e engine) Ping(ctx context.Context) (dockerclient.PingResult, error) {
	return e.Client.Ping(ctx, dockerclient.PingOptions{})
}

func (e engine) ServerVersion(ctx context.Context) (dockerclient.ServerVersionResult, error) {
	return e.Client.ServerVersion(ctx, dockerclient.ServerVersionOptions{})
}

func (e engine) Info(ctx context.Context) (system.Info, error) {
	r, err := e.Client.Info(ctx, dockerclient.InfoOptions{})
	return r.Info, err
}

func (e engine) ContainerList(ctx context.Context, o dockerclient.ContainerListOptions) ([]container.Summary, error) {
	r, err := e.Client.ContainerList(ctx, o)
	return r.Items, err
}

func (e engine) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	r, err := e.Client.ContainerInspect(ctx, id, dockerclient.ContainerInspectOptions{})
	return r.Container, err
}

func (e engine) ContainerCreate(ctx context.Context, cfg *container.Config, host *container.HostConfig, net *network.NetworkingConfig, plat *ocispec.Platform, name string) (dockerclient.ContainerCreateResult, error) {
	return e.Client.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Config: cfg, HostConfig: host, NetworkingConfig: net, Platform: plat, Name: name})
}

func (e engine) ContainerStart(ctx context.Context, id string, o dockerclient.ContainerStartOptions) error {
	_, err := e.Client.ContainerStart(ctx, id, o)
	return err
}

func (e engine) ContainerStop(ctx context.Context, id string, o dockerclient.ContainerStopOptions) error {
	_, err := e.Client.ContainerStop(ctx, id, o)
	return err
}

func (e engine) ContainerRemove(ctx context.Context, id string, o dockerclient.ContainerRemoveOptions) error {
	_, err := e.Client.ContainerRemove(ctx, id, o)
	return err
}

func (e engine) ContainerKill(ctx context.Context, id, signal string) error {
	_, err := e.Client.ContainerKill(ctx, id, dockerclient.ContainerKillOptions{Signal: signal})
	return err
}

func (e engine) ContainerPause(ctx context.Context, id string) error {
	_, err := e.Client.ContainerPause(ctx, id, dockerclient.ContainerPauseOptions{})
	return err
}

func (e engine) ContainerUnpause(ctx context.Context, id string) error {
	_, err := e.Client.ContainerUnpause(ctx, id, dockerclient.ContainerUnpauseOptions{})
	return err
}

func (e engine) ContainerLogs(ctx context.Context, id string, o dockerclient.ContainerLogsOptions) (io.ReadCloser, error) {
	return e.Client.ContainerLogs(ctx, id, o)
}

func (e engine) ContainerWait(ctx context.Context, id string, cond container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	r := e.Client.ContainerWait(ctx, id, dockerclient.ContainerWaitOptions{Condition: cond})
	return r.Result, r.Error
}

func (e engine) ContainerUpdate(ctx context.Context, id string, u container.UpdateConfig) (container.UpdateResponse, error) {
	r, err := e.Client.ContainerUpdate(ctx, id, dockerclient.ContainerUpdateOptions{Resources: &u.Resources, RestartPolicy: &u.RestartPolicy})
	return container.UpdateResponse{Warnings: r.Warnings}, err
}

func (e engine) ContainerStatsOneShot(ctx context.Context, id string) (dockerclient.ContainerStatsResult, error) {
	return e.Client.ContainerStats(ctx, id, dockerclient.ContainerStatsOptions{})
}

func (e engine) ContainerExecCreate(ctx context.Context, id string, o dockerclient.ExecCreateOptions) (dockerclient.ExecCreateResult, error) {
	return e.Client.ExecCreate(ctx, id, o)
}

func (e engine) ContainerExecAttach(ctx context.Context, execID string, o dockerclient.ExecAttachOptions) (dockerclient.HijackedResponse, error) {
	r, err := e.Client.ExecAttach(ctx, execID, o)
	return r.HijackedResponse, err
}

func (e engine) ContainerExecInspect(ctx context.Context, execID string) (dockerclient.ExecInspectResult, error) {
	return e.Client.ExecInspect(ctx, execID, dockerclient.ExecInspectOptions{})
}

func (e engine) ContainerExecResize(ctx context.Context, execID string, o dockerclient.ExecResizeOptions) error {
	_, err := e.Client.ExecResize(ctx, execID, o)
	return err
}

func (e engine) CopyFromContainer(ctx context.Context, id, path string) (io.ReadCloser, container.PathStat, error) {
	r, err := e.Client.CopyFromContainer(ctx, id, dockerclient.CopyFromContainerOptions{SourcePath: path})
	return r.Content, r.Stat, err
}

func (e engine) CopyToContainer(ctx context.Context, id, path string, content io.Reader, o dockerclient.CopyToContainerOptions) error {
	o.DestinationPath, o.Content = path, content
	_, err := e.Client.CopyToContainer(ctx, id, o)
	return err
}

func (e engine) ImageInspect(ctx context.Context, id string) (image.InspectResponse, error) {
	r, err := e.Client.ImageInspect(ctx, id)
	return r.InspectResponse, err
}

func (e engine) ImageList(ctx context.Context, o dockerclient.ImageListOptions) ([]image.Summary, error) {
	r, err := e.Client.ImageList(ctx, o)
	return r.Items, err
}

func (e engine) ImageRemove(ctx context.Context, id string, o dockerclient.ImageRemoveOptions) ([]image.DeleteResponse, error) {
	r, err := e.Client.ImageRemove(ctx, id, o)
	return r.Items, err
}

func (e engine) ImageTag(ctx context.Context, src, dst string) error {
	_, err := e.Client.ImageTag(ctx, dockerclient.ImageTagOptions{Source: src, Target: dst})
	return err
}

func (e engine) ImagePull(ctx context.Context, ref string, o dockerclient.ImagePullOptions) (io.ReadCloser, error) {
	return e.Client.ImagePull(ctx, ref, o)
}

func (e engine) ImageHistory(ctx context.Context, id string) ([]image.HistoryResponseItem, error) {
	r, err := e.Client.ImageHistory(ctx, id)
	return r.Items, err
}

func (e engine) ImageSave(ctx context.Context, ids []string) (io.ReadCloser, error) {
	return e.Client.ImageSave(ctx, ids)
}

func (e engine) ImageLoad(ctx context.Context, in io.Reader) (io.ReadCloser, error) {
	return e.Client.ImageLoad(ctx, in)
}

func (e engine) ImageBuild(ctx context.Context, bc io.Reader, o dockerclient.ImageBuildOptions) (dockerclient.ImageBuildResult, error) {
	return e.Client.ImageBuild(ctx, bc, o)
}

func (e engine) ImagesPrune(ctx context.Context, f dockerclient.Filters) (image.PruneReport, error) {
	r, err := e.Client.ImagePrune(ctx, dockerclient.ImagePruneOptions{Filters: f})
	return r.Report, err
}

func (e engine) BuildCachePrune(ctx context.Context, o dockerclient.BuildCachePruneOptions) (*build.CachePruneReport, error) {
	r, err := e.Client.BuildCachePrune(ctx, o)
	return &r.Report, err
}

func (e engine) NetworkInspect(ctx context.Context, id string, o dockerclient.NetworkInspectOptions) (network.Inspect, error) {
	r, err := e.Client.NetworkInspect(ctx, id, o)
	return r.Network, err
}

func (e engine) NetworkList(ctx context.Context, o dockerclient.NetworkListOptions) ([]network.Summary, error) {
	r, err := e.Client.NetworkList(ctx, o)
	return r.Items, err
}

func (e engine) NetworkCreate(ctx context.Context, name string, o dockerclient.NetworkCreateOptions) (dockerclient.NetworkCreateResult, error) {
	return e.Client.NetworkCreate(ctx, name, o)
}

func (e engine) NetworkRemove(ctx context.Context, id string) error {
	_, err := e.Client.NetworkRemove(ctx, id, dockerclient.NetworkRemoveOptions{})
	return err
}

func (e engine) NetworkConnect(ctx context.Context, id, ctr string, ep *network.EndpointSettings) error {
	_, err := e.Client.NetworkConnect(ctx, id, dockerclient.NetworkConnectOptions{Container: ctr, EndpointConfig: ep})
	return err
}

func (e engine) NetworkDisconnect(ctx context.Context, id, ctr string, force bool) error {
	_, err := e.Client.NetworkDisconnect(ctx, id, dockerclient.NetworkDisconnectOptions{Container: ctr, Force: force})
	return err
}

func (e engine) VolumeList(ctx context.Context, o dockerclient.VolumeListOptions) (volumeList, error) {
	r, err := e.Client.VolumeList(ctx, o)
	return volumeList{Volumes: ptrs(r.Items), Warnings: r.Warnings}, err
}

func (e engine) VolumeInspect(ctx context.Context, id string) (volume.Volume, error) {
	r, err := e.Client.VolumeInspect(ctx, id, dockerclient.VolumeInspectOptions{})
	return r.Volume, err
}

func (e engine) VolumeCreate(ctx context.Context, o dockerclient.VolumeCreateOptions) (volume.Volume, error) {
	r, err := e.Client.VolumeCreate(ctx, o)
	return r.Volume, err
}

func (e engine) VolumeRemove(ctx context.Context, id string, force bool) error {
	_, err := e.Client.VolumeRemove(ctx, id, dockerclient.VolumeRemoveOptions{Force: force})
	return err
}

func (e engine) Events(ctx context.Context, o dockerclient.EventsListOptions) (<-chan events.Message, <-chan error) {
	r := e.Client.Events(ctx, o)
	return r.Messages, r.Err
}

type volumeList struct {
	Volumes  []*volume.Volume
	Warnings []string
}

func ptrs[T any](in []T) []*T {
	out := make([]*T, len(in))
	for i := range in {
		out[i] = &in[i]
	}
	return out
}
