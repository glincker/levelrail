package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// placeholderBasicAuthUser is the user a gated domain gets until the operator sets the real credential.
const placeholderBasicAuthUser = "admin"

// rootAbility mirrors the API's root ability, which the app node route requires.
const rootAbility = "root"

// applyIngress sets the translated proxy settings with the caller's own token. Basic auth uses a random
// password nobody sees, so the domain fails closed until the real credential is set.
func applyIngress(ctx context.Context, client *Client, name string, a *mappedApp) {
	p := a.Service.Ingress
	if p == nil || !p.Translated() {
		return
	}
	review := func(detail string) {
		a.Issues = appendIssue(a.Issues, "ingress", issueReview, detail)
	}
	if len(a.Service.Domains) == 0 {
		review("the source proxy settings were translated but the app has no domain yet, set them after adding one")
		return
	}
	if p.BasicAuth {
		for _, d := range a.Service.Domains {
			pw, err := placeholderPassword()
			if err == nil {
				_, err = client.SetDomainBasicAuth(ctx, name, d, apiclient.SetDomainBasicAuthRequest{Username: placeholderBasicAuthUser, Password: pw})
			}
			if err != nil {
				review("basic auth was not set on " + d + ", the domain is public until you run domains basic-auth set: " + err.Error())
			}
		}
	}
	d := a.Service.Domains[0]
	if p.Canonical != "" {
		if _, err := client.SetDomainCanonical(ctx, name, d, apiclient.CanonicalRequest{Preset: p.Canonical}); err != nil {
			review("the " + p.Canonical + " redirect was not set on " + d + ": " + err.Error())
		}
	}
	if p.ForceHTTPS {
		if _, err := client.SetDomainRedirectSettings(ctx, name, d, trafficpolicy.Redirects{ForceHTTPS: true}); err != nil {
			review("force HTTPS was not set on " + d + ": " + err.Error())
		}
	}
	if len(p.Forwarders) > 0 {
		if _, err := client.SetDomainForwarders(ctx, name, d, trafficpolicy.Forwarders{Rules: p.Forwarders}); err != nil {
			review("forwarder rules were not set on " + d + ": " + err.Error())
		}
	}
	if len(p.Headers) > 0 {
		if _, err := client.SetDomainHeaders(ctx, name, d, trafficpolicy.Headers{Rules: p.Headers}); err != nil {
			review("header rules were not set on " + d + ": " + err.Error())
		}
	}
}

func placeholderPassword() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate placeholder password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// gatePreviews keeps previews of a gated app private: they get the preview gate with a random
// password nobody sees. It returns false when previews must stay off.
func gatePreviews(a *mappedApp, req *apiclient.SetPreviewPolicyRequest) bool {
	if a.Service.Ingress == nil || !a.Service.Ingress.BasicAuth {
		return true
	}
	pw, err := placeholderPassword()
	if err != nil {
		return false
	}
	on, user := true, placeholderBasicAuthUser
	req.GateBasicAuth, req.GateUsername, req.GatePassword = &on, &user, &pw
	a.Issues = appendIssue(a.Issues, "previews", issuePlanned, "previews are gated with basic auth and a random placeholder password that is never shown, set the real one in the preview policy")
	return true
}

// placeApps records each app's target node from --node-map; an unmapped server keeps the default node.
func placeApps(apps []mappedApp, nodeMap map[string]string) {
	if len(nodeMap) == 0 {
		return
	}
	for i := range apps {
		a := &apps[i]
		if a.Blocking {
			continue
		}
		if node, ok := nodeMap[a.SourceServer]; ok && a.SourceServer != "" {
			a.Node = node
			continue
		}
		server := a.SourceServer
		if server == "" {
			server = "an unreported server"
		}
		a.Issues = appendIssue(a.Issues, "node", issueReview, fmt.Sprintf("the source ran it on %q, which --node-map does not name: it stays on the control plane node, move it later with apps set-node", server))
	}
}

// resolvePlacements checks every mapped node against the target's real node list before anything is created.
func resolvePlacements(ctx context.Context, client *Client, apps []mappedApp, nodeMap map[string]string) error {
	if len(nodeMap) == 0 {
		return nil
	}
	// Placement needs the root ability; checking first stops a weaker token from creating every app unplaced.
	access, err := client.MyAccess(ctx)
	if err != nil {
		return fmt.Errorf("check token abilities for --node-map: %w", err)
	}
	if !slices.Contains(access.Abilities, rootAbility) {
		return newValidationError("%s", "--node-map needs a token with the root ability (placing apps on nodes is infrastructure placement); nothing was created")
	}
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list target nodes for --node-map: %w", err)
	}
	refs := make([]platformimport.NodeRef, 0, len(nodes))
	for _, n := range nodes {
		refs = append(refs, platformimport.NodeRef{ID: n.ID, Name: n.Name})
	}
	ids, err := platformimport.ResolveNodeMap(nodeMap, refs)
	if err != nil {
		return newValidationError("%s", err.Error())
	}
	for i := range apps {
		if apps[i].Node != "" {
			apps[i].nodeID = ids[apps[i].SourceServer]
		}
	}
	return nil
}

// applyPlacement pins a created app to its mapped node through the same route as apps set-node.
func applyPlacement(ctx context.Context, client *Client, name string, a *mappedApp) {
	if a.nodeID == "" {
		return
	}
	if _, err := client.SetAppNode(ctx, name, a.nodeID); err != nil {
		a.Issues = appendIssue(a.Issues, "node", issueReview, fmt.Sprintf("the app was not placed on node %q: %s", a.Node, err.Error()))
	}
}
