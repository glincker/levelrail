package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const (
	maxSAMLNameLen      = 64
	maxSAMLAttributeLen = 256
)

var samlDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

type samlCertificateInfo struct {
	Subject  string `json:"subject"`
	NotAfter string `json:"not_after"`
}

type samlConnectionResource struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Enabled         bool                  `json:"enabled"`
	Configured      bool                  `json:"configured"`
	IDPEntityID     string                `json:"idp_entity_id"`
	IDPSSOURL       string                `json:"idp_sso_url"`
	IDPCertificates string                `json:"idp_certificates"`
	IDPMetadataURL  string                `json:"idp_metadata_url"`
	Certificates    []samlCertificateInfo `json:"certificates"`
	EmailAttribute  string                `json:"email_attribute"`
	NameAttribute   string                `json:"name_attribute"`
	GroupsAttribute string                `json:"groups_attribute"`
	AllowedDomains  []string              `json:"allowed_domains"`
	JITProvisioning bool                  `json:"jit_provisioning"`
	DefaultRoleID   string                `json:"default_role_id"`
	SyncRoles       bool                  `json:"sync_roles"`
	SPEntityID      string                `json:"sp_entity_id"`
	SPACSURL        string                `json:"sp_acs_url"`
	SPMetadataURL   string                `json:"sp_metadata_url"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
}

type samlConnectionRequest struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	IDPEntityID     string   `json:"idp_entity_id"`
	IDPSSOURL       string   `json:"idp_sso_url"`
	IDPCertificates string   `json:"idp_certificates"`
	IDPMetadataURL  string   `json:"idp_metadata_url"`
	EmailAttribute  string   `json:"email_attribute"`
	NameAttribute   string   `json:"name_attribute"`
	GroupsAttribute string   `json:"groups_attribute"`
	AllowedDomains  []string `json:"allowed_domains"`
	JITProvisioning bool     `json:"jit_provisioning"`
	DefaultRoleID   string   `json:"default_role_id"`
	SyncRoles       bool     `json:"sync_roles"`
}

type samlMetadataImportRequest struct {
	URL string `json:"url"`
	XML string `json:"xml"`
}

// samlBinding is the IdP side of a connection, from the library or the draft.
func (rt *Router) samlBinding(ctx context.Context, c store.SSOConnection) (store.SSODraft, error) {
	if c.LibraryID == "" {
		return c.Draft, nil
	}
	lib, err := rt.authEngine.SAMLConnection(ctx, c.LibraryID)
	if err != nil {
		return store.SSODraft{}, err
	}
	return store.SSODraft{IDPEntityID: lib.IDPEntityID, IDPSSOURL: lib.IDPSSOURL, IDPCertificate: lib.IDPCertPEM,
		EmailAttribute: lib.Attributes.Email, NameAttribute: lib.Attributes.Name, GroupsAttribute: lib.Attributes.Groups}, nil
}

func (rt *Router) samlResource(ctx context.Context, c store.SSOConnection) (samlConnectionResource, error) {
	b, err := rt.samlBinding(ctx, c)
	if err != nil {
		return samlConnectionResource{}, err
	}
	res := samlConnectionResource{
		ID: c.ID, Name: c.Name, Enabled: c.Enabled, Configured: c.LibraryID != "",
		IDPEntityID: b.IDPEntityID, IDPSSOURL: b.IDPSSOURL, IDPCertificates: b.IDPCertificate, IDPMetadataURL: c.IDPMetadataURL,
		Certificates: []samlCertificateInfo{}, EmailAttribute: b.EmailAttribute, NameAttribute: b.NameAttribute,
		GroupsAttribute: b.GroupsAttribute, AllowedDomains: nonNilStrings(c.AllowedDomains), JITProvisioning: c.JITProvisioning,
		DefaultRoleID: c.DefaultRoleID, SyncRoles: c.SyncRoles,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if subject, notAfter, ok := authengine.IDPCertificateInfo(b.IDPCertificate); ok {
		res.Certificates = append(res.Certificates, samlCertificateInfo{Subject: subject, NotAfter: notAfter.UTC().Format(time.RFC3339)})
	}
	if base, err := rt.samlBaseURL(ctx); err == nil {
		res.SPEntityID, res.SPACSURL = samlEndpoints(base, c.ID)
		res.SPMetadataURL = res.SPEntityID
	}
	return res, nil
}

func normalizeSAMLDomains(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" {
			continue
		}
		if !samlDomainRe.MatchString(d) {
			return nil, errors.New("allowed_domains: " + d + " is not a domain name")
		}
		out = append(out, d)
	}
	return out, nil
}

// applySAMLRequest validates req onto c's platform fields and returns the
// IdP binding it asks for.
func (rt *Router) applySAMLRequest(ctx context.Context, c *store.SSOConnection, req samlConnectionRequest) (store.SSODraft, int, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxSAMLNameLen {
		return store.SSODraft{}, http.StatusBadRequest, errors.New("name is required and at most 64 characters")
	}
	for _, a := range []string{req.EmailAttribute, req.NameAttribute, req.GroupsAttribute} {
		if len(a) > maxSAMLAttributeLen {
			return store.SSODraft{}, http.StatusBadRequest, errors.New("attribute names are at most 256 characters")
		}
	}
	domains, err := normalizeSAMLDomains(req.AllowedDomains)
	if err != nil {
		return store.SSODraft{}, http.StatusBadRequest, err
	}
	if len(domains) == 0 {
		return store.SSODraft{}, http.StatusBadRequest, errors.New("allowed_domains: add at least one email domain so sign-in is limited to your organization")
	}
	b := store.SSODraft{IDPEntityID: strings.TrimSpace(req.IDPEntityID), IDPSSOURL: strings.TrimSpace(req.IDPSSOURL),
		IDPCertificate: strings.TrimSpace(req.IDPCertificates), EmailAttribute: strings.TrimSpace(req.EmailAttribute),
		NameAttribute: strings.TrimSpace(req.NameAttribute), GroupsAttribute: strings.TrimSpace(req.GroupsAttribute)}
	if b.IDPCertificate != "" {
		pemCert, err := authengine.NormalizeIDPCertificate(b.IDPCertificate)
		if err != nil {
			return store.SSODraft{}, http.StatusBadRequest, errors.New("idp_certificates: " + err.Error())
		}
		b.IDPCertificate = pemCert
	}
	if req.IDPMetadataURL != "" && !strings.HasPrefix(req.IDPMetadataURL, "https://") {
		return store.SSODraft{}, http.StatusBadRequest, errors.New("idp_metadata_url must use https")
	}
	if req.DefaultRoleID != "" {
		if _, err := rt.roles.GetRole(ctx, req.DefaultRoleID); err != nil {
			return store.SSODraft{}, http.StatusBadRequest, errors.New("default_role_id does not name a stored role")
		}
	}
	for _, d := range domains {
		owner, err := rt.ssoDomainOwner(ctx, d, ssoKindSAML, c.ID)
		if err != nil {
			return store.SSODraft{}, http.StatusInternalServerError, errors.New("internal error")
		}
		if owner != "" {
			return store.SSODraft{}, http.StatusConflict, errors.New("allowed_domains: " + d + " is already claimed by " + owner)
		}
	}
	c.Name, c.Enabled, c.IDPMetadataURL = name, req.Enabled, strings.TrimSpace(req.IDPMetadataURL)
	c.AllowedDomains, c.JITProvisioning, c.DefaultRoleID, c.SyncRoles = domains, req.JITProvisioning, req.DefaultRoleID, req.SyncRoles
	return b, 0, nil
}

// placeSAMLBinding writes b to the library when it is complete, else keeps it
// as a draft. A connection already in the library cannot fall back to a
// draft, and only a library-backed connection may be enabled.
func (rt *Router) placeSAMLBinding(ctx context.Context, c *store.SSOConnection, b store.SSODraft) (createdLib string, status int, err error) {
	idp := authengine.SAMLIdP{EntityID: b.IDPEntityID, SSOURL: b.IDPSSOURL, CertPEM: b.IDPCertificate}
	idpErr := idp.Check()
	base, baseErr := rt.samlBaseURL(ctx)
	if c.LibraryID != "" && idpErr != nil {
		return "", http.StatusBadRequest, errors.New("the identity provider settings of a configured connection cannot be cleared: " + idpErr.Error())
	}
	if idpErr != nil || baseErr != nil {
		if c.Enabled {
			if idpErr != nil {
				return "", http.StatusBadRequest, errors.New("cannot enable an incomplete connection: " + idpErr.Error())
			}
			return "", http.StatusConflict, baseErr
		}
		if c.LibraryID == "" {
			c.Draft = b
			return "", 0, nil
		}
		return "", http.StatusConflict, baseErr
	}
	entityID, acs := samlEndpoints(base, c.ID)
	lc := authengine.SAMLConnection{ID: c.LibraryID, IDPEntityID: idp.EntityID, IDPSSOURL: idp.SSOURL, IDPCertPEM: idp.CertPEM,
		SPEntityID: entityID, SPACSURL: acs,
		Attributes: authengine.SAMLAttributes{Email: b.EmailAttribute, Name: b.NameAttribute, Groups: b.GroupsAttribute}}
	if c.LibraryID != "" {
		return "", 0, rt.authEngine.UpdateSAMLConnection(ctx, lc)
	}
	id, err := rt.authEngine.CreateSAMLConnection(ctx, lc)
	if err != nil {
		return "", 0, err
	}
	c.LibraryID, c.Draft = id, store.SSODraft{}
	return id, 0, nil
}

// saveSAMLConnection places the binding, then stores the platform row; if the
// row cannot be stored, a library connection made by this call is removed again.
func (rt *Router) saveSAMLConnection(w http.ResponseWriter, r *http.Request, c store.SSOConnection, b store.SSODraft, create bool) {
	ctx := r.Context()
	createdLib, status, err := rt.placeSAMLBinding(ctx, &c, b)
	if status != 0 {
		writeError(w, status, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: save saml connection in library failed", err)
		return
	}
	now := time.Now()
	if err := rt.sso.SaveSSOConnection(ctx, c, create, now); err != nil {
		if createdLib != "" {
			if derr := rt.authEngine.DeleteSAMLConnection(context.WithoutCancel(ctx), createdLib); derr != nil {
				rt.logger.Error("api: roll back library saml connection failed", slog.String("connection_id", c.ID), slog.String("error", derr.Error()))
			}
		}
		rt.writeSAMLStoreError(w, "save", err)
		return
	}
	if create {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	res, err := rt.samlResource(ctx, c)
	if err != nil {
		rt.internalError(w, "api: render saml connection failed", err)
		return
	}
	status = http.StatusOK
	if create {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

// syncSAMLEndpoints rewrites the library's SP URLs when the dashboard URL moved.
func (rt *Router) syncSAMLEndpoints(ctx context.Context, c store.SSOConnection) error {
	base, err := rt.samlBaseURL(ctx)
	if err != nil {
		return err
	}
	lib, err := rt.authEngine.SAMLConnection(ctx, c.LibraryID)
	if err != nil {
		return err
	}
	entityID, acs := samlEndpoints(base, c.ID)
	if lib.SPEntityID == entityID && lib.SPACSURL == acs {
		return nil
	}
	lib.SPEntityID, lib.SPACSURL = entityID, acs
	return rt.authEngine.UpdateSAMLConnection(ctx, lib)
}

func (rt *Router) handleListSAMLConnections(w http.ResponseWriter, r *http.Request) {
	if !rt.samlReady(w) {
		return
	}
	conns, err := rt.sso.ListSSOConnections(r.Context())
	if err != nil {
		rt.internalError(w, "api: list saml connections failed", err)
		return
	}
	out := make([]samlConnectionResource, 0, len(conns))
	for _, c := range conns {
		res, err := rt.samlResource(r.Context(), c)
		if err != nil {
			rt.internalError(w, "api: render saml connection failed", err)
			return
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) loadSAMLConnection(w http.ResponseWriter, r *http.Request) (store.SSOConnection, bool) {
	if !rt.samlReady(w) {
		return store.SSOConnection{}, false
	}
	c, err := rt.sso.GetSSOConnection(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSSOConnectionNotFound) {
		writeError(w, http.StatusNotFound, "saml connection not found")
		return store.SSOConnection{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get saml connection failed", err)
		return store.SSOConnection{}, false
	}
	return c, true
}

func (rt *Router) handleGetSAMLConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	res, err := rt.samlResource(r.Context(), c)
	if err != nil {
		rt.internalError(w, "api: render saml connection failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleCreateSAMLConnection(w http.ResponseWriter, r *http.Request) {
	if !rt.samlReady(w) {
		return
	}
	var req samlConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, authengine.MaxSAMLMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := randomOpaqueID("saml_")
	if err != nil {
		rt.internalError(w, "api: create saml connection id failed", err)
		return
	}
	c := store.SSOConnection{ID: id}
	b, status, err := rt.applySAMLRequest(r.Context(), &c, req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	rt.saveSAMLConnection(w, r, c, b, true)
}

func (rt *Router) handleUpdateSAMLConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	var req samlConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, authengine.MaxSAMLMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b, status, err := rt.applySAMLRequest(r.Context(), &c, req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	rt.saveSAMLConnection(w, r, c, b, false)
}

// handleDeleteSAMLConnection removes the library connection (and the
// identities linked through it) first, so no half-deleted connection can sign anyone in.
func (rt *Router) handleDeleteSAMLConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	if c.LibraryID != "" {
		if err := rt.authEngine.DeleteSAMLConnection(r.Context(), c.LibraryID); err != nil && !errors.Is(err, authengine.ErrSAMLConnectionNotFound) {
			rt.internalError(w, "api: delete library saml connection failed", err)
			return
		}
	}
	if err := rt.sso.DeleteSSOConnection(r.Context(), c.ID); err != nil {
		rt.writeSAMLStoreError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleImportSAMLMetadata fills the IdP fields from metadata given as XML or
// fetched from an https URL. It never enables the connection by itself.
func (rt *Router) handleImportSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	var req samlMetadataImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*authengine.MaxSAMLMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.URL == "") == (req.XML == "") {
		writeError(w, http.StatusBadRequest, "give exactly one of url or xml")
		return
	}
	raw := []byte(req.XML)
	if req.URL != "" {
		body, err := authengine.FetchSAMLIdPMetadata(r.Context(), netguard.NewClient(), req.URL)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		raw = body
	}
	idp, err := authengine.ParseSAMLIdPMetadata(raw, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := rt.samlBinding(r.Context(), c)
	if err != nil {
		rt.internalError(w, "api: load saml binding failed", err)
		return
	}
	b.IDPEntityID, b.IDPSSOURL, b.IDPCertificate = idp.EntityID, idp.SSOURL, idp.CertPEM
	if req.URL != "" {
		c.IDPMetadataURL = req.URL
	}
	rt.saveSAMLConnection(w, r, c, b, false)
}

func (rt *Router) writeSAMLStoreError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrSSOConnectionNotFound):
		writeError(w, http.StatusNotFound, "saml connection not found")
	case errors.Is(err, store.ErrSSOConnectionNameTaken):
		writeError(w, http.StatusConflict, "a saml connection with that name already exists")
	default:
		rt.internalError(w, "api: "+op+" saml connection failed", err)
	}
}
