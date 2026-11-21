import type { AwsEcrLoginToken, AwsServiceName } from '../types/cloudAccounts'

// Runtime setting key per service card; the parameters key backs the secrets route.
export const AWS_SERVICE_SETTING: Record<AwsServiceName, string> = {
  ecr: 'cloud.guard.service.ecr',
  acm: 'cloud.guard.service.acm',
  secrets: 'cloud.guard.service.parameters',
  cloudtrail: 'cloud.guard.service.cloudtrail',
}

// The token is a short lived credential: it goes to the clipboard only, never to the page.
export function ecrLoginCommand(t: AwsEcrLoginToken): string {
  return `echo '${t.password}' | docker login --username ${t.user} --password-stdin ${t.registry}`
}

export function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}
