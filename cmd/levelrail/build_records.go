package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/buildrecord"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// buildRecordWiring signs remote build results and gates deploys on them; the gate is always installed, the recorder needs a secrets store.
func buildRecordWiring(ctx context.Context, logger *slog.Logger, db *store.DB, mgr *secrets.Manager) (build.RouterOption, deploy.Option) {
	st := buildrecord.DBStore{DB: db}
	if mgr == nil {
		return nil, deploy.WithBuildGate(installGate(db, &buildrecord.Gate{Store: st, Policy: buildPolicy(db)}))
	}
	signer, err := buildrecord.LoadOrCreateSigner(ctx, mgr)
	if err != nil {
		logger.Error("build records: signing key unavailable, personal builders stay blocked", slog.String("error", err.Error()))
		return nil, deploy.WithBuildGate(installGate(db, &buildrecord.Gate{Store: st, Policy: buildPolicy(db)}))
	}
	recorder := func(ctx context.Context, in build.RecordInput) (string, error) {
		claims := buildrecord.Claims{
			App: in.App, Repo: in.Ref, Digest: in.Digest, BuilderNodeID: in.BuilderNode,
			BuilderTrust: in.BuilderTrust, SourceRef: sourceRefOfTag(in.Tag), IssuedAt: time.Now().UTC(),
		}
		if env, err := db.EnvironmentOfApp(ctx, in.App); err == nil && env != nil {
			claims.Environment = env.Name
		}
		signed, err := signer.Sign(claims)
		if err != nil {
			return "", err
		}
		id := signed.Claims.Nonce
		return id, st.SaveRecord(ctx, id, in.App, signed)
	}
	return build.WithRouterRecorder(recorder), deploy.WithBuildGate(installGate(db, &buildrecord.Gate{Signer: signer, Store: st, Policy: buildPolicy(db)}))
}

// installGate also vets every image written as desired state, so promote, rollback and redeploy cannot skip the checks.
func installGate(db *store.DB, g *buildrecord.Gate) *buildrecord.Gate {
	db.SetDesiredImageGuard(g.CheckImage)
	return g
}

func sourceRefOfTag(tag string) string {
	if _, t, err := build.SplitRef(tag); err == nil {
		return t
	}
	return ""
}

func buildPolicy(db *store.DB) func(ctx context.Context, app string) (buildrecord.Policy, error) {
	settings := runtimesettings.New(db, os.LookupEnv)
	return func(ctx context.Context, app string) (buildrecord.Policy, error) {
		pol := buildrecord.Policy{PersonalInProd: settings.Bool(ctx, api.KeyBuildPersonalInProduction)}
		env, err := db.EnvironmentOfApp(ctx, app)
		if err != nil || env == nil {
			return pol, err
		}
		pol.EnvKind, pol.EnvName = env.Kind, env.Name
		if allowed, err := db.GetEnvironmentBuilders(ctx, env.ID); err == nil {
			pol.Allowed = allowed
		}
		return pol, nil
	}
}
