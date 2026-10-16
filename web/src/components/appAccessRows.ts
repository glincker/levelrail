import type { AppUrlAddress, AppUrls } from '../queries/appUrls'

export type AccessRowId =
  'container' | 'host' | 'local' | 'lan' | 'public' | 'domain' | 'internal'

export type AccessNote =
  | 'container'
  | 'host'
  | 'hostStopped'
  | 'hostNone'
  | 'local'
  | 'localNone'
  | 'lan'
  | 'lanLocalhost'
  | 'lanRemote'
  | 'public'
  | 'domain'
  | 'internal'

export type AccessState = 'ok' | 'warn' | 'off' | 'unknown'

export interface AccessRow {
  key: string
  id: AccessRowId
  /** Text the copy button puts on the clipboard; empty hides the button. */
  value: string
  /** Shown instead of value when value is empty. */
  display: string
  href?: string
  state: AccessState
  /** Key under appAccess:note.* */
  note: AccessNote
  allowNetwork?: boolean
}

function stateOf(a: AppUrlAddress): AccessState {
  switch (a.reachable_hint) {
    case 'reachable':
      return 'ok'
    case 'bound_localhost':
      return 'warn'
    case 'not_running':
      return 'off'
    default:
      return 'unknown'
  }
}

function addressRow(
  a: AppUrlAddress,
  index: number,
  local: boolean,
): AccessRow {
  const key = `${a.kind}-${a.host ?? ''}-${index}`
  if (a.kind === 'loopback') {
    return {
      key,
      id: 'local',
      value: a.url ?? '',
      display: a.url ?? '',
      href: a.url,
      state: stateOf(a),
      note: a.url ? 'local' : 'localNone',
    }
  }
  if (a.kind === 'lan') {
    if (a.reachable_hint === 'bound_localhost') {
      return {
        key,
        id: 'lan',
        value: '',
        display: '',
        state: 'warn',
        note: 'lanLocalhost',
        allowNetwork: local,
      }
    }
    if (a.reachable_hint === 'remote_node') {
      return {
        key,
        id: 'lan',
        value: '',
        display: '',
        state: 'unknown',
        note: 'lanRemote',
      }
    }
    return {
      key,
      id: 'lan',
      value: a.url ?? '',
      display: a.url ?? '',
      href: a.url,
      state: stateOf(a),
      note: 'lan',
    }
  }
  if (a.kind === 'public') {
    return {
      key,
      id: 'public',
      value: a.url ?? '',
      display: a.url ?? '',
      href: a.url,
      state: stateOf(a),
      note: 'public',
    }
  }
  if (a.kind === 'domain') {
    return {
      key,
      id: 'domain',
      value: a.url ?? '',
      display: a.url ?? a.host ?? '',
      href: a.url,
      state: 'unknown',
      note: 'domain',
    }
  }
  const dns = a.host ? `${a.host}:${a.port ?? 0}` : ''
  return {
    key,
    id: 'internal',
    value: dns,
    display: dns,
    state: 'unknown',
    note: 'internal',
  }
}

/** Rows of the Access card, in display order; pure so every case is testable. */
export function deriveAccessRows(urls: AppUrls): AccessRow[] {
  const rows: AccessRow[] = [
    {
      key: 'container',
      id: 'container',
      value: String(urls.container_port),
      display: String(urls.container_port),
      state: urls.running ? 'ok' : 'off',
      note: 'container',
    },
    {
      key: 'host',
      id: 'host',
      value: urls.host_port ? String(urls.host_port) : '',
      display: urls.host_port ? String(urls.host_port) : '',
      state: urls.running && urls.host_port ? 'ok' : 'off',
      note: !urls.running
        ? 'hostStopped'
        : urls.host_port
          ? 'host'
          : 'hostNone',
    },
  ]
  const order: Record<AppUrlAddress['kind'], number> = {
    loopback: 0,
    lan: 1,
    public: 2,
    domain: 3,
    internal: 4,
  }
  const sorted = urls.addresses
    .map((a, i) => ({ a, i }))
    .sort((x, y) => order[x.a.kind] - order[y.a.kind] || x.i - y.i)
  for (const { a, i } of sorted) {
    rows.push(addressRow(a, i, urls.local_node))
  }
  return rows
}

/** URL "Open app" should use: domain first, then LAN, then localhost. */
export function pickOpenUrl(urls: AppUrls | undefined): string | null {
  if (!urls) return null
  const hit = (kind: AppUrlAddress['kind']) =>
    urls.addresses.find((a) => a.kind === kind && a.url)?.url ?? null
  if (!urls.running) return hit('domain')
  return hit('domain') ?? hit('lan') ?? hit('public') ?? hit('loopback')
}
