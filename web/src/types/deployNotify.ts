// Wire types for the deploy-notify-target resource, matching
// internal/api/deploy_notify_targets.go's deployTargetResource exactly
// (wave-2 roadmap item #5, deploy-outcome notifications): same field
// names, same snake_case JSON tags. Distinct from types/alerts.ts's
// AlertRule on purpose: a deploy-outcome notify target has no
// metric/comparator/threshold/restart fields and no evaluation state,
// see internal/alerting/deploy_notify.go's own doc comment for why it's
// a much narrower shape than an alert rule.

export type DeployNotifyKind = 'generic' | 'slack' | 'discord' | 'telegram' | 'email'

// GET/POST /api/v1/apps/{name}/deploy-notify-targets response shape.
// `id` and `resource_id` are always server-assigned (deployTargetResource's
// doc comment on toDeployTarget/handleCreateDeployNotifyTarget), never
// something this app sends on create.
export interface DeployNotifyTarget {
  id: string
  resource_id: string
  notify_url?: string
  notify_kind?: DeployNotifyKind
  enabled: boolean
}

// POST /api/v1/apps/{name}/deploy-notify-targets request body. No `id`
// or `resource_id`: the server mints/derives both itself
// (alerting.NewDeployTargetID, resourceIDForApp(name)).
export interface CreateDeployNotifyTargetRequest {
  notify_url: string
  notify_kind?: DeployNotifyKind
  enabled: boolean
}
