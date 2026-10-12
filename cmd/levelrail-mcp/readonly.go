package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// envReadonly is --readonly's environment counterpart.
const envReadonly = "APP_MCP_READONLY"

// writeAbilities are the abilities --readonly refuses to run with.
var writeAbilities = []string{"write", "write:sensitive", "deploy", "root"}

func checkReadonlyAbilities(abilities []string) error {
	var held []string
	for _, a := range abilities {
		if slices.Contains(writeAbilities, a) {
			held = append(held, a)
		}
	}
	if len(held) > 0 {
		return fmt.Errorf("--readonly: the API token holds %s; refusing to start, use a token with only read or read:sensitive", strings.Join(held, ", "))
	}
	return nil
}

// enforceReadonly asks the API which abilities the token really holds, so
// a mislabeled or widened token cannot slip past the flag.
func enforceReadonly(client *apiclient.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	who, err := client.Whoami(ctx)
	if err != nil {
		return fmt.Errorf("--readonly: cannot verify the API token's abilities, refusing to start: %w", err)
	}
	return checkReadonlyAbilities(who.Abilities)
}
