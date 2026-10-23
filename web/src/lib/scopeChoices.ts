import type { GlobalEnvironment } from '../types/environment'

/** environmentsFor lists the environments a project can use: its own plus the global ones. */
export function environmentsFor(
  envs: GlobalEnvironment[],
  projectId: string,
): GlobalEnvironment[] {
  if (projectId === '') return envs
  return envs.filter((e) => e.scope === 'global' || e.project_id === projectId)
}

/** defaultEnvironmentFor picks production when the project has several environments, else leaves all selected. */
export function defaultEnvironmentFor(
  envs: GlobalEnvironment[],
  projectId: string,
): string {
  const usable = environmentsFor(envs, projectId)
  if (usable.length < 2) return ''
  return usable.find((e) => e.kind === 'production')?.id ?? ''
}
