import type { Domain } from '../../../queries/domains'
import type { GitRepoItem } from '../../../types/gitRepos'

function envNumber(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

export const DEFAULT_IDLE_MINUTES = envNumber(
  import.meta.env.VITE_FUNCTION_DEFAULT_IDLE_MINUTES,
  10,
)
export const MIN_IDLE_MINUTES = envNumber(
  import.meta.env.VITE_FUNCTION_MIN_IDLE_MINUTES,
  5,
)
export const FALLBACK_PORT = 8080
export const NAME_MAX_LENGTH = 40

export const IDLE_CHOICES = ['5', '10', '60', '1440', 'custom'] as const
export type IdleChoice = (typeof IDLE_CHOICES)[number]

const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/
const IMAGE_PATTERN = /^[^\s:@]+([:@]\S+)?$/

export interface FlowValues {
  source: 'repo' | 'image'
  repoUrl: string
  ref: string
  directory: string
  image: string
  name: string
  port: string
  idleChoice: IdleChoice
  customIdle: string
  routeDomain: string
  routePath: string
}

export type ValidationKey =
  | 'validation.repoRequired'
  | 'validation.repoInvalid'
  | 'validation.imageRequired'
  | 'validation.imageInvalid'
  | 'validation.nameRequired'
  | 'validation.nameInvalid'
  | 'validation.nameTooLong'
  | 'validation.nameTaken'
  | 'validation.portInvalid'
  | 'validation.idleInvalid'
  | 'validation.domainRequired'
  | 'validation.pathInvalid'

export type FieldErrors = Partial<Record<keyof FlowValues, ValidationKey>>

export function toKebab(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/\.git$/, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, NAME_MAX_LENGTH)
    .replace(/-+$/g, '')
}

export function nameFromRepoUrl(url: string): string {
  const trimmed = url.trim().replace(/\/+$/, '')
  const last = trimmed.split(/[/:]/).pop() ?? ''
  return toKebab(last)
}

export function nameFromImage(image: string): string {
  const last = image.trim().split('/').pop() ?? ''
  return toKebab(last.split(/[:@]/)[0] ?? '')
}

export function suggestName(input: {
  source: 'repo' | 'image'
  repoUrl: string
  image: string
  presetName: string
}): string {
  const fromSource =
    input.source === 'image'
      ? nameFromImage(input.image)
      : nameFromRepoUrl(input.repoUrl)
  return fromSource || toKebab(input.presetName)
}

export function normalizePath(raw: string): string {
  const p = raw.trim()
  if (p === '') return ''
  const withSlash = p.startsWith('/') ? p : `/${p}`
  return withSlash.length > 1 ? withSlash.replace(/\/+$/, '') : withSlash
}

export function idleMinutes(v: Pick<FlowValues, 'idleChoice' | 'customIdle'>) {
  if (v.idleChoice === 'custom') return Number(v.customIdle)
  return Number(v.idleChoice)
}

export function isHttpUrl(raw: string): boolean {
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'https:' || u.protocol === 'http:'
  } catch {
    return false
  }
}

function validateSource(v: FlowValues, e: FieldErrors) {
  if (v.source === 'repo') {
    if (v.repoUrl.trim() === '') e.repoUrl = 'validation.repoRequired'
    else if (!isHttpUrl(v.repoUrl)) e.repoUrl = 'validation.repoInvalid'
    return
  }
  const image = v.image.trim()
  if (image === '') e.image = 'validation.imageRequired'
  else if (!IMAGE_PATTERN.test(image)) e.image = 'validation.imageInvalid'
}

/** Returns i18n keys under the functions namespace, one per invalid field. */
export function validate(v: FlowValues, nameTaken = false): FieldErrors {
  const e: FieldErrors = {}
  validateSource(v, e)

  const name = v.name.trim()
  if (name === '') e.name = 'validation.nameRequired'
  else if (!NAME_PATTERN.test(name)) e.name = 'validation.nameInvalid'
  else if (name.length > NAME_MAX_LENGTH) e.name = 'validation.nameTooLong'
  else if (nameTaken) e.name = 'validation.nameTaken'

  const port = Number(v.port)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    e.port = 'validation.portInvalid'
  }
  if (v.idleChoice === 'custom') {
    const mins = Number(v.customIdle)
    if (!Number.isInteger(mins) || mins < MIN_IDLE_MINUTES) {
      e.customIdle = 'validation.idleInvalid'
    }
  }
  const path = normalizePath(v.routePath)
  if (path !== '' && v.routeDomain === '') {
    e.routeDomain = 'validation.domainRequired'
  }
  if (path === '/' || (path !== '' && !/^\/[\w.~/-]+$/.test(path))) {
    e.routePath = 'validation.pathInvalid'
  }
  return e
}

export function routableDomains(domains: Domain[]): Domain[] {
  return domains.filter((d) => d.kind !== 'standalone' && d.service_name !== '')
}

/** Derives the generated host pattern from any automatic domain on the server. */
export function functionHost(name: string, domains: Domain[]): string | null {
  const auto = domains.find((d) => d.automatic)
  if (!auto || name === '') return null
  const rest = auto.domain.split('.').slice(1).join('.')
  return rest ? `${name}.${rest}` : null
}

export interface UrlPreview {
  functionUrl: string | null
  routeUrl: string | null
}

export function previewUrls(input: {
  name: string
  routeDomain: string
  routePath: string
  domains: Domain[]
}): UrlPreview {
  const host = functionHost(input.name.trim(), input.domains)
  const path = normalizePath(input.routePath)
  return {
    functionUrl: host ? `https://${host}` : null,
    routeUrl:
      input.routeDomain && path ? `https://${input.routeDomain}${path}` : null,
  }
}

export function sortRecentFirst(items: GitRepoItem[]): GitRepoItem[] {
  return [...items].sort((a, b) =>
    (b.pushed_at ?? '').localeCompare(a.pushed_at ?? ''),
  )
}
