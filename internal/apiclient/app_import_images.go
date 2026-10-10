package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppImportImage is one host-built image of an import session.
type AppImportImage struct {
	SourceID      string `json:"source_id"`
	App           string `json:"app"`
	Target        string `json:"target,omitempty"`
	Image         string `json:"image"`
	SourceImageID string `json:"source_image_id,omitempty"`
	LoadedImageID string `json:"loaded_image_id,omitempty"`
	Node          string `json:"node,omitempty"`
	State         string `json:"state"`
	Bytes         int64  `json:"bytes"`
	Verified      bool   `json:"verified"`
	Error         string `json:"error,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// AppImportImages is the image move state of a session.
type AppImportImages struct {
	Running         bool             `json:"running"`
	Source          string           `json:"source,omitempty"`
	CredentialsHeld bool             `json:"credentials_held"`
	Supported       bool             `json:"supported"`
	MaxBytes        int64            `json:"max_bytes"`
	Images          []AppImportImage `json:"images"`
}

// AppImportImagesTransfer is the body of POST .../images/transfer. The
// private key travels in this body only and is never stored.
type AppImportImagesTransfer struct {
	SSH        string   `json:"ssh,omitempty"`
	Port       int      `json:"port,omitempty"`
	PrivateKey string   `json:"private_key,omitempty"`
	Passphrase string   `json:"passphrase,omitempty"`
	UseAgent   bool     `json:"use_agent,omitempty"`
	Items      []string `json:"items,omitempty"`
}

func appImportImagesPath(id string) string {
	return appImportPath + "/sessions/" + url.PathEscape(id) + "/images"
}

// AppImportImages calls GET .../sessions/{id}/images.
func (c *Client) AppImportImages(ctx context.Context, id string) (AppImportImages, error) {
	var out AppImportImages
	err := c.do(ctx, http.MethodGet, appImportImagesPath(id), nil, &out)
	return out, err
}

// AppImportImagesStatus calls GET .../sessions/{id}/images/status.
func (c *Client) AppImportImagesStatus(ctx context.Context, id string) (AppImportImages, error) {
	var out AppImportImages
	err := c.do(ctx, http.MethodGet, appImportImagesPath(id)+"/status", nil, &out)
	return out, err
}

// TransferAppImportImages calls POST .../sessions/{id}/images/transfer.
func (c *Client) TransferAppImportImages(ctx context.Context, id string, body AppImportImagesTransfer) (AppImportImages, error) {
	var out AppImportImages
	err := c.do(ctx, http.MethodPost, appImportImagesPath(id)+"/transfer", body, &out)
	return out, err
}

// CancelAppImportImages calls POST .../sessions/{id}/images/cancel.
func (c *Client) CancelAppImportImages(ctx context.Context, id string) (AppImportImages, error) {
	var out AppImportImages
	err := c.do(ctx, http.MethodPost, appImportImagesPath(id)+"/cancel", nil, &out)
	return out, err
}
