package api

import "net/http"

// registerMailboxRoutes wires mailbox, alias and queue management (ADR 059
// section 2.6). Handlers also check the owning team and IAM resources.
func (rt *Router) registerMailboxRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/mail/domains", rt.requireAbility(AbilityRead, rt.handleListMailDomains))
	mux.HandleFunc("POST /api/v1/mail/domains", rt.requireAbility(AbilityRoot, rt.handleClaimMailDomain))
	mux.HandleFunc("GET /api/v1/mail/mailboxes", rt.requireAbility(AbilityRead, rt.handleListMailboxes))
	mux.HandleFunc("POST /api/v1/mail/mailboxes", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateMailbox))
	mux.HandleFunc("POST /api/v1/mail/mailboxes/{address}/suspend", rt.requireAbility(AbilityWriteSensitive, rt.handleSuspendMailbox(true)))
	mux.HandleFunc("POST /api/v1/mail/mailboxes/{address}/resume", rt.requireAbility(AbilityWriteSensitive, rt.handleSuspendMailbox(false)))
	mux.HandleFunc("POST /api/v1/mail/mailboxes/{address}/password", rt.requireAbility(AbilityWriteSensitive, rt.handleMailboxPassword))
	mux.HandleFunc("DELETE /api/v1/mail/mailboxes/{address}", rt.requireAbility(AbilityRoot, rt.handleDeleteMailbox))
	mux.HandleFunc("GET /api/v1/mail/aliases", rt.requireAbility(AbilityRead, rt.handleListMailAliases))
	mux.HandleFunc("POST /api/v1/mail/aliases", rt.requireAbility(AbilityWriteSensitive, rt.handleAddMailAlias))
	mux.HandleFunc("DELETE /api/v1/mail/aliases/{address}", rt.requireAbility(AbilityWriteSensitive, rt.handleRemoveMailAlias))
	mux.HandleFunc("GET /api/v1/mail/queue", rt.requireAbility(AbilityReadSensitive, rt.handleMailQueue))
	mux.HandleFunc("POST /api/v1/mail/queue/hold", rt.requireAbility(AbilityWriteSensitive, rt.handleMailQueueHold(true)))
	mux.HandleFunc("POST /api/v1/mail/queue/release", rt.requireAbility(AbilityWriteSensitive, rt.handleMailQueueHold(false)))
}
