/** Mirrors internal/deploywhy.Report on the wire. */
export interface DeployWhyTemplate {
  template_id: string
  template: string
  count: number
  sample: string
}

export interface DeployWhyHealth {
  target: string
  success: boolean
  status_code?: number
  error?: string
}

export interface DeployWhy {
  deploy_id: string
  verdict: string
  exit_code?: number
  oom_killed?: boolean
  health?: DeployWhyHealth
  baseline_deploy_id?: string
  new_template_count: number
  new_templates: DeployWhyTemplate[]
}
