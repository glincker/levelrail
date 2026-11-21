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
