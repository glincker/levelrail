// Matches internal/api/twofactor.go's response structs exactly.

export interface TwoFactorStatus {
  enabled: boolean
  recovery_codes_remaining: number
  // Only present when the library engine serves MFA and no usable code is left.
  recovery_codes_need_regeneration?: boolean
}

export interface TwoFactorSetup {
  secret: string
  provisioning_uri: string
}

export interface TwoFactorRecoveryCodes {
  recovery_codes: string[]
}
