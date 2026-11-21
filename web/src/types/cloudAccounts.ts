export interface CloudAccount {
  id: string
  provider: string
  label: string
  method: string
  config: Record<string, string>
  team_id?: string
  status: string
  linked: boolean
  last_verified_at?: string
  created_by?: string
  created_at: string
  updated_at: string
}

export interface CloudVerifyResult {
  ok: boolean
  class: string
  message: string
  identity?: { account: string; arn: string }
  needs?: string
}

export interface CloudCapabilityResult {
  name: string
  ok: boolean
  verified: boolean
  method: string
  missing?: string[]
  hint?: string
}

export interface CloudRegion {
  id: string
  name: string
}

export interface AwsLinkKitInput {
  capabilities: string[]
  regions: string[]
  role_name?: string
  trusted_principal_arn: string
  guard_policy_arn?: string
  template_url?: string
  stack_name?: string
  region?: string
}

export interface AwsLinkKit {
  external_id: string
  role_name: string
  template_yaml: string
  quick_create_url?: string
  terraform: string
  policy_json: string
  guard_policy_arn?: string
}

export interface CreateAwsRoleAccountInput {
  label: string
  role_arn: string
  external_id: string
  region: string
}

export interface CloudGrant {
  id: string
  account_id: string
  resource_kind: string
  resource_ref: string
  principal_type: string
  principal_id: string
  actions: string[]
  granted_by: string
  expires_at?: string
  created_at: string
}

export interface CloudGrantInput {
  resource_kind: string
  resource_ref?: string
  principal_type: string
  principal_id: string
  actions: string[]
  expires_at?: string
}

export interface CloudAccessData {
  root: boolean
  accounts: Record<string, string[]>
}

export interface AwsControlIdentity {
  account: string
  arn: string
  configured: boolean
}

export interface CloudInstance {
  id: string
  name: string
  region: string
  state: string
  raw_state: string
  shape: string
  ocpus?: number
  memory_gb?: number
  public_ipv4?: string
  created_at: string
  tags: Record<string, string>
  managed: boolean
  read_only: boolean
}

export interface CloudInstanceDetail extends CloudInstance {
  private_ipv4?: string
  zone?: string
  image_id?: string
  boot_volume_gb?: number
  project?: string
  environment?: string
  resize_advisory?: string
  verbs: string[]
}

export type CloudInstanceAction =
  'start' | 'stop' | 'soft_stop' | 'reboot' | 'soft_reboot'

export interface CloudResizeInput {
  instance_type?: string
  ocpus?: number
  memory_gb?: number
}

export interface CloudSpendTotal {
  currency: string
  amount: number
}

export interface CloudSpendLine extends CloudSpendTotal {
  name: string
  provider_id?: string
}

export interface CloudSpendReport {
  account_id: string
  provider: string
  period_start: string
  period_end: string
  state: string
  fix_hint?: string
  totals: CloudSpendTotal[]
  by_service: CloudSpendLine[]
  source: string
  estimated: boolean
  data_as_of?: string
  fetched_at: string
  stale: boolean
  error_code?: string
}
