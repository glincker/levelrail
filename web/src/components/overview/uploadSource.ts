import type { DeployAttempt } from '../../types/deployAttempt'

const FILE_COUNT = /,\s*(\d+)\s+files?/

/** File count from the "Uploaded from CLI, N files, hash" message the server records. */
export function uploadFileCount(message?: string): number | null {
  const m = message ? FILE_COUNT.exec(message) : null
  return m ? Number(m[1]) : null
}

/** Latest upload attempt, or undefined when the app was never deployed from a directory. */
export function latestUpload(
  attempts: DeployAttempt[],
): DeployAttempt | undefined {
  return attempts.find((a) => a.source === 'upload')
}
