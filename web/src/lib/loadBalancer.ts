import type {
  LoadBalancerAlgorithm,
  LoadBalancerConfig,
  UpstreamState,
} from '../queries/appLoadBalancer'

export const ALGORITHM_OPTIONS: {
  value: LoadBalancerAlgorithm
  label: string
  description: string
}[] = [
  {
    value: 'round_robin',
    label: 'Round robin',
    description: 'Each request goes to the next replica in turn.',
  },
  {
    value: 'least_conn',
    label: 'Least connections',
    description: 'Send to the replica with the fewest active requests.',
  },
  {
    value: 'ip_hash',
    label: 'IP hash',
    description: 'The same client address always reaches the same replica.',
  },
  {
    value: 'uri_hash',
    label: 'URI hash',
    description:
      'The same path always reaches the same replica (cache friendly).',
  },
  {
    value: 'cookie',
    label: 'Sticky cookie',
    description: 'A cookie pins a browser session to one replica.',
  },
  {
    value: 'weighted',
    label: 'Weighted',
    description: 'Give each replica a share of traffic, useful for canaries.',
  },
]

export const MAX_WEIGHT = 100

export interface LbFormState {
  algorithm: LoadBalancerAlgorithm
  cookieName: string
  weights: number[]
  healthEnabled: boolean
  healthPath: string
  healthInterval: string
  healthTimeout: string
  healthPasses: string
  healthFails: string
  healthStatus: string
  passiveEnabled: boolean
  maxFails: string
  failDuration: string
  retriesEnabled: boolean
  retryCount: string
  tryDuration: string
  slowStart: string
  drainTimeout: string
  requestTimeout: string
  rateEnabled: boolean
  rps: string
  burst: string
  tlsEnabled: boolean
  tlsInsecure: boolean
  tlsServerName: string
}

const str = (n: number | undefined): string => (n ? String(n) : '')

export function formFromConfig(
  cfg: LoadBalancerConfig | undefined,
  replicas: number,
): LbFormState {
  const count = Math.max(replicas, cfg?.weights?.length ?? 0, 1)
  const weights = Array.from(
    { length: count },
    (_, i) => cfg?.weights?.[i] ?? 1,
  )
  return {
    algorithm: cfg?.algorithm ?? 'round_robin',
    cookieName: cfg?.cookie_name ?? '',
    weights,
    healthEnabled: cfg?.active_health !== undefined,
    healthPath: cfg?.active_health?.path ?? '/',
    healthInterval: cfg?.active_health?.interval ?? '10s',
    healthTimeout: cfg?.active_health?.timeout ?? '5s',
    healthPasses: str(cfg?.active_health?.passes),
    healthFails: str(cfg?.active_health?.fails),
    healthStatus: str(cfg?.active_health?.expect_status),
    passiveEnabled: cfg?.passive_health !== undefined,
    maxFails: str(cfg?.passive_health?.max_fails),
    failDuration: cfg?.passive_health?.fail_duration ?? '30s',
    retriesEnabled: cfg?.retries !== undefined,
    retryCount: str(cfg?.retries?.count) || '2',
    tryDuration: cfg?.retries?.try_duration ?? '5s',
    slowStart: cfg?.slow_start ?? '',
    drainTimeout: cfg?.drain_timeout ?? '',
    requestTimeout: cfg?.request_timeout ?? '',
    rateEnabled: cfg?.rate_limit !== undefined,
    rps: str(cfg?.rate_limit?.rps),
    burst: str(cfg?.rate_limit?.burst),
    tlsEnabled: cfg?.upstream_tls !== undefined,
    tlsInsecure: cfg?.upstream_tls?.insecure_skip_verify ?? false,
    tlsServerName: cfg?.upstream_tls?.server_name ?? '',
  }
}

const num = (s: string): number | undefined => {
  const n = Number.parseInt(s, 10)
  return Number.isFinite(n) && n > 0 ? n : undefined
}

const opt = (s: string): string | undefined => {
  const t = s.trim()
  return t === '' ? undefined : t
}

export function configFromForm(f: LbFormState): LoadBalancerConfig {
  const cfg: LoadBalancerConfig = { algorithm: f.algorithm }
  if (f.algorithm === 'cookie' && opt(f.cookieName)) {
    cfg.cookie_name = f.cookieName.trim()
  }
  if (f.algorithm === 'weighted') cfg.weights = f.weights
  if (f.healthEnabled) {
    cfg.active_health = {
      path: f.healthPath.trim(),
      interval: opt(f.healthInterval),
      timeout: opt(f.healthTimeout),
      passes: num(f.healthPasses),
      fails: num(f.healthFails),
      expect_status: num(f.healthStatus),
    }
  }
  if (f.passiveEnabled) {
    cfg.passive_health = {
      max_fails: num(f.maxFails),
      fail_duration: opt(f.failDuration),
    }
  }
  if (f.retriesEnabled) {
    cfg.retries = { count: num(f.retryCount), try_duration: opt(f.tryDuration) }
  }
  if (opt(f.slowStart) && f.algorithm === 'weighted') {
    cfg.slow_start = f.slowStart.trim()
  }
  if (opt(f.drainTimeout)) cfg.drain_timeout = f.drainTimeout.trim()
  if (opt(f.requestTimeout)) cfg.request_timeout = f.requestTimeout.trim()
  if (f.rateEnabled)
    cfg.rate_limit = { rps: num(f.rps) ?? 0, burst: num(f.burst) }
  if (f.tlsEnabled) {
    cfg.upstream_tls = {
      insecure_skip_verify: f.tlsInsecure || undefined,
      server_name: opt(f.tlsServerName),
    }
  }
  return JSON.parse(JSON.stringify(cfg)) as LoadBalancerConfig
}

const DURATION_RE = /^\d+(\.\d+)?(ms|s|m|h)$/

function toSeconds(d: string): number {
  const m = /^(\d+(?:\.\d+)?)(ms|s|m|h)$/.exec(d)
  if (!m) return Number.NaN
  const unit = { ms: 0.001, s: 1, m: 60, h: 3600 }[
    m[2] as 'ms' | 's' | 'm' | 'h'
  ]
  return Number(m[1]) * unit
}

export type LbFormErrors = Partial<Record<keyof LbFormState, string>>

export function validateForm(f: LbFormState): LbFormErrors {
  const errors: LbFormErrors = {}
  const duration = (key: keyof LbFormState, value: string) => {
    if (value.trim() !== '' && !DURATION_RE.test(value.trim())) {
      errors[key] = 'Use a duration such as 500ms, 5s or 2m'
    }
  }
  if (f.healthEnabled) {
    if (!f.healthPath.startsWith('/'))
      errors.healthPath = 'Path must start with /'
    duration('healthInterval', f.healthInterval)
    duration('healthTimeout', f.healthTimeout)
    if (
      !errors.healthInterval &&
      !errors.healthTimeout &&
      toSeconds(f.healthTimeout) >= toSeconds(f.healthInterval)
    ) {
      errors.healthTimeout = 'Timeout must be shorter than the interval'
    }
    const status = f.healthStatus.trim()
    if (status !== '' && !(Number(status) >= 100 && Number(status) <= 599)) {
      errors.healthStatus = 'Enter an HTTP status code'
    }
  }
  if (f.passiveEnabled) duration('failDuration', f.failDuration)
  if (f.retriesEnabled) {
    duration('tryDuration', f.tryDuration)
    const count = Number(f.retryCount)
    if (!Number.isInteger(count) || count < 0 || count > 10) {
      errors.retryCount = 'Retries must be between 0 and 10'
    }
  }
  duration('slowStart', f.slowStart)
  duration('drainTimeout', f.drainTimeout)
  duration('requestTimeout', f.requestTimeout)
  if (f.rateEnabled && (num(f.rps) ?? 0) < 1) {
    errors.rps = 'Requests per second must be at least 1'
  }
  return errors
}

export const STATE_VARIANT: Record<
  UpstreamState,
  'success' | 'destructive' | 'warning' | 'muted'
> = {
  healthy: 'success',
  unhealthy: 'destructive',
  draining: 'warning',
  disabled: 'muted',
  unknown: 'muted',
}

export function summarizePool(upstreams: { healthy: boolean }[]): string {
  const healthy = upstreams.filter((u) => u.healthy).length
  return `${healthy} of ${upstreams.length} healthy`
}
