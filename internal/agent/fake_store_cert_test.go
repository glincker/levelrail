package agent

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// The cert lifecycle half of fakeEnrollStore. Tests that exercise renewal
// semantics in depth use the real store instead (openLiveTestStore).

func (f *fakeEnrollStore) RotateNodeCert(_ context.Context, id, presented string, c store.NodeCert, grace time.Duration, now time.Time) error {
	n, ok := f.nodes[id]
	if !ok {
		return store.ErrNodeNotFound
	}
	if !n.AcceptsCert(presented, now) {
		return store.ErrNodeCertNotAccepted
	}
	until := now.Add(grace)
	n.PrevCertFingerprint, n.PrevCertValidUntil = presented, &until
	n.CertFingerprint, n.CertSerial, n.CertNotAfter = c.Fingerprint, c.Serial, &c.NotAfter
	return nil
}

func (f *fakeEnrollStore) ReenrollNodeCert(_ context.Context, id string, c store.NodeCert, _ time.Time) error {
	n, ok := f.nodes[id]
	if !ok {
		return store.ErrNodeNotFound
	}
	n.CertFingerprint, n.CertNotAfter, n.CertRevokedAt, n.PrevCertFingerprint = c.Fingerprint, &c.NotAfter, nil, ""
	return nil
}

func (f *fakeEnrollStore) SyncNodeCertDetails(context.Context, string, string, string, time.Time) error {
	return nil
}

func (f *fakeEnrollStore) UpdateNodeAgentInfo(_ context.Context, id string, info store.NodeAgentInfo, _ time.Time) error {
	f.agentInfoMu.Lock()
	defer f.agentInfoMu.Unlock()
	if f.agentInfo == nil {
		f.agentInfo = map[string]store.NodeAgentInfo{}
	}
	f.agentInfo[id] = info
	return nil
}

func (f *fakeEnrollStore) agentInfoFor(id string) (store.NodeAgentInfo, bool) {
	f.agentInfoMu.Lock()
	defer f.agentInfoMu.Unlock()
	info, ok := f.agentInfo[id]
	return info, ok
}
