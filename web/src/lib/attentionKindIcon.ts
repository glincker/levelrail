export const KIND_ICON_KINDS = [
  'host_pressure',
  'deploy_interrupted',
  'unclean_restart',
  'git_connection_reauth',
  'git_token_expiring',
  'token_grace_sweep_failing',
  'build_queue_paused',
] as const

export type KindWithIcon = (typeof KIND_ICON_KINDS)[number]

export function hasKindIcon(kind: string | undefined): kind is KindWithIcon {
  return (KIND_ICON_KINDS as readonly string[]).includes(kind ?? '')
}
