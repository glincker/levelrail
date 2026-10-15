package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	theauth "github.com/glincker/theauth-go/v2"
)

// ErrSAMLConnectionNotFound means no library connection has that id.
var ErrSAMLConnectionNotFound = errors.New("authengine: saml connection not found")

// ErrSAMLDisabled means the engine runs without an SP keypair.
var ErrSAMLDisabled = errors.New("authengine: saml is not enabled")

// SAMLAttributes names the assertion attributes; empty falls back to the
// library's WS-Federation defaults.
type SAMLAttributes struct {
	Email  string
	Name   string
	Groups string
}

// SAMLConnection is the library's view of one IdP binding.
type SAMLConnection struct {
	ID          string
	IDPEntityID string
	IDPSSOURL   string
	IDPCertPEM  string
	SPEntityID  string
	SPACSURL    string
	Attributes  SAMLAttributes
}

func (c SAMLConnection) input(org theauth.ULID) theauth.SAMLConnectionInput {
	return theauth.SAMLConnectionInput{
		OrganizationID: org,
		IdPEntityID:    c.IDPEntityID,
		IdPSSOURL:      c.IDPSSOURL,
		IdPX509Cert:    c.IDPCertPEM,
		SPEntityID:     c.SPEntityID,
		SPACSURL:       c.SPACSURL,
		AttributeMap: theauth.SAMLAttributeMap{
			Email: c.Attributes.Email, Name: c.Attributes.Name, Groups: c.Attributes.Groups,
		},
	}
}

func fromLibraryConnection(c theauth.SAMLConnection) SAMLConnection {
	return SAMLConnection{
		ID: c.ID.String(), IDPEntityID: c.IdPEntityID, IDPSSOURL: c.IdPSSOURL, IDPCertPEM: c.IdPX509Cert,
		SPEntityID: c.SPEntityID, SPACSURL: c.SPACSURL,
		Attributes: SAMLAttributes{Email: c.AttributeMap.Email, Name: c.AttributeMap.Name, Groups: c.AttributeMap.Groups},
	}
}

func (e *Engine) samlConnID(id string) (theauth.ULID, error) {
	if !e.samlOn {
		return theauth.ULID{}, ErrSAMLDisabled
	}
	u, err := parseULID(id)
	if err != nil {
		return theauth.ULID{}, ErrSAMLConnectionNotFound
	}
	return u, nil
}

// CreateSAMLConnection stores a connection in the instance organization and returns its id.
func (e *Engine) CreateSAMLConnection(ctx context.Context, c SAMLConnection) (string, error) {
	if !e.samlOn {
		return "", ErrSAMLDisabled
	}
	row, err := e.auth.CreateSAMLConnection(ctx, c.input(e.orgID))
	if err != nil {
		return "", fmt.Errorf("authengine: create saml connection: %w", err)
	}
	return row.ID.String(), nil
}

// UpdateSAMLConnection rewrites a connection in place.
func (e *Engine) UpdateSAMLConnection(ctx context.Context, c SAMLConnection) error {
	id, err := e.samlConnID(c.ID)
	if err != nil {
		return err
	}
	if _, err := e.SAMLConnection(ctx, c.ID); err != nil {
		return err
	}
	if _, err := e.auth.UpdateSAMLConnection(ctx, id, c.input(e.orgID)); err != nil {
		return fmt.Errorf("authengine: update saml connection: %w", err)
	}
	return nil
}

// DeleteSAMLConnection removes a connection and the identities linked through it.
func (e *Engine) DeleteSAMLConnection(ctx context.Context, connID string) error {
	id, err := e.samlConnID(connID)
	if err != nil {
		return err
	}
	if err := e.auth.DeleteSAMLConnection(ctx, id); err != nil {
		if errors.Is(err, theauth.ErrStorageNotFound) {
			return ErrSAMLConnectionNotFound
		}
		return fmt.Errorf("authengine: delete saml connection: %w", err)
	}
	return nil
}

// SAMLConnection loads one connection of the instance organization.
func (e *Engine) SAMLConnection(ctx context.Context, connID string) (SAMLConnection, error) {
	id, err := e.samlConnID(connID)
	if err != nil {
		return SAMLConnection{}, err
	}
	row, err := e.auth.SAMLConnectionByID(ctx, id)
	if errors.Is(err, theauth.ErrStorageNotFound) || (err == nil && row.OrganizationID != e.orgID) {
		return SAMLConnection{}, ErrSAMLConnectionNotFound
	}
	if err != nil {
		return SAMLConnection{}, fmt.Errorf("authengine: load saml connection: %w", err)
	}
	return fromLibraryConnection(*row), nil
}

// SAMLMetadata renders the SP metadata IdP administrators import.
func (e *Engine) SAMLMetadata(ctx context.Context, connID string) ([]byte, error) {
	if _, err := e.SAMLConnection(ctx, connID); err != nil {
		return nil, err
	}
	id, _ := e.samlConnID(connID)
	md, err := e.auth.SAMLMetadataXML(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("authengine: saml metadata: %w", err)
	}
	return md, nil
}

// BeginSAMLLogin returns the IdP redirect and the AuthnRequest ID inside it,
// which the caller binds to the browser.
func (e *Engine) BeginSAMLLogin(ctx context.Context, connID string) (redirect, requestID string, err error) {
	if _, err := e.SAMLConnection(ctx, connID); err != nil {
		return "", "", err
	}
	id, _ := e.samlConnID(connID)
	redirect, err = e.auth.BeginSAMLLogin(ctx, id, "")
	if err != nil {
		return "", "", fmt.Errorf("authengine: begin saml login: %w", err)
	}
	requestID, err = authnRequestID(redirect)
	if err != nil {
		return "", "", err
	}
	return redirect, requestID, nil
}

// FinishSAMLLogin hands an already verified response to the library, which
// links the identity and issues the session. It returns the session token and
// the library user it belongs to.
func (e *Engine) FinishSAMLLogin(ctx context.Context, connID, samlResponse, userAgent, ip string) (token, engineUserID string, err error) {
	if _, err := e.SAMLConnection(ctx, connID); err != nil {
		return "", "", err
	}
	id, _ := e.samlConnID(connID)
	token, sess, err := e.auth.FinishSAMLLogin(ctx, id, samlResponse, userAgent, hostOnly(ip))
	if err != nil {
		return "", "", fmt.Errorf("authengine: finish saml login: %w", err)
	}
	return token, sess.UserID.String(), nil
}

// SAMLIdentityUser returns the library user linked to (connection, NameID), or "".
func (e *Engine) SAMLIdentityUser(ctx context.Context, connID, nameID string) (string, error) {
	var uid string
	err := e.db.QueryRowContext(ctx, `SELECT user_id FROM theauth_saml_identities WHERE connection_id = ? AND name_id = ?`,
		connID, nameID).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("authengine: saml identity lookup: %w", err)
	}
	return uid, nil
}

// LegacyUserFor maps a library user id to the platform user, or "".
func (e *Engine) LegacyUserFor(ctx context.Context, engineUserID string) (string, error) {
	id, ok, err := e.dir.LegacyUserID(ctx, engineUserID)
	if err != nil || !ok {
		return "", err
	}
	return id, nil
}

// DiscardLibraryUser removes a library user that has no platform user, so a
// refused sign-in never leaves an orphan behind.
func (e *Engine) DiscardLibraryUser(ctx context.Context, engineUserID string) error {
	legacy, err := e.LegacyUserFor(ctx, engineUserID)
	if err != nil {
		return err
	}
	if legacy != "" {
		return errors.New("authengine: refusing to discard a mapped user")
	}
	return e.sessions.DeleteUser(ctx, strings.TrimSpace(engineUserID))
}
