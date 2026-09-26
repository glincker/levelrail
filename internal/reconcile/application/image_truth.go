package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RolloutRecorder stamps what is actually serving onto deploy history.
// *store.DB satisfies this structurally.
type RolloutRecorder interface {
	RecordRollout(ctx context.Context, serviceName, image, state, runningImageID string) error
}

// WithRolloutRecorder records, per reconcile, whether the running
// containers match the deployed image. nil records nothing.
func WithRolloutRecorder(r RolloutRecorder) Option {
	return func(ctrl *Controller) { ctrl.rollouts = r }
}

// WithPreviousReleaseHold keeps the previous release's running containers
// for d after a blue-green or rolling cutover, as an instant rollback
// target and for routes still draining. 0 (the default) removes them at
// cutover.
func WithPreviousReleaseHold(d time.Duration) Option {
	return func(ctrl *Controller) { ctrl.previousReleaseHold = d }
}

// NameImage is the image identity container names hash. A build's local
// image ID is folded in so rebuilding the same tag with new content gets a
// new container instead of silently keeping the old one.
func NameImage(svc store.DesiredService) string {
	if id := svc.LocalImageID(); id != "" {
		return svc.Image + "\x00id:" + id
	}
	return svc.Image
}

// imageMismatchError means a container runs different content than the
// desired image resolves to on its node.
type imageMismatchError struct {
	container, want, got string
}

func (e *imageMismatchError) Error() string {
	return fmt.Sprintf("container %s runs image %s but the deployed image resolves to %s; redeploy to converge", e.container, shortID(e.got), shortID(e.want))
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// expectedImageID is the local image ID desired content must run as, or ""
// when it cannot be known (an unpinned legacy tag, or a transport that
// cannot inspect images). Unknown means unverified, never mismatched.
func (c *Controller) expectedImageID(ctx context.Context, desired *store.DesiredService) (string, error) {
	if id := desired.LocalImageID(); id != "" {
		return id, nil
	}
	if docker.ImageDigestOf(desired.Image) == "" {
		return "", nil
	}
	inspector, ok := c.runtime.(docker.ImageInspector)
	if !ok {
		return "", nil
	}
	id, err := inspector.InspectImageID(ctx, desired.Image)
	if err != nil {
		return "", fmt.Errorf("inspect desired image: %w", err)
	}
	return id, nil
}

// verifyImage fails when state is known to run content other than desired.
func (c *Controller) verifyImage(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService) error {
	if state == nil || state.ImageID == "" {
		return nil
	}
	want, err := c.expectedImageID(ctx, desired)
	if err != nil || want == "" {
		return err
	}
	if want != state.ImageID {
		return &imageMismatchError{container: state.Name, want: want, got: state.ImageID}
	}
	return nil
}

func (c *Controller) recordRollout(ctx context.Context, desired *store.DesiredService, state, runningImageID string) {
	if c.rollouts == nil {
		return
	}
	// Best effort: history annotation must never fail a reconcile.
	_ = c.rollouts.RecordRollout(ctx, c.serviceName, desired.Image, state, runningImageID)
}

// removeStaleAfterHold is removeStale that keeps the most recent previous
// release's running containers until previousReleaseHold has passed since
// the current cutover, so a rollback and a roll forward inside the window
// are both instant. Older releases, stopped containers and excess replicas
// of the current release are never held.
func (c *Controller) removeStaleAfterHold(ctx context.Context, targets []string) error {
	if c.previousReleaseHold <= 0 {
		return c.removeStale(ctx, targets)
	}
	all, err := c.runtime.ListByPrefix(ctx, c.serviceName+"-")
	if err != nil {
		return fmt.Errorf("list containers for %s: %w", c.serviceName, err)
	}
	due, heldUntil := c.heldFilter(all, targets, time.Now())
	c.recordHold(ctx, heldUntil)
	return c.removeContainers(ctx, due)
}

// releaseOf is a container name without its replica suffix.
func releaseOf(name string) string {
	if i := strings.LastIndex(name, "-r"); i > 0 {
		if n, err := strconv.Atoi(name[i+2:]); err == nil && n > 0 && !strings.HasPrefix(name[i+2:], "+") {
			return name[:i]
		}
	}
	return name
}

// heldFilter returns the stale containers due for removal at now, and when
// the held previous release (if any) is due. A release's cutover is its
// replica 0's creation, so scaling up does not extend the window.
func (c *Controller) heldFilter(all []docker.ContainerState, targets []string, now time.Time) (due []docker.ContainerState, heldUntil time.Time) {
	firstCreated := map[string]time.Time{}
	lastCreated := map[string]time.Time{}
	seenBase := map[string]bool{}
	for _, cs := range all {
		if !ownsContainer(c.serviceName, cs.Name) {
			continue
		}
		rel := releaseOf(cs.Name)
		if cs.Name == rel {
			firstCreated[rel] = cs.Created
			seenBase[rel] = true
		} else if first, ok := firstCreated[rel]; !seenBase[rel] && (!ok || cs.Created.Before(first)) {
			firstCreated[rel] = cs.Created
		}
		if cs.Created.After(lastCreated[rel]) {
			lastCreated[rel] = cs.Created
		}
	}
	var cutover time.Time
	for _, t := range firstCreated {
		if t.After(cutover) {
			cutover = t
		}
	}
	holding := !cutover.IsZero() && now.Before(cutover.Add(c.previousReleaseHold))
	currentBase := ""
	if len(targets) > 0 {
		currentBase = releaseOf(targets[0])
	}

	stale := c.filterStale(all, targets)
	newestPrevious := ""
	if holding {
		for _, cs := range stale {
			rel := releaseOf(cs.Name)
			if !cs.Running || rel == currentBase {
				continue
			}
			if newestPrevious == "" || lastCreated[rel].After(lastCreated[newestPrevious]) || (lastCreated[rel].Equal(lastCreated[newestPrevious]) && rel > newestPrevious) {
				newestPrevious = rel
			}
		}
	}
	for _, cs := range stale {
		if newestPrevious != "" && cs.Running && releaseOf(cs.Name) == newestPrevious {
			continue
		}
		due = append(due, cs)
	}
	if newestPrevious != "" {
		heldUntil = cutover.Add(c.previousReleaseHold)
	}
	return due, heldUntil
}
