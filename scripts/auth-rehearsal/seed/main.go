// Command seed writes a synthetic legacy database and its answer key for the auth backfill rehearsal.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/scripts/auth-rehearsal/rehearsal"
)

func main() {
	d := rehearsal.DefaultConfig("")
	dir := flag.String("dir", "", "data directory to create (required)")
	flag.Uint64Var(&d.Seed, "seed", d.Seed, "deterministic seed")
	flag.IntVar(&d.Users, "users", d.Users, "number of users")
	flag.IntVar(&d.Tokens, "tokens", d.Tokens, "number of API tokens")
	flag.IntVar(&d.Passkeys, "passkeys", d.Passkeys, "number of passkeys")
	flag.IntVar(&d.TOTPUsers, "totp-users", d.TOTPUsers, "users enrolled in TOTP")
	flag.IntVar(&d.CaseDupes, "case-dupes", d.CaseDupes, "users whose email differs from another only by case")
	flag.IntVar(&d.OAuth, "oauth-identities", d.OAuth, "linked OAuth identities (default 0, for the OAuth backfill)")
	flag.IntVar(&d.BcryptCost, "bcrypt-cost", d.BcryptCost, "bcrypt cost of seeded password hashes")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "seed: -dir is required")
		os.Exit(2)
	}
	d.Dir = *dir
	m, err := rehearsal.Seed(context.Background(), d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	fmt.Printf("seeded %s: %d users, %d tokens, %d passkeys, %d totp\n", d.Dir, m.Expected.Users, len(m.Tokens), m.Expected.Passkeys, m.Expected.TOTP)
}
