import type { AppListEntry } from '../types/appDetail'
import type { DoctorReport } from '../queries/systemDoctor'
import type { HttpsStatus } from '../queries/httpsStatus'
import type { AppPhase } from './firstApp'

export type FirstRunStepId = 'server' | 'domain' | 'deploy'
export type FirstRunStepState =
  'todo' | 'working' | 'done' | 'attention' | 'failed'

export interface FirstRunStep {
  id: FirstRunStepId
  state: FirstRunStepState
  /** Doctor line or generated address preview, empty when there is nothing to add. */
  detail: string
}

export interface FirstRunInput {
  doctor?: DoctorReport
  https?: HttpsStatus
  app?: AppListEntry
  phase: AppPhase
  /** The one address the app can be opened at, when it has one. */
  address?: string
  sampleName: string
}

export interface FirstRunModel {
  steps: FirstRunStep[]
  live: boolean
  address?: string
  appName?: string
  hasDomain: boolean
  /** The step whose action is highlighted; absent once live. */
  primary?: FirstRunStepId
}

function serverStep(doctor: DoctorReport | undefined): FirstRunStep {
  if (!doctor) return { id: 'server', state: 'todo', detail: '' }
  const bad = doctor.checks.find((c) => c.status === 'fail')
  if (bad) return { id: 'server', state: 'failed', detail: bad.message }
  const warn = doctor.checks.find((c) => c.status === 'warn')
  if (warn) return { id: 'server', state: 'attention', detail: warn.message }
  return { id: 'server', state: 'done', detail: '' }
}

function domainStep(input: FirstRunInput, hasDomain: boolean): FirstRunStep {
  const { https } = input
  if (hasDomain) return { id: 'domain', state: 'done', detail: '' }
  if (https?.state === 'pending') {
    return { id: 'domain', state: 'working', detail: https.domain ?? '' }
  }
  if (https?.state === 'failed') {
    return { id: 'domain', state: 'failed', detail: https.error ?? '' }
  }
  if (https?.state === 'issued') {
    return { id: 'domain', state: 'done', detail: https.domain ?? '' }
  }
  const suggested = https?.suggested_domain
  return {
    id: 'domain',
    state: 'todo',
    detail: suggested ? `${input.sampleName}.${suggested}` : '',
  }
}

function deployStep(input: FirstRunInput): FirstRunStep {
  if (input.phase === 'live') return { id: 'deploy', state: 'done', detail: '' }
  if (input.phase === 'failed') {
    return { id: 'deploy', state: 'failed', detail: '' }
  }
  if (input.phase === 'none') return { id: 'deploy', state: 'todo', detail: '' }
  return { id: 'deploy', state: 'working', detail: '' }
}

/** buildFirstRun folds the doctor, HTTPS and app queries into the three first-run steps. */
export function buildFirstRun(input: FirstRunInput): FirstRunModel {
  const hasDomain = (input.app?.domains ?? []).length > 0
  const server = serverStep(input.doctor)
  const domain = domainStep(input, hasDomain)
  const deploy = deployStep(input)
  const live = input.phase === 'live' && Boolean(input.app)
  let primary: FirstRunStepId | undefined
  if (!live) {
    primary = server.state === 'failed' ? 'server' : 'deploy'
  }
  return {
    steps: [server, domain, deploy],
    live,
    address: input.address,
    appName: input.app?.name,
    hasDomain,
    primary,
  }
}
