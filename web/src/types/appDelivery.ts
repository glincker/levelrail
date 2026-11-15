// Delivery tab wire types for GET /api/v1/apps/{name}/timeline and /bindings.

export type TimelineStatus =
  'succeeded' | 'failed' | 'in_progress' | 'pending' | 'info'

export interface TimelineRef {
  type: string
  id: string
}

export interface TimelineItem {
  id: string
  at: string
  kind: string
  status: string
  actor: string
  title: string
  detail?: string
  url?: string
  ref?: TimelineRef
}

export interface TimelinePage {
  items: TimelineItem[]
  next_cursor?: string
}

export interface AppBinding {
  id?: string
  ref: string
  target_type: string
  target_id: string
  role: string
  // The server stores rules as canonical JSON text.
  rules?: string
  origin: string
}

export interface ResolvedBinding extends AppBinding {
  /** Org id the row comes from when it is an inherited default. */
  inherited_from?: string
  /** An org default the app's own binding for the same role replaces. */
  overridden?: boolean
}

export interface AppBindingsResponse {
  bindings: AppBinding[] | null
  resolved?: ResolvedBinding[] | null
}

export interface AppBindingsData {
  bindings: AppBinding[]
  resolved: ResolvedBinding[]
}
