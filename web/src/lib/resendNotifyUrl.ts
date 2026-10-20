// Packs a Resend API key and destination address into the notify_url
// shape the control plane expects: internal/alerting's own
// parseResendCreds reads them back out of this same key/to/from query
// string, the identical convention buildPushoverNotifyUrl already uses
// for its own two credentials.

const RESEND_ENDPOINT = 'https://api.resend.com/emails'

export function buildResendNotifyUrl(
  apiKey: string,
  to: string,
  from?: string,
): string {
  const params = new URLSearchParams({ key: apiKey, to })
  if (from) {
    params.set('from', from)
  }
  return `${RESEND_ENDPOINT}?${params.toString()}`
}
