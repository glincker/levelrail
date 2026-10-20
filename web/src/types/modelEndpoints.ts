// Wire shapes for external inference endpoints, mirroring
// internal/api/models_endpoints.go.

export type EndpointApiType = 'openai' | 'ollama'

export type EndpointState =
  'unknown' | 'online' | 'slow' | 'unreachable' | 'auth_failed' | 'error'

// Typed fixes the API suggests for a failed probe (fixFor in the API).
export type EndpointFix =
  | 'check_port'
  | 'check_network'
  | 'check_host'
  | 'use_other_address'
  | 'enable_internal'
  | 'set_credential'
  | 'check_api_type'
  | 'use_http_or_trust'
  | 'pull_model'
  | 'check_server'

export interface EndpointDiagnosis {
  code: string
  message: string
  fix: EndpointFix | ''
}

export interface EndpointHealth {
  state: EndpointState
  latency_ms: number
  checked_at?: string
  last_ok_at?: string
  consecutive_failures: number
  diagnosis?: EndpointDiagnosis
}

export interface ExternalEndpoint {
  base_url: string
  api_type: EndpointApiType
  credential_set: boolean
  fallback_model?: string
  fallback_model_ref?: string
  latency_budget_ms: number
  fallback_count: number
  models: string[]
  discovery_error?: string
  private_allowed: boolean
  health: EndpointHealth
}

export interface ExternalEndpointRequest {
  base_url: string
  api_type: EndpointApiType
  credential?: string
  fallback_model?: string
  fallback_model_ref?: string
  latency_budget_ms?: number
}

export interface EndpointTestResult {
  state: EndpointState
  latency_ms: number
  models: string[]
  diagnosis?: EndpointDiagnosis
}
