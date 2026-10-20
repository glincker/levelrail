package docker

import "testing"

func TestNetworkShareDriverOpts_NFS(t *testing.T) {
	opts, err := NetworkShareDriverOpts(NetworkShareVolumeOpts{
		Protocol:     NetworkShareProtocolNFS,
		Host:         "nas.lan",
		RemotePath:   "/export/media",
		MountOptions: "nfsvers=4",
	})
	if err != nil {
		t.Fatalf("NetworkShareDriverOpts() error = %v", err)
	}
	want := map[string]string{
		"type":   "nfs",
		"o":      "addr=nas.lan,nfsvers=4",
		"device": ":/export/media",
	}
	if opts["type"] != want["type"] || opts["o"] != want["o"] || opts["device"] != want["device"] {
		t.Errorf("NetworkShareDriverOpts() = %+v, want %+v", opts, want)
	}
}

func TestNetworkShareDriverOpts_NFS_NoExtraOptions(t *testing.T) {
	opts, err := NetworkShareDriverOpts(NetworkShareVolumeOpts{
		Protocol:   NetworkShareProtocolNFS,
		Host:       "nas.lan",
		RemotePath: "/export/media",
	})
	if err != nil {
		t.Fatalf("NetworkShareDriverOpts() error = %v", err)
	}
	if opts["o"] != "addr=nas.lan" {
		t.Errorf("o = %q, want %q", opts["o"], "addr=nas.lan")
	}
}

func TestNetworkShareDriverOpts_CIFS(t *testing.T) {
	opts, err := NetworkShareDriverOpts(NetworkShareVolumeOpts{
		Protocol:     NetworkShareProtocolCIFS,
		Host:         "nas.lan",
		RemotePath:   "/backups",
		MountOptions: "vers=3.0",
		Username:     "backup-user",
		Password:     "s3cret",
	})
	if err != nil {
		t.Fatalf("NetworkShareDriverOpts() error = %v", err)
	}
	want := map[string]string{
		"type":   "cifs",
		"o":      "username=backup-user,password=s3cret,addr=nas.lan,vers=3.0",
		"device": "//nas.lan/backups",
	}
	if opts["type"] != want["type"] || opts["o"] != want["o"] || opts["device"] != want["device"] {
		t.Errorf("NetworkShareDriverOpts() = %+v, want %+v", opts, want)
	}
}

func TestNetworkShareDriverOpts_UnsupportedProtocol(t *testing.T) {
	_, err := NetworkShareDriverOpts(NetworkShareVolumeOpts{Protocol: "ftp"})
	if err == nil {
		t.Fatal("NetworkShareDriverOpts() error = nil, want an error for an unsupported protocol")
	}
}
