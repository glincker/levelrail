import { APP_NAME_PATTERN, editsFromAnalysis, type NewAppEdits } from './newApp'
import type { ImportAnalysis } from '../queries/importAnalyze'

const REPO_URL =
  /^(?:(?:https?|ssh|git):\/\/[^\s/]+\/[^\s/]+\/\S+|[\w.-]+@[\w.-]+:\S+\/\S+)$/i
const DOMAIN =
  /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i
const BRAND_HOSTS: Record<string, string> = {
  'github.com': 'github',
  'gitlab.com': 'gitlab',
  'bitbucket.org': 'bitbucket',
  'codeberg.org': 'codeberg',
}

export function looksLikeRepoUrl(raw: string): boolean {
  return REPO_URL.test(raw.trim())
}

export function repoNameFromUrl(raw: string): string {
  const path = raw
    .trim()
    .replace(/[?#].*$/, '')
    .replace(/\.git$/, '')
  const parts = path.split(/[/:]/).filter(Boolean)
  return parts[parts.length - 1] ?? ''
}

export function hostBrand(host: string | undefined): string {
  if (!host) return 'git'
  const h = host.toLowerCase().replace(/^www\./, '')
  if (BRAND_HOSTS[h]) return BRAND_HOSTS[h]
  if (h.startsWith('gitlab.')) return 'gitlab'
  if (h.startsWith('gitea.') || h.startsWith('forgejo.')) return 'gitea'
  return 'git'
}

export function suggestAppName(repoName: string): string {
  const slug = repoName
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/g, '')
  return slug
}

export function validateAppName(name: string, taken: ReadonlySet<string>) {
  if (name.trim() === '') return 'required'
  if (!APP_NAME_PATTERN.test(name)) return 'format'
  if (taken.has(name)) return 'taken'
  return null
}

export function isValidDomain(value: string): boolean {
  return DOMAIN.test(value.trim())
}

/** Env keys the operator must supply: required and not filled by a managed database. */
export function neededEnvKeys(a: ImportAnalysis): ImportAnalysis['env'] {
  const dbVars = new Set(a.databases.map((d) => d.env_var))
  return a.env.filter((k) => k.required && !k.provided_by && !dbVars.has(k.key))
}

export interface SimpleFormValues {
  name: string
  env: Record<string, string>
  customDomain: string
  advanced: Partial<
    Pick<
      NewAppEdits,
      'port' | 'healthPath' | 'memoryMb' | 'buildType' | 'baseDirectory'
    >
  >
}

/** Detected defaults overlaid with the few things the operator typed. */
export function editsFromForm(
  a: ImportAnalysis,
  v: SimpleFormValues,
): NewAppEdits {
  const base = editsFromAnalysis(a)
  const custom = v.customDomain.trim()
  return {
    ...base,
    ...v.advanced,
    name: v.name.trim(),
    domain: custom !== '' ? custom : base.domain,
    env: Object.fromEntries(
      Object.entries(v.env).filter(([k, val]) => k.trim() !== '' && val !== ''),
    ),
  }
}

export function stackLabel(a: ImportAnalysis): string {
  return a.stack.framework || a.detection?.framework || a.stack.runtime
}
