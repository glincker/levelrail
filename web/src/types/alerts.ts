// Wire types for the alert-rule resource, matching
// internal/api/alerts.go's ruleResource exactly (TASKS.md 2.5/2.7):
// same field names, same snake_case JSON tags, same "threshold-kind
// fields ignored for a crashloop rule and vice versa" shape rather than
// two separate response types, because that's what the server actually
// sends back for both kinds through one endpoint.

// cert_expiry watches every certificate on the whole control plane, not
// just this app's own domains, and patch_status watches every node's
// pending OS security patches the same way: neither uses any of the
// threshold/crashloop fields below, matching internal/alerting.Rule's
// own shape.
export type AlertRuleKind = 'threshold' | 'crashloop' | 'cert_expiry' | 'patch_status'

export type Comparator = '>' | '<' | '>=' | '<='

export type NotifyKind =
  | 'generic'
  | 'slack'
  | 'discord'
  | 'telegram'
  | 'email'
  | 'pushover'
  | 'pagerduty'
  | 'teams'

// GET/POST /api/v1/apps/{name}/alerts response shape. `id` and
// `resource_id` are always server-assigned (ruleResource's doc comment
// on toRule/handleCreateAlertRule), never something this app sends on
// create, so they only ever appear on responses, not on
// CreateAlertRuleRequest below.
export interface AlertRule {
  id: string
  name: string
  kind: AlertRuleKind
  resource_id: string

  // Threshold-kind fields, absent/zero-value for a crashloop rule.
  metric?: string
  comparator?: Comparator
  threshold: number
  for_duration?: string

  // Crashloop-kind fields, absent/zero-value for a threshold rule.
  restart_count_threshold: number
  restart_window?: string

  // notify_url/notify_kind are the *resolved* values: the attached
  // channel's own when channel_id is set, this rule's legacy columns
  // otherwise (rules created before notification channels existed).
  channel_id?: string
  notify_url?: string
  notify_kind?: NotifyKind
  enabled: boolean

  // Evaluation state, response-only: only the evaluator ever sets these
  // (internal/alerting's Engine via UpdateState), a create request never
  // carries them. See ruleResource's own doc comment for the same
  // request/response asymmetry on the Go side.
  firing?: boolean
  pending_since?: string
  firing_since?: string
  last_evaluated_at?: string
  last_value?: number
}

// POST /api/v1/apps/{name}/alerts request body. No `id` or
// `resource_id`: the server mints/derives both itself
// (alerting.NewRuleID, resourceIDForApp(name)) and discards anything a
// caller puts in those fields, so this app never sends them in the
// first place.
export interface CreateAlertRuleRequest {
  name: string
  kind: AlertRuleKind
  metric?: string
  comparator?: Comparator
  threshold?: number
  for_duration?: string
  restart_count_threshold?: number
  restart_window?: string
  channel_id?: string
  notify_url?: string
  notify_kind?: NotifyKind
  enabled: boolean
}
