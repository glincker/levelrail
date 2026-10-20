package selfupgrade

import (
	"os"
	"testing"
	"time"
)

func TestSignaturePinRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := ReadPin(dir); err != nil || ok {
		t.Fatalf("fresh dir: ok=%v err=%v, want no pin", ok, err)
	}
	at := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	if err := WritePin(dir, Pin{Version: "v1.2.3", VerifiedAt: at}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadPin(dir)
	if err != nil || !ok || got.Version != "v1.2.3" || !got.VerifiedAt.Equal(at) {
		t.Fatalf("ReadPin = %+v ok=%v err=%v", got, ok, err)
	}
	info, err := os.Stat(PinPath(dir))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("pin file mode = %v err=%v, want 0644", info.Mode().Perm(), err)
	}
	removed, err := RemovePin(dir)
	if err != nil || !removed {
		t.Fatalf("RemovePin = %v, %v", removed, err)
	}
	if _, ok, _ := ReadPin(dir); ok {
		t.Fatal("pin still present after RemovePin")
	}
	if removed, err := RemovePin(dir); err != nil || removed {
		t.Fatalf("second RemovePin = %v, %v, want false, nil", removed, err)
	}
}

func TestReadPinWrittenByInstaller(t *testing.T) {
	dir := t.TempDir()
	body := "# written by install.sh\nversion=v0.4.0\nverified_at=2026-10-12T08:00:00Z\n"
	if err := os.WriteFile(PinPath(dir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadPin(dir)
	if err != nil || !ok || got.Version != "v0.4.0" {
		t.Fatalf("ReadPin = %+v ok=%v err=%v", got, ok, err)
	}
}

func TestReadPinEmptyFileStillPins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(PinPath(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadPin(dir); err != nil || !ok {
		t.Fatalf("an existing but empty pin must still pin: ok=%v err=%v", ok, err)
	}
}
