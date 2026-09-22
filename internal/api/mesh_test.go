package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

// fakeMeshStatus is a MeshStatusProvider test double, standing in for a
// real network.Device (which needs a TUN device and root) the same way
// this package's other newTestRouterWith* fakes stand in for their own
// real, privileged or networked dependency.
type fakeMeshStatus struct {
	status network.Status
	err    error
}

func (f *fakeMeshStatus) Status(context.Context) (network.Status, error) {
	return f.status, f.err
}

// fakeMeshRotator is a MeshKeyRotator test double.
type fakeMeshRotator struct {
	result   network.RotationResult
	err      error
	statuses map[string]network.RotationStatus
	rotated  []string
}

func (f *fakeMeshRotator) RotateKey(_ context.Context, nodeID string) (network.RotationResult, error) {
	f.rotated = append(f.rotated, nodeID)
	if f.err != nil {
		return network.RotationResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeMeshRotator) RotationStatusFor(nodeID string) (network.RotationStatus, bool) {
	s, ok := f.statuses[nodeID]
	return s, ok
}

func TestHandleGetMeshStatus_NotEnabled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mesh", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleGetMeshStatus(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	priv, err := network.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	peerPriv, err := network.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	peerPub, err := peerPriv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	if err := db.UpdateNodeMesh(context.Background(), "node_a", peerPub.String(), "10.181.0.2"); err != nil {
		t.Fatalf("UpdateNodeMesh: %v", err)
	}

	now := time.Now()
	mesh := &fakeMeshStatus{status: network.Status{
		Backend:    network.BackendUserspace,
		Interface:  "levelrail0",
		PublicKey:  pub,
		ListenPort: 51820,
		Address:    netip.MustParsePrefix("10.181.0.1/16"),
		Peers: []network.PeerStatus{{
			NodeID:        "node_a",
			PublicKey:     peerPub,
			Endpoint:      "203.0.113.5:51820",
			LastHandshake: now,
			TransferRx:    100,
			TransferTx:    200,
		}},
	}}
	rt.SetLocalNodeID("local")
	rt.SetMesh(mesh, nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mesh", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got meshStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.PublicKey != pub.String() {
		t.Errorf("PublicKey = %q, want %q", got.PublicKey, pub.String())
	}
	if len(got.Peers) != 1 {
		t.Fatalf("Peers = %+v, want exactly one", got.Peers)
	}
	p := got.Peers[0]
	if p.Name != "alpha" {
		t.Errorf("peer Name = %q, want %q (joined from the store by public key)", p.Name, "alpha")
	}
	if p.MeshAddress != "10.181.0.2" {
		t.Errorf("peer MeshAddress = %q, want %q", p.MeshAddress, "10.181.0.2")
	}
	if !p.Live {
		t.Error("peer Live = false, want true: this peer came from the live device status")
	}
	if !p.Healthy {
		t.Error("peer Healthy = false, want true: handshake was just now")
	}
	if p.LastHandshake == nil {
		t.Error("peer LastHandshake is nil, want the handshake time")
	}
}

func TestHandleGetMeshStatus_ListsUnpeeredNodes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_b", "beta")

	peerPriv, err := network.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	peerPub, err := peerPriv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if err := db.UpdateNodeMesh(context.Background(), "node_b", peerPub.String(), "10.181.0.3"); err != nil {
		t.Fatalf("UpdateNodeMesh: %v", err)
	}

	rt.SetMesh(&fakeMeshStatus{status: network.Status{Backend: network.BackendUserspace}}, nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mesh", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got meshStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Peers) != 1 {
		t.Fatalf("Peers = %+v, want the known-but-unpeered node still listed", got.Peers)
	}
	if got.Peers[0].Live {
		t.Error("Live = true for a node with no live device peer entry, want false")
	}
	if got.Peers[0].NodeID != "node_b" {
		t.Errorf("NodeID = %q, want node_b", got.Peers[0].NodeID)
	}
}

func TestHandleRotateNodeMeshKey_NotEnabled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_a/mesh/rotate-key", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleRotateNodeMeshKey_NodeNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetMesh(nil, &fakeMeshRotator{})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/nope/mesh/rotate-key", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleRotateNodeMeshKey(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	oldKey, _ := network.GeneratePrivateKey()
	oldPub, _ := oldKey.PublicKey()
	newKey, _ := network.GeneratePrivateKey()
	newPub, _ := newKey.PublicKey()

	rotator := &fakeMeshRotator{result: network.RotationResult{
		NodeID: "node_a", OldPublicKey: oldPub, NewPublicKey: newPub,
	}}
	rt.SetMesh(nil, rotator)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_a/mesh/rotate-key", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got rotateKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OldPublicKey != oldPub.String() || got.NewPublicKey != newPub.String() {
		t.Errorf("got %+v, want old=%s new=%s", got, oldPub, newPub)
	}
	if len(rotator.rotated) != 1 || rotator.rotated[0] != "node_a" {
		t.Errorf("rotated = %v, want exactly [node_a]", rotator.rotated)
	}
}

func TestHandleRotateNodeMeshKey_NotSupported(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rt.SetMesh(nil, &fakeMeshRotator{err: network.ErrRotationNotSupported})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_a/mesh/rotate-key", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

// TestHandleRotateNodeMeshKey_RemoteNodeNotConnected covers a remote node
// with no live agent session right now (this file's own header): unlike
// the old, permanent 501 this route used to return for any node but the
// local one, that is now a 500 with the real reason logged server-side,
// the same as any other transport failure this package reports, not a
// missing-capability response.
func TestHandleRotateNodeMeshKey_RemoteNodeNotConnected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_b", "beta")

	rt.SetMesh(nil, &fakeMeshRotator{err: fmt.Errorf("agent: node not registered in this transport registry: %q", "node_b")})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_b/mesh/rotate-key", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
