// Package deploywatch pins released images as rollback targets and watches
// each new release after cutover, rolling it back when it misbehaves.
package deploywatch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PinStore is the store surface the Pinner needs. *store.DB satisfies it.
type PinStore interface {
	LatestDeployAttemptFor(ctx context.Context, serviceName, image string) (*store.DeployAttempt, error)
	SaveRollbackPin(ctx context.Context, p store.RollbackPin) error
	ListRollbackPins(ctx context.Context, serviceName string) ([]store.RollbackPin, error)
	DeleteRollbackPin(ctx context.Context, serviceName, attemptID string) error
}

// Pinner tags each release that takes traffic with an extra local
// reference, keeping the newest Keep per app. It only pins apps on the
// local node, where its image client can reach the image.
type Pinner struct {
	Store       PinStore
	Images      docker.ImagePinner
	Keep        int
	Namespace   string
	LocalNodeID string
	Logger      *slog.Logger
}

// PinRef is the local tag that pins attemptID's image for serviceName.
func PinRef(namespace, serviceName, attemptID string) string {
	ns := sanitizeRepo(namespace)
	if ns == "" {
		ns = "app"
	}
	return fmt.Sprintf("%s-rollback/%s:%s", ns, sanitizeRepo(serviceName), attemptID)
}

func sanitizeRepo(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-._")
}

// PinServing implements application.RollbackPinner. It returns the new
// pin reference, or "" when nothing needed pinning.
func (p *Pinner) PinServing(ctx context.Context, desired store.DesiredService, imageID string) (string, error) {
	if p == nil || p.Keep <= 0 || p.Images == nil || p.Store == nil {
		return "", nil
	}
	if desired.NodeID != "" && desired.NodeID != p.LocalNodeID {
		return "", nil
	}
	attempt, err := p.Store.LatestDeployAttemptFor(ctx, desired.Name, desired.Image)
	if err != nil {
		return "", fmt.Errorf("deploywatch: pin %q: %w", desired.Name, err)
	}
	if attempt == nil {
		return "", nil
	}
	pins, err := p.Store.ListRollbackPins(ctx, desired.Name)
	if err != nil {
		return "", fmt.Errorf("deploywatch: pin %q: %w", desired.Name, err)
	}
	for _, pin := range pins {
		if pin.DeployAttemptID == attempt.ID {
			return "", nil
		}
	}

	source := imageID
	if source == "" {
		source = desired.Image
	}
	ref := PinRef(p.Namespace, desired.Name, attempt.ID)
	if err := p.Images.TagImage(ctx, source, ref); err != nil {
		return "", fmt.Errorf("deploywatch: pin %q: %w", desired.Name, err)
	}
	pin := store.RollbackPin{ServiceName: desired.Name, DeployAttemptID: attempt.ID, Image: desired.Image, ImageID: imageID, PinRef: ref}
	if err := p.Store.SaveRollbackPin(ctx, pin); err != nil {
		_ = p.Images.UntagImage(ctx, ref)
		return "", fmt.Errorf("deploywatch: pin %q: %w", desired.Name, err)
	}
	p.prune(ctx, desired.Name)
	return ref, nil
}

// prune unpins everything past the newest Keep. Untagging never force
// removes, so an image a container still runs survives.
func (p *Pinner) prune(ctx context.Context, serviceName string) {
	pins, err := p.Store.ListRollbackPins(ctx, serviceName)
	if err != nil || len(pins) <= p.Keep {
		return
	}
	for _, old := range pins[p.Keep:] {
		if err := p.Images.UntagImage(ctx, old.PinRef); err != nil {
			p.logger().Warn("deploywatch: unpin old release failed", slog.String("service", serviceName), slog.String("pin", old.PinRef), slog.String("error", err.Error()))
			continue
		}
		if err := p.Store.DeleteRollbackPin(ctx, serviceName, old.DeployAttemptID); err != nil {
			p.logger().Warn("deploywatch: delete rollback pin failed", slog.String("service", serviceName), slog.String("attempt_id", old.DeployAttemptID), slog.String("error", err.Error()))
		}
	}
}

func (p *Pinner) logger() *slog.Logger {
	if p.Logger == nil {
		return slog.Default()
	}
	return p.Logger
}
