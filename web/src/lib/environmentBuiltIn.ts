import type { GlobalEnvironment } from '../types/environment'

const BUILT_IN_IDS = ['env_dev', 'env_test', 'env_uat', 'env_production']

// The four seeded environments the server refuses to delete.
export function isBuiltIn(environment?: GlobalEnvironment): boolean {
  return environment !== undefined && BUILT_IN_IDS.includes(environment.id)
}
