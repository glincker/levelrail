// Packs an Opsgenie API key into the notify_url shape the control plane
// expects: internal/alerting's own parseOpsgenieCreds reads it back out
// of this same key query string, the identical convention
// buildResendNotifyUrl already uses for its own credentials.

const OPSGENIE_ENDPOINT = 'https://api.opsgenie.com/v2/alerts'

export function buildOpsgenieNotifyUrl(apiKey: string): string {
  const params = new URLSearchParams({ key: apiKey })
  return `${OPSGENIE_ENDPOINT}?${params.toString()}`
}
