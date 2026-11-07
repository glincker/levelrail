// Azure DevOps connections deploy on push only: no PR previews, no commit statuses.
const PUSH_ONLY_PROVIDERS: readonly string[] = ['azure_devops']

export function supportsPrFeatures(provider: string | undefined): boolean {
  return provider === undefined || !PUSH_ONLY_PROVIDERS.includes(provider)
}
