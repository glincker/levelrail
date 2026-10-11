import type security from '../../locales/en/security.json'

export type PostureItemKey = keyof (typeof security)['items']

const KNOWN = new Set<string>([
  'admin_without_mfa',
  'account_flagged',
  'recovery_codes_missing',
  'no_admin_passkey',
  'token_root',
  'token_no_expiry',
  'token_signin_approve',
  'token_unused',
  'sessions_old',
  'new_device_approval_off',
  'approval_password_only',
  'code_login_admins',
  'public_exposure',
  'docker_api_exposed',
  'offbox_backup_off',
  'master_key_rotation',
  'secrets_old',
  'tls_not_public',
  'hsts_off',
  'agents_outdated',
  'login_anomalies',
  'own_mfa',
  'own_recovery_codes',
  'own_account_flagged',
] satisfies PostureItemKey[])

// A rule id the server adds before this copy exists renders as its raw id.
export function postureItemKey(id: string): PostureItemKey | null {
  return KNOWN.has(id) ? (id as PostureItemKey) : null
}
