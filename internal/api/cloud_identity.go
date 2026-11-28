package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/glincker/levelrail/internal/store"
)

type awsControlIdentity struct {
	Account    string `json:"account"`
	ARN        string `json:"arn"`
	Configured bool   `json:"configured"`
}

// awsCallerIdentity is the seam tests replace; production asks STS on the ambient chain.
var awsCallerIdentity = func(ctx context.Context) (account, arn string, err error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", "", err
	}
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", err
	}
	return aws.ToString(out.Account), aws.ToString(out.Arn), nil
}

// handleAWSControlIdentity lets the UI prefill the trusted principal; errors never carry provider text.
func (rt *Router) handleAWSControlIdentity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	account, arn, err := awsCallerIdentity(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, awsControlIdentity{})
		return
	}
	writeJSON(w, http.StatusOK, awsControlIdentity{Account: account, ARN: arn, Configured: arn != ""})
}

type cloudAccessWire struct {
	Root     bool                `json:"root"`
	Accounts map[string][]string `json:"accounts"`
}

// handleMeCloudAccess reports the actions the caller holds per account so the UI can gate buttons.
func (rt *Router) handleMeCloudAccess(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	ptype, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: cloud access principal failed", err)
		return
	}
	accts, err := st.ListCloudAccounts(r.Context(), "")
	if err != nil {
		rt.internalError(w, "api: cloud access accounts failed", err)
		return
	}
	out := cloudAccessWire{Accounts: map[string][]string{}}
	if hasAbility(abilities, AbilityRoot) {
		out.Root = true
		all := append(store.CloudGrantActions(), cloudActionBuild, cloudActionDelete)
		for _, a := range accts {
			out.Accounts[a.ID] = all
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	caller, err := rt.cloudCallerFor(r.Context(), st, ptype, pid)
	if err != nil {
		rt.internalError(w, "api: cloud access caller failed", err)
		return
	}
	grants, err := st.ListCloudGrantsForPrincipals(r.Context(), "", caller.principals)
	if err != nil {
		rt.internalError(w, "api: cloud access grants failed", err)
		return
	}
	now := time.Now().UTC()
	byAcct := map[string]map[string]bool{}
	for _, g := range grants {
		if g.ExpiresAt != nil && !g.ExpiresAt.After(now) {
			continue
		}
		if byAcct[g.AccountID] == nil {
			byAcct[g.AccountID] = map[string]bool{}
		}
		for _, a := range g.Actions {
			byAcct[g.AccountID][a] = true
		}
	}
	for _, a := range accts {
		set := byAcct[a.ID]
		if a.TeamID != "" && isTeamAdminRole(caller.teamRoles[a.TeamID]) {
			set = map[string]bool{}
			for _, x := range store.CloudGrantActions() {
				set[x] = true
			}
		}
		if len(set) == 0 {
			continue
		}
		acts := make([]string, 0, len(set))
		for x := range set {
			acts = append(acts, x)
		}
		sort.Strings(acts)
		out.Accounts[a.ID] = acts
	}
	writeJSON(w, http.StatusOK, out)
}
