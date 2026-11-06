export type FirstRunStepId = 'deploy' | 'domain' | 'https'

export interface FirstRunStep {
  id: FirstRunStepId
  done: boolean
  current: boolean
}

// Derived from data the page already loads: the first unfinished step is current.
export function firstRunSteps(input: {
  appCount: number
  domainCount: number
  httpsOff: boolean | undefined
}): FirstRunStep[] {
  const done = [
    input.appCount > 0,
    input.domainCount > 0,
    input.httpsOff === false,
  ]
  const ids: FirstRunStepId[] = ['deploy', 'domain', 'https']
  const currentIdx = done.indexOf(false)
  return ids.map((id, i) => ({
    id,
    done: done[i] ?? false,
    current: i === currentIdx,
  }))
}
