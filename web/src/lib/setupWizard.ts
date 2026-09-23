// Pure state logic for the setup wizard: step order, resume, gating
// reasons, and the polling state machines behind each verification step.

import type { AppListEntry } from '../types/appDetail'
import type { CertificateStatus } from '../queries/certificates'
import type { DoctorReport } from '../queries/systemDoctor'
import type { IngressDomainCheckResult } from '../queries/domains'
import type { AppNetwork } from '../queries/appNetwork'
import type { GitProviderStatus } from '../types/gitProviders'

export const SETUP_STEPS = ['server', 'domain', 'git', 'app', 'done'] as const
export type SetupStepId = (typeof SETUP_STEPS)[number]
export type SetupStepStatus = 'completed' | 'skipped'
export type SetupStepMap = Partial<Record<SetupStepId, SetupStepStatus>>

export const SETUP_STEP_META: Record<
  SetupStepId,
  { title: string; optional: boolean }
> = {
  server: { title: 'Server check', optional: false },
  domain: { title: 'Dashboard domain', optional: true },
  git: { title: 'Git provider', optional: true },
  app: { title: 'First app', optional: true },
  done: { title: 'Done', optional: false },
}

export type StepGate =
  { canContinue: true } | { canContinue: false; reason: string }

const OPEN: StepGate = { canContinue: true }

function blocked(reason: string): StepGate {
  return { canContinue: false, reason }
}

function isSetupStep(value: string): value is SetupStepId {
  return (SETUP_STEPS as readonly string[]).includes(value)
}

/** resumeStep picks where a reopened wizard lands: the saved step, else the first unfinished one. */
export function resumeStep(
  currentStep: string,
  steps: SetupStepMap,
): SetupStepId {
  if (isSetupStep(currentStep)) return currentStep
  return SETUP_STEPS.find((id) => steps[id] === undefined) ?? 'done'
}

/** nextStep returns the step after id, or id itself for the last step. */
export function nextStep(id: SetupStepId): SetupStepId {
  const i = SETUP_STEPS.indexOf(id)
  return SETUP_STEPS[Math.min(i + 1, SETUP_STEPS.length - 1)] ?? 'done'
}

/** withStepStatus returns a copy of steps with id set to status. */
export function withStepStatus(
  steps: SetupStepMap,
  id: SetupStepId,
  status: SetupStepStatus,
): SetupStepMap {
  return { ...steps, [id]: status }
}

// Checks the wizard refuses to continue past: without these nothing can deploy.
const HARD_FAILURE_CODES = ['docker', 'database', 'data_dir_writable']

/** hardFailures lists the doctor checks that block the server step. */
export function hardFailures(report: DoctorReport) {
  return report.checks.filter(
    (c) => c.status === 'fail' && HARD_FAILURE_CODES.includes(c.code),
  )
}

/** serverCheckGate decides whether the server step can continue. */
export function serverCheckGate(report: DoctorReport | undefined): StepGate {
  if (!report) return blocked('Checks are still running.')
  const hard = hardFailures(report)
  if (hard.length > 0) {
    return blocked(
      `${hard.map((c) => c.name).join(', ')} failed. Fix it and re-run the checks; nothing can deploy until it passes.`,
    )
  }
  return OPEN
}

const IPV4_OR_V6 = /([0-9]{1,3}(?:\.[0-9]{1,3}){3}|[0-9a-f]*:[0-9a-f:]+)/i

/** publicIpFromDoctor extracts the detected address from the public_ip check, if it passed. */
export function publicIpFromDoctor(
  report: DoctorReport | undefined,
): string | undefined {
  const check = report?.checks.find((c) => c.code === 'public_ip')
  if (check?.status !== 'ok') return undefined
  return IPV4_OR_V6.exec(check.message)?.[1]
}

const DOMAIN_PATTERN =
  /^(?=.{1,253}$)([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/

/** normalizeDomain lowercases and strips a scheme or trailing slash, returning '' when invalid. */
export function normalizeDomain(raw: string): string {
  const d = raw
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, '')
    .replace(/\/+$/, '')
  return DOMAIN_PATTERN.test(d) ? d : ''
}

/** httpsOrigin returns "https://<domain>" for a valid hostname, else null, so only an https origin with a real host is ever linked to. */
export function httpsOrigin(domain: string): string | null {
  if (!DOMAIN_PATTERN.test(domain)) return null
  const url = new URL(`https://${domain}`)
  return url.protocol === 'https:' && url.hostname === domain
    ? url.origin
    : null
}

export type SubStepState = 'pending' | 'waiting' | 'ok' | 'error'

export interface DomainProgress {
  dns: SubStepState
  dnsDetail: string
  cert: SubStepState
  certDetail: string
}

/** domainProgress derives the DNS and certificate sub-step states from the latest poll results. */
export function domainProgress({
  domain,
  check,
  certificates,
  publicIp,
}: {
  domain: string
  check: IngressDomainCheckResult | undefined
  certificates: CertificateStatus[] | undefined
  publicIp: string | undefined
}): DomainProgress {
  let dns: SubStepState = 'waiting'
  let dnsDetail = `Looking up ${domain}.`
  if (check?.configured && check.domain === domain) {
    const resolved = check.resolved_hosts ?? []
    const pointsHere =
      check.status === 'connected' ||
      (publicIp !== undefined && resolved.includes(publicIp))
    if (pointsHere) {
      dns = 'ok'
      dnsDetail = `${domain} resolves to this server.`
    } else if (check.resolved) {
      dns = 'error'
      dnsDetail = `${domain} resolves to ${resolved.join(', ')}, not this server. Update the record; changes can take a few minutes to spread.`
    } else {
      dnsDetail = `${domain} does not resolve yet. Waiting for the DNS record to appear.`
    }
  }

  if (dns !== 'ok') {
    return {
      dns,
      dnsDetail,
      cert: 'pending',
      certDetail: 'Starts once DNS points here.',
    }
  }

  const cert = certificates?.find(
    (c) => c.domain === domain || (c.sans ?? []).includes(domain),
  )
  if (!cert) {
    return {
      dns,
      dnsDetail,
      cert: 'waiting',
      certDetail:
        'Requesting a certificate. This usually takes under a minute once ports 80 and 443 are reachable.',
    }
  }
  if (cert.status === 'expired') {
    return {
      dns,
      dnsDetail,
      cert: 'error',
      certDetail: 'The stored certificate has expired.',
    }
  }
  return {
    dns,
    dnsDetail,
    cert: 'ok',
    certDetail: cert.issuer
      ? `Issued by ${cert.issuer}.`
      : 'Certificate issued.',
  }
}

/** domainGate explains why Continue is disabled on the domain step. */
export function domainGate(
  saved: boolean,
  progress: DomainProgress | undefined,
): StepGate {
  if (!saved || !progress) {
    return blocked(
      'Enter a domain and save it to start verification, or skip this step.',
    )
  }
  if (progress.dns !== 'ok')
    return blocked(`Waiting for DNS: ${progress.dnsDetail}`)
  if (progress.cert !== 'ok')
    return blocked(`Waiting for HTTPS: ${progress.certDetail}`)
  return OPEN
}

/** pollInterval returns intervalMs while polling should continue, false once done or past maxMs since startedAt. */
export function pollInterval(
  done: boolean,
  startedAt: number,
  now: number,
  intervalMs: number,
  maxMs: number,
): number | false {
  if (done) return false
  if (now - startedAt >= maxMs) return false
  return intervalMs
}

/** gitGate allows Continue once any provider is connected. */
export function gitGate(providers: GitProviderStatus[]): StepGate {
  return providers.some((p) => p.connected)
    ? OPEN
    : blocked(
        'No git provider is connected yet. Connect one, or skip this step.',
      )
}

export type AppPhase = 'none' | 'deploying' | 'slow' | 'live' | 'failed'

/** pickTrackedApp prefers the wizard's own sample app, else the first app. */
export function pickTrackedApp(
  apps: AppListEntry[],
  preferredName: string,
): AppListEntry | undefined {
  return apps.find((a) => a.name === preferredName) ?? apps[0]
}

/** firstAppPhase maps an app's status summary plus elapsed time onto the wizard's deploy phases. */
export function firstAppPhase(
  app: AppListEntry | undefined,
  trackingSince: number,
  now: number,
  slowAfterMs: number,
): AppPhase {
  if (!app) return 'none'
  if (app.status.variant === 'success') return 'live'
  if (app.status.variant === 'destructive') return 'failed'
  return now - trackingSince >= slowAfterMs ? 'slow' : 'deploying'
}

/** appGate allows Continue once the first app is healthy. */
export function appGate(phase: AppPhase): StepGate {
  switch (phase) {
    case 'live':
      return OPEN
    case 'none':
      return blocked('Deploy something first, or skip this step.')
    case 'failed':
      return blocked(
        'The app is not healthy. Check the diagnosis below, or skip this step.',
      )
    default:
      return blocked('Waiting for the app to pass its health check.')
  }
}

/** liveUrl picks the best address to open a live app at. */
export function liveUrl(
  app: AppListEntry,
  network: AppNetwork | undefined,
  hostname: string,
): string | undefined {
  const [domain] = app.domains ?? []
  if (domain) return `https://${domain}`
  if (network?.fallback_url) return network.fallback_url
  const loopbackOnly = app.bind_address === 'private' || app.bind_address === ''
  const browserOnHost = hostname === 'localhost' || hostname === '127.0.0.1'
  if (network?.host_port && (!loopbackOnly || browserOnHost)) {
    return `http://${hostname}:${network.host_port}`
  }
  return undefined
}
