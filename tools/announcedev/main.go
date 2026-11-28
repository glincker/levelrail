// Command announcedev prepares a dev announcement feed signed with the public
// TEST keys of the golden vectors. It never touches a production key.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type role struct {
	id    string
	tiers []string
}

var roles = []role{
	{"N", []string{"info", "feature", "maintenance", "newsletter"}},
	{"S1", []string{"advisory", "critical"}},
	{"S2", []string{"advisory", "critical"}},
}

func testKey(id string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("levelrail-announce-test-vector-key/" + id))
	return ed25519.NewKeyFromSeed(seed[:])
}

func writeJSON(path string, v any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), mode)
}

func main() {
	dir := flag.String("dir", "", "work directory to fill with keys, roles and items")
	rng := flag.String("range", ">=0.0.1", "applies_to range for the advisory and critical items")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "announcedev needs -dir")
		os.Exit(2)
	}
	if err := prepare(*dir, *rng, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, "announcedev:", err)
		os.Exit(1)
	}
}

func prepare(dir, rng string, now time.Time) error {
	keysDir := filepath.Join(dir, "keys")
	year := filepath.Join(dir, "items", now.Format("2006"))
	for _, d := range []string{keysDir, year} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
	}
	var roleKeys []map[string]any
	var devKeys []string
	for _, r := range roles {
		priv := testKey(r.id)
		pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
		kf := map[string]string{"format": "levelrail-announce-key-v1", "key_id": r.id, "public_key": pub, "private_key": base64.StdEncoding.EncodeToString(priv.Seed())}
		if err := writeJSON(filepath.Join(keysDir, r.id+".json"), kf, 0o600); err != nil {
			return err
		}
		roleKeys = append(roleKeys, map[string]any{"key_id": r.id, "public_key": pub, "tiers": r.tiers})
		devKeys = append(devKeys, r.id+":"+pub+":"+joinTiers(r.tiers))
	}
	if err := writeJSON(filepath.Join(dir, "roles.json"), map[string]any{"keys": roleKeys}, 0o644); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "dev-keys.json"), devKeys, 0o644); err != nil {
		return err
	}
	date := now.Format("2006-01-02")
	items := []map[string]any{
		item(date, "info", "Dev info notice", "A small informational notice from the dev feed.", "", ""),
		item(date, "feature", "Dev feature notice", "A new feature is available in this build.", "", ""),
		item(date, "advisory", "Dev security advisory", "Upgrade to a fixed build when you can.", rng, "updates"),
		item(date, "critical", "Dev critical notice", "This build is affected. Upgrade now.", rng, "updates"),
		item(date, "newsletter", "Dev newsletter issue", "Notes from the project this month.", "", ""),
	}
	for _, it := range items {
		it["issued_at"] = now.Add(-time.Hour).Format(time.RFC3339)
		it["expires_at"] = now.Add(30 * 24 * time.Hour).Format(time.RFC3339)
		if err := writeJSON(filepath.Join(year, it["id"].(string)+".json"), it, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func item(date, tier, title, body, applies, cta string) map[string]any {
	it := map[string]any{"id": date + "-dev-" + tier, "tier": tier, "kind": "announcement", "title": title, "body": body}
	if applies != "" {
		it["applies_to"] = applies
	}
	if cta != "" {
		it["cta"] = cta
	}
	return it
}

func joinTiers(t []string) string {
	out := ""
	for i, s := range t {
		if i > 0 {
			out += "|"
		}
		out += s
	}
	return out
}
