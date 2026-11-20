package main

import (
	"context"
	"encoding/json"
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

// buildRecordStore adapts the store rows to the buildrecord.Store the gate reads.
type buildRecordStore struct{ db *store.DB }

func (s buildRecordStore) SaveRecord(ctx context.Context, id, app string, r buildrecord.Signed) error {
	claims, err := json.Marshal(r.Claims)
	if err != nil {
		return err
	}
	return s.db.SaveBuildRecord(ctx, store.BuildRecord{ID: id, App: app, Claims: string(claims), Signature: r.Signature, KeyID: r.KeyID, CreatedAt: r.Claims.IssuedAt})
}

func (s buildRecordStore) GetRecord(ctx context.Context, id string) (buildrecord.Signed, error) {
	row, err := s.db.GetBuildRecord(ctx, id)
	if err != nil {
		return buildrecord.Signed{}, err
	}
	out := buildrecord.Signed{Signature: row.Signature, KeyID: row.KeyID}
	if err := json.Unmarshal([]byte(row.Claims), &out.Claims); err != nil {
		return buildrecord.Signed{}, err
	}
	return out, nil
}

func (s buildRecordStore) LinkAttempt(ctx context.Context, id, attemptID string) error {
	return s.db.LinkBuildRecordAttempt(ctx, id, attemptID)
}

// buildRecordWiring signs remote build results and gates deploys on them; both are nil without a secrets store.
func buildRecordWiring(ctx context.Context, logger *slog.Logger, db *store.DB, mgr *secrets.Manager) (build.RouterOption, deploy.Option) {
	if mgr == nil {
		return nil, nil
	}
	signer, err := buildrecord.LoadOrCreateSigner(ctx, mgr)
	if err != nil {
		logger.Error("build records: signing key unavailable, personal builders stay blocked", slog.String("error", err.Error()))
		return nil, deploy.WithBuildGate(&buildrecord.Gate{Policy: buildPolicy(db)})
	}
	st := buildRecordStore{db: db}
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
	return build.WithRouterRecorder(recorder), deploy.WithBuildGate(&buildrecord.Gate{Signer: signer, Store: st, Policy: buildPolicy(db)})
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
		pol.EnvKind = env.Kind
		if allowed, err := db.GetEnvironmentBuilders(ctx, env.ID); err == nil {
			pol.Allowed = allowed
		}
		return pol, nil
	}
}
