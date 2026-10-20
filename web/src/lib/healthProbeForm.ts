import { z } from 'zod'
import type { ServiceProbe } from '../types/appDetail'

const NS_PER_SECOND = 1_000_000_000
const STATUS_TOKEN = /^([1-5]\d{2})(?:-([1-5]\d{2}))?$/

// Mirrors internal/probe.ParseStatusSet so the form rejects what the API would.
export function statusSetError(value: string): string | null {
  const tokens = value.split(/[\s,]+/).filter(Boolean)
  if (tokens.length === 0) return 'Enter a status code or range like 200-399'
  for (const token of tokens) {
    const match = STATUS_TOKEN.exec(token)
    if (!match) {
      return `"${token}" is not a status code or range like 200-399`
    }
    if (match[2] && Number(match[1]) > Number(match[2])) {
      return `Range "${token}" runs backwards, write it low-high`
    }
  }
  return null
}

function positiveNumberIssue(value: string, integer: boolean): boolean {
  if (value === '') return false
  const n = Number(value)
  return !Number.isFinite(n) || n <= 0 || (integer && !Number.isInteger(n))
}

export const probeSchema = z
  .object({
    enabled: z.boolean(),
    useExec: z.boolean(),
    path: z.string().trim(),
    https: z.boolean(),
    host: z.string().trim(),
    tlsSkipVerify: z.boolean(),
    followRedirects: z.boolean(),
    expectedStatus: z.string().trim(),
    execCommand: z.string().trim(),
    intervalSeconds: z.string().trim(),
    timeoutSeconds: z.string().trim(),
    failures: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (!data.enabled) return
    const issue = (path: string, message: string) =>
      ctx.addIssue({ code: 'custom', message, path: [path] })

    if (data.useExec) {
      if (!data.execCommand) issue('execCommand', 'Command is required')
    } else {
      if (!data.path) issue('path', 'Path is required')
      else if (!data.path.startsWith('/'))
        issue('path', 'Path must start with /')
      if (data.host && /[\s/]/.test(data.host)) {
        issue('host', 'Host must be a bare hostname or host:port')
      }
      if (data.tlsSkipVerify && !data.https) {
        issue('tlsSkipVerify', 'Skipping TLS verification needs HTTPS')
      }
      if (data.expectedStatus) {
        const err = statusSetError(data.expectedStatus)
        if (err) issue('expectedStatus', err)
      }
    }
    if (positiveNumberIssue(data.intervalSeconds, false)) {
      issue('intervalSeconds', 'Interval must be a positive number of seconds')
    }
    if (positiveNumberIssue(data.timeoutSeconds, false)) {
      issue('timeoutSeconds', 'Timeout must be a positive number of seconds')
    }
    if (positiveNumberIssue(data.failures, true)) {
      issue('failures', 'Failure threshold must be a positive whole number')
    }
  })

export type ProbeFormValues = z.infer<typeof probeSchema>

// An argv of the ["/bin/sh", "-c", script] shape edits as its script;
// any other argv edits as its space-joined words and saves back as a shell command.
function execToCommand(exec: string[] | undefined): string {
  if (!exec || exec.length === 0) return ''
  if (
    exec.length === 3 &&
    (exec[0] === '/bin/sh' || exec[0] === 'sh') &&
    exec[1] === '-c'
  ) {
    return exec[2] ?? ''
  }
  return exec.join(' ')
}

function secondsField(ns?: number): string {
  return ns && ns > 0 ? String(ns / NS_PER_SECOND) : ''
}

export function toProbeFieldValues(
  probe?: ServiceProbe | null,
): ProbeFormValues {
  return {
    enabled: !!probe,
    useExec: !!probe?.exec?.length,
    path: probe?.path ?? '',
    https: probe?.scheme === 'https',
    host: probe?.host ?? '',
    tlsSkipVerify: probe?.tls_skip_verify ?? false,
    followRedirects: probe?.follow_redirects ?? true,
    expectedStatus: probe?.expected_status ?? '',
    execCommand: execToCommand(probe?.exec),
    intervalSeconds: secondsField(probe?.interval),
    timeoutSeconds: secondsField(probe?.timeout),
    failures:
      probe?.failures && probe.failures > 0 ? String(probe.failures) : '',
  }
}

export function toProbe(values: ProbeFormValues): ServiceProbe | null {
  if (!values.enabled) return null
  const probe: ServiceProbe = values.useExec
    ? { path: '', exec: ['/bin/sh', '-c', values.execCommand.trim()] }
    : {
        path: values.path.trim(),
        follow_redirects: values.followRedirects,
        ...(values.https ? { scheme: 'https' as const } : {}),
        ...(values.https && values.tlsSkipVerify
          ? { tls_skip_verify: true }
          : {}),
        ...(values.host ? { host: values.host.trim() } : {}),
        ...(values.expectedStatus
          ? { expected_status: values.expectedStatus.trim() }
          : {}),
      }
  if (values.intervalSeconds !== '') {
    probe.interval = Math.round(Number(values.intervalSeconds) * NS_PER_SECOND)
  }
  if (values.timeoutSeconds !== '') {
    probe.timeout = Math.round(Number(values.timeoutSeconds) * NS_PER_SECOND)
  }
  if (values.failures !== '')
    probe.failures = Math.round(Number(values.failures))
  return probe
}

// Short human summary of a saved probe for the card's "Currently:" line.
export function describeProbe(probe: ServiceProbe): string {
  if (probe.exec?.length) return `exec ${execToCommand(probe.exec)}`
  const scheme = probe.scheme ?? 'http'
  const status = probe.expected_status ?? '200-299'
  return `${scheme.toUpperCase()} ${probe.path}, expects ${status}`
}
