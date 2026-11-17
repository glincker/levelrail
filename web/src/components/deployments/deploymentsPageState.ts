import type { BuildRunner } from '../../queries/buildRunners'

export type DeploymentsPageState = 'empty' | 'idle' | 'active'

export interface DeploymentsPageProblems {
  /** The live stream dropped and is retrying. */
  streamDown: boolean
  /** A runner is offline or under pressure. */
  runnerProblem: boolean
}

// empty: nothing ever deployed and nothing running; active wins over history.
export function pageState(input: {
  hasHistory: boolean
  buildsActive: boolean
}): DeploymentsPageState {
  if (input.buildsActive) return 'active'
  return input.hasHistory ? 'idle' : 'empty'
}

// The error overlay: a healthy stream and healthy runners report nothing.
export function pageProblems(input: {
  streamState: 'connecting' | 'live' | 'reconnecting'
  runners: readonly BuildRunner[]
}): DeploymentsPageProblems {
  return {
    streamDown: input.streamState === 'reconnecting',
    runnerProblem: input.runners.some(
      (r) => r.status === 'offline' || r.status === 'pressure',
    ),
  }
}
