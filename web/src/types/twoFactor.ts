// Matches internal/api/twofactor.go's response structs exactly.

export interface TwoFactorStatus {
  enabled: boolean
  recovery_codes_remaining: number
}

export interface TwoFactorSetup {
  secret: string
  provisioning_uri: string
}

export interface TwoFactorRecoveryCodes {
  recovery_codes: string[]
}
