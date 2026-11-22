import type { AppDetail } from '../../types/appDetail'

/** Distinct database names this app is linked to, from env bindings and the single attachment. */
export function linkedDatabases(app: AppDetail): string[] {
  const names = new Set<string>()
  for (const ref of Object.values(app.database_env ?? {})) {
    names.add(ref.database)
  }
  if (app.database_attachment?.database_name) {
    names.add(app.database_attachment.database_name)
  }
  return [...names]
}

/** Registry host of an image reference; null for a bare Docker Hub style name. */
export function registryHost(image: string): string | null {
  const first = image.split('/')[0] ?? ''
  return image.includes('/') && /[.:]|^localhost$/.test(first) ? first : null
}
