export type IncidentInternalState = 'triggered' | 'acknowledged' | 'resolved'
export type IncidentPublicStatus =
  'investigating' | 'identified' | 'monitoring' | 'resolved'

export interface IncidentTimelineEntry {
  status: string
  body: string
  visibility: string
  author?: string
  created_at: string
}

export interface Incident {
  id: string
  title: string
  status: IncidentPublicStatus
  impact: string
  source: string
  source_ref?: string
  internal_state: IncidentInternalState
  published: boolean
  published_at?: string
  updated_at: string
  draft?: { title: string; body: string }
  starts_at: string
  acknowledged_at?: string
  acknowledged_by?: string
  internal_resolved_at?: string
  timeline?: IncidentTimelineEntry[]
}

export interface IncidentDraftInput {
  title: string
  status: IncidentPublicStatus
  body: string
}

export interface PostmortemAlert {
  kind: string
  ref: string
  down_at?: string
  up_at?: string
}

export interface IncidentPostmortem {
  incident_id: string
  state: 'draft' | 'published'
  summary: string
  impact: string
  root_cause: string
  resolution: string
  action_items: string[]
  impact_start: string
  impact_end?: string
  alerts: PostmortemAlert[]
  published_by?: string
  published_at?: string
  updated_at: string
}

export interface PostmortemEditInput {
  summary: string
  impact: string
  root_cause: string
  resolution: string
  action_items: string[]
}
