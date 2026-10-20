import type { DeviceAuthRequest } from '../queries/deviceAuth'

// m:ss for the live countdown, "0:00" once the time has run out.
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const minutes = Math.floor(total / 60)
  const seconds = total % 60
  return `${String(minutes)}:${String(seconds).padStart(2, '0')}`
}

export function msUntilExpiry(request: DeviceAuthRequest, now: number): number {
  return new Date(request.expires_at).getTime() - now
}

// Live requests only, soonest-to-expire first so the banner always shows
// the one that will lapse next.
export function liveDeviceRequests(
  requests: DeviceAuthRequest[] | undefined,
  now: number,
): DeviceAuthRequest[] {
  return (requests ?? [])
    .filter((r) => msUntilExpiry(r, now) > 0)
    .sort((a, b) => msUntilExpiry(a, now) - msUntilExpiry(b, now))
}

// A short, human label for what is asking: the client name the CLI sent
// (its host name by default), else a browser-ish hint from the user agent.
export function requesterLabel(request: DeviceAuthRequest): string {
  if (request.client_name) {
    return request.client_name
  }
  return request.user_agent.split(' ')[0] || ''
}
