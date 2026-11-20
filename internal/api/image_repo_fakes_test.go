package api

import "context"

// The ownership table is a required AppStore method; these fakes never own a repo.
func (f *fakeDrainAppStore) ClaimImageRepo(_ context.Context, _, app string) (string, error) {
	return app, nil
}

func (f *fakeDrainAppStore) ImageRepoOwner(context.Context, string) (string, error) { return "", nil }

func (f *fakeProjectLifecycleAppStore) ClaimImageRepo(_ context.Context, _, app string) (string, error) {
	return app, nil
}

func (f *fakeProjectLifecycleAppStore) ImageRepoOwner(context.Context, string) (string, error) {
	return "", nil
}
