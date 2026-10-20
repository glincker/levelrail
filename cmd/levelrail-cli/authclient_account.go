package main

import (
	"context"
	"net/http"
)

type deleteAccountRequest struct {
	Confirm  string `json:"confirm"`
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

// DeleteAccount calls DELETE /api/v1/auth/account with the typed email and an inline password proof.
func (c *authSessionClient) DeleteAccount(ctx context.Context, confirm, password, code string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/account", deleteAccountRequest{Confirm: confirm, Password: password, Code: code}, nil)
}
