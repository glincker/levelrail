export type SuggestionKind =
  'app-action' | 'failing-app' | 'recent-app' | 'assistant'

export interface SuggestionSpec {
  key: string
  kind: SuggestionKind
  label: string
  app?: string
  action?: 'restart' | 'deploy'
}

const MAX_FAILING = 3
const MAX_RECENT = 3

// Order: current app actions, failing apps, recent apps, then the assistant.
export function buildPaletteSuggestions(input: {
  currentApp?: string
  apps: { name: string; failing: boolean }[]
  recentKeys: readonly string[]
}): SuggestionSpec[] {
  const out: SuggestionSpec[] = []
  const seen = new Set<string>()
  const { currentApp } = input
  if (currentApp) {
    seen.add(currentApp)
    out.push(
      {
        key: `suggest-restart-${currentApp}`,
        kind: 'app-action',
        label: `Restart ${currentApp}`,
        app: currentApp,
        action: 'restart',
      },
      {
        key: `suggest-deploy-${currentApp}`,
        kind: 'app-action',
        label: `Deploy ${currentApp}`,
        app: currentApp,
        action: 'deploy',
      },
    )
  }
  for (const app of input.apps
    .filter((a) => a.failing && a.name !== currentApp)
    .slice(0, MAX_FAILING)) {
    seen.add(app.name)
    out.push({
      key: `suggest-failing-${app.name}`,
      kind: 'failing-app',
      label: `${app.name} needs attention`,
      app: app.name,
    })
  }
  const known = new Set(input.apps.map((a) => a.name))
  const recents = input.recentKeys
    .filter((k) => k.startsWith('app-') && !k.startsWith('app-action-'))
    .map((k) => k.slice('app-'.length))
    .filter((n) => known.has(n) && !seen.has(n))
    .slice(0, MAX_RECENT)
  for (const name of recents) {
    out.push({
      key: `suggest-recent-${name}`,
      kind: 'recent-app',
      label: name,
      app: name,
    })
  }
  out.push({
    key: 'suggest-assistant',
    kind: 'assistant',
    label: 'Ask the assistant',
  })
  return out
}
