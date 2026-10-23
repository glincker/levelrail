import type { TimeRangeKey } from '../lib/timeRange'

export type WidgetType =
  | 'metric_chart'
  | 'request_percentiles'
  | 'error_rate'
  | 'log_tail'
  | 'deploy_timeline'
  | 'resource_counts'
  | 'cost'
  | 'alert_status'
  | 'node_capacity'
  | 'link'

export type WidgetSize = 'S' | 'M' | 'L'
export type TargetKind = 'app' | 'database' | 'node'
export type DashboardScope = 'user' | 'shared'

export interface WidgetTarget {
  kind: TargetKind
  name: string
}

export interface DashboardWidget {
  id: string
  type: WidgetType
  title?: string
  size: WidgetSize
  target?: WidgetTarget
  metric?: string
  query?: string
  url?: string
}

// Matches internal/dashboards.Doc, the exportable body of a dashboard.
export interface DashboardDoc {
  version: 1
  name: string
  description?: string
  time_range?: TimeRangeKey
  refresh_seconds?: number
  widgets: DashboardWidget[]
}

// Matches internal/api's dashboardResource.
export interface CustomDashboard {
  id: string
  owner: string
  scope: DashboardScope
  project_id?: string
  is_default: boolean
  name: string
  editable: boolean
  created_at: string
  updated_at: string
  doc: DashboardDoc
}

export interface DashboardTemplate {
  key: string
  needs_target?: TargetKind
  doc: DashboardDoc
}

export interface DashboardWrite {
  scope: DashboardScope
  project_id?: string
  is_default?: boolean
  doc: DashboardDoc
}
