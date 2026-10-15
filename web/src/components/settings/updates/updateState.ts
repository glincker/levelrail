export type UpdateState =
  | 'upToDate'
  | 'available'
  | 'scheduled'
  | 'inProgress'
  | 'checkFailed'
  | 'noReleases'
  | 'hostBinary'

export interface UpdateStateInput {
  latestVersion: string | null
  updateAvailable: boolean
  hostBinaryEdge: boolean
  scheduleActive: boolean
  upgradeRunning: boolean
  checkFailed: boolean
}

export function deriveUpdateState(i: UpdateStateInput): UpdateState {
  if (i.upgradeRunning) return 'inProgress'
  if (i.checkFailed) return 'checkFailed'
  if (i.hostBinaryEdge) return 'hostBinary'
  if (i.scheduleActive) return 'scheduled'
  if (i.latestVersion === null) return 'noReleases'
  return i.updateAvailable ? 'available' : 'upToDate'
}
