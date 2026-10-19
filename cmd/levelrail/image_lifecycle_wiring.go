package main

import (
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	envImageVerifyKeyFile = "APP_IMAGE_VERIFY_PUBKEY_FILE"
	envImageVerifyMode    = "APP_IMAGE_VERIFY_MODE"
)

// imageVerifyOption turns on signature checks when a public key file is
// configured. A key that cannot be read is logged and verification stays off,
// so a bad path never blocks startup.
func imageVerifyOption(logger *slog.Logger) api.Option {
	mode := os.Getenv(envImageVerifyMode)
	if mode != api.ImageVerifyEnforce {
		mode = api.ImageVerifyOff
	}
	pub, err := api.ImageVerifyConfigFromEnv(os.Getenv(envImageVerifyKeyFile))
	if err != nil {
		logger.Error("images: signature verification disabled", slog.String("error", err.Error()))
		return api.WithImageVerifyIssue(err.Error(), mode)
	}
	if pub == nil {
		if mode == api.ImageVerifyEnforce {
			return api.WithImageVerifyIssue("", mode)
		}
		return func(*api.Router) {}
	}
	return api.WithImageVerify(pub, mode)
}

// imageInventoryOption exposes the Docker image inventory of every node whose
// runtime can list images.
func imageInventoryOption(resolve func(nodeID string) (docker.Runtime, error)) api.Option {
	return api.WithImageInventory(func(nodeID string) (docker.ImageInventory, error) {
		rt, err := resolve(nodeID)
		if err != nil {
			return nil, err
		}
		inv, ok := rt.(docker.ImageInventory)
		if !ok {
			return nil, errUnsupportedImageInventory
		}
		return inv, nil
	})
}

// buildCacheOption exposes the BuildKit cache of every node whose runtime can report it.
func buildCacheOption(resolve func(nodeID string) (docker.Runtime, error)) api.Option {
	return api.WithBuildCache(func(nodeID string) (docker.BuildCacheAdmin, error) {
		rt, err := resolve(nodeID)
		if err != nil {
			return nil, err
		}
		admin, ok := rt.(docker.BuildCacheAdmin)
		if !ok {
			return nil, errUnsupportedBuildCache
		}
		return admin, nil
	})
}

const errUnsupportedBuildCache = imageInventoryError("this node does not report its build cache yet")

type imageInventoryError string

func (e imageInventoryError) Error() string { return string(e) }

const errUnsupportedImageInventory = imageInventoryError("this node does not report its images yet")
