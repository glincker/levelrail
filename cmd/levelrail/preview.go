package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/preview"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type previewNotifier interface {
	NotifyReady(app, image, runningImageID string)
}

// previewRolloutRecorder forwards rollout records to inner and tells the
// preview manager whenever a release is observed serving.
type previewRolloutRecorder struct {
	inner  application.RolloutRecorder
	notify previewNotifier
}

func (p previewRolloutRecorder) RecordRollout(ctx context.Context, serviceName, image, state, runningImageID string) error {
	err := p.inner.RecordRollout(ctx, serviceName, image, state, runningImageID)
	if state == store.RolloutStateServing && p.notify != nil {
		p.notify.NotifyReady(serviceName, image, runningImageID)
	}
	return err
}

// rolloutRecorderFor wraps db with the preview hook when a manager exists.
func rolloutRecorderFor(db *store.DB, notify previewNotifier) application.RolloutRecorder {
	if notify == nil {
		return db
	}
	return previewRolloutRecorder{inner: db, notify: notify}
}

// previewResolver maps an app to the current release and its Docker network.
type previewResolver struct {
	db            *store.DB
	sql           *preview.SQLStore
	networkPrefix string
	localNodeID   string
}

func (r previewResolver) Resolve(ctx context.Context, app, image string) (preview.Target, error) {
	svc, err := r.db.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return preview.Target{}, &preview.SkipError{Reason: "app_gone"}
	}
	if err != nil {
		return preview.Target{}, fmt.Errorf("load app %q: %w", app, err)
	}
	if image != "" && svc.Image != image {
		return preview.Target{}, &preview.SkipError{Reason: "superseded"}
	}
	id, err := r.sql.LatestServingDeployment(ctx, app, svc.Image)
	if err != nil {
		return preview.Target{}, err
	}
	if id == "" {
		return preview.Target{}, &preview.SkipError{Reason: "not_serving"}
	}
	skip := func(reason, detail string) error {
		return &preview.SkipError{DeploymentID: id, Reason: reason, Detail: detail}
	}
	switch {
	case svc.NodeID != "" && svc.NodeID != r.localNodeID:
		return preview.Target{}, skip(preview.ReasonRemoteNode, "the app runs on another node")
	case svc.AppID == "":
		return preview.Target{}, skip(preview.ReasonNoAppNetwork, "the app has no private network to capture from")
	case svc.Port <= 0:
		return preview.Target{}, skip(preview.ReasonNoAppNetwork, "the app declares no port")
	}
	return preview.Target{
		DeploymentID: id, Image: svc.Image,
		Network: application.NetworkName(r.networkPrefix, svc.AppID),
		Host:    application.ServiceAlias(svc), Port: svc.Port,
	}, nil
}

func (r previewResolver) CurrentDeployment(ctx context.Context, app string) (string, error) {
	svc, err := r.db.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load app %q: %w", app, err)
	}
	return r.sql.LatestSucceededDeployment(ctx, app, svc.Image)
}

func (r previewResolver) AppExists(ctx context.Context, app string) (bool, error) {
	_, err := r.db.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load app %q: %w", app, err)
	}
	return true, nil
}

// newPreviewManager builds the preview manager and starts its sweep.
func newPreviewManager(ctx context.Context, logger *slog.Logger, db *store.DB, client *docker.Client, dataDir, namespace, localNodeID string) *preview.Manager {
	cfg := preview.ConfigFromEnv(os.LookupEnv, logger)
	sqlStore := preview.NewSQLStore(db)
	m := preview.New(cfg, preview.Deps{
		Store:     sqlStore,
		Resolver:  previewResolver{db: db, sql: sqlStore, networkPrefix: namespace, localNodeID: localNodeID},
		Runner:    preview.NewDockerRunner(client),
		DataDir:   dataDir,
		Namespace: namespace,
		Logger:    logger,
	})
	m.Start(ctx)
	return m
}
