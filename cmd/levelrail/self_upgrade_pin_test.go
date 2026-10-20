package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

func okCosign(context.Context, string, ...string) ([]byte, error) { return nil, nil }

func TestSelfUpgradeSignerPinsOnFirstVerification(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := selfUpgradeSigner(selfupgrade.VerifyAuto, "acme/app", dir, "v1.2.3", logger, io.Discard)
	if s.Pinned {
		t.Fatal("a fresh host must not start pinned")
	}
	s.Run = okCosign
	if _, err := s.Verify(context.Background(), "sums", "bundle"); err != nil {
		t.Fatal(err)
	}
	pin, ok, err := selfupgrade.ReadPin(dir)
	if err != nil || !ok || pin.Version != "v1.2.3" {
		t.Fatalf("pin after verification = %+v ok=%v err=%v", pin, ok, err)
	}

	next := selfUpgradeSigner(selfupgrade.VerifyAuto, "acme/app", dir, "v1.2.4", logger, io.Discard)
	next.Run = okCosign
	if _, err := next.Verify(context.Background(), "sums", ""); !errors.Is(err, selfupgrade.ErrSignaturePinned) {
		t.Fatalf("unsigned release after the pin: err = %v, want ErrSignaturePinned", err)
	}
}

func TestSelfUpgradeSignerOffWhilePinnedIsLogged(t *testing.T) {
	dir := t.TempDir()
	if err := selfupgrade.WritePin(dir, selfupgrade.Pin{Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	var logs, out bytes.Buffer
	s := selfUpgradeSigner(selfupgrade.VerifyOff, "acme/app", dir, "v1.0.1", slog.New(slog.NewTextHandler(&logs, nil)), &out)
	if _, err := s.Verify(context.Background(), "sums", ""); err != nil {
		t.Fatalf("explicit off must still be honored: %v", err)
	}
	if !strings.Contains(logs.String(), "pinned") || !strings.Contains(out.String(), "pinned") {
		t.Fatalf("off while pinned must be logged and printed, logs=%q out=%q", logs.String(), out.String())
	}
}

func TestSelfUpgradeUnpinSignature(t *testing.T) {
	dir := t.TempDir()
	if err := selfupgrade.WritePin(dir, selfupgrade.Pin{Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runSelfUpgrade(context.Background(), []string{"--unpin-signature"}, dir, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := selfupgrade.ReadPin(dir); ok {
		t.Fatal("pin survived --unpin-signature")
	}
	if !strings.Contains(out.String(), "removed") {
		t.Fatalf("unpin output = %q", out.String())
	}
}
