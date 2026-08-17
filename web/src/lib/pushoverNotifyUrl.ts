// Packs a Pushover User Key and Application API Token into the
// notify_url shape the control plane expects: internal/alerting's own
// parsePushoverCreds reads them back out of this same token/user query
// string, the identical convention already used for a Telegram
// notify_url's chat_id query param.

const PUSHOVER_ENDPOINT = 'https://api.pushover.net/1/messages.json'

export function buildPushoverNotifyUrl(userKey: string, apiToken: string): string {
  const params = new URLSearchParams({ token: apiToken, user: userKey })
  return `${PUSHOVER_ENDPOINT}?${params.toString()}`
}
