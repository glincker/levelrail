// Wire types for the deploy-notify-target resource, matching
// internal/api/deploy_notify_targets.go's deployTargetResource.

export type DeployNotifyKind =
  'generic' | 'slack' | 'discord' | 'telegram' | 'email'

// notify_url/notify_kind are the *resolved* values: the attached
// channel's own when channel_id is set, this row's legacy columns
// otherwise (rows created before notification channels existed).
export interface DeployNotifyTarget {
  id: string
  resource_id: string
  channel_id?: string
  notify_url?: string
  notify_kind?: DeployNotifyKind
  enabled: boolean
}

// Request body: attach an already-connected channel by ID, not a URL.
export interface CreateDeployNotifyTargetRequest {
  channel_id: string
  enabled: boolean
}
