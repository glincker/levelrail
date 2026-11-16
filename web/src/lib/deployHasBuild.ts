import type { DeployAttemptSource } from '../types/deployAttempt'

// Sources that redeploy an existing image: they never run a build, so there is no build log.
const NO_BUILD_SOURCES: readonly DeployAttemptSource[] = [
  'image',
  'restart',
  'start',
  'stop',
  'auto_rollback',
  'promote',
  'clone',
]

/** True only when the attempt's source is known to run a build. */
export function deployHasBuild(source: DeployAttemptSource | undefined) {
  return source !== undefined && !NO_BUILD_SOURCES.includes(source)
}
