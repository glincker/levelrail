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
