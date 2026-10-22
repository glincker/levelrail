import type { TemplateEnvPrompt, TemplateInstallInput } from '../types/compose'

export type EnvFieldKind = 'required' | 'generated' | 'optional'

export interface EnvFormGroups {
  required: TemplateEnvPrompt[]
  generated: TemplateEnvPrompt[]
  optional: TemplateEnvPrompt[]
}

export const APP_NAME_PATTERN = /^[a-z][a-z0-9-]{0,39}$/
const DOMAIN_PATTERN =
  /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/

/** envFieldKind: generated wins, then a required prompt with no default, else optional. */
export function envFieldKind(p: TemplateEnvPrompt): EnvFieldKind {
  if (p.generate) return 'generated'
  return p.required && !p.default ? 'required' : 'optional'
}

export function groupEnvPrompts(prompts: TemplateEnvPrompt[]): EnvFormGroups {
  const groups: EnvFormGroups = { required: [], generated: [], optional: [] }
  for (const p of prompts) groups[envFieldKind(p)].push(p)
  return groups
}

export interface FormValues {
  name: string
  env: Record<string, string>
  customDomain: boolean
  domains: Record<string, string>
}

export type FormErrors = Record<string, string>

/** validateTemplateForm returns an error code per field key ("name", "env:KEY", "domain:svc"). */
export function validateTemplateForm(
  prompts: TemplateEnvPrompt[],
  values: FormValues,
): FormErrors {
  const errors: FormErrors = {}
  if (values.name && !APP_NAME_PATTERN.test(values.name)) errors.name = 'name'
  for (const p of prompts) {
    if (envFieldKind(p) === 'required' && !values.env[p.key]?.trim()) {
      errors[`env:${p.key}`] = 'required'
    }
  }
  if (values.customDomain) {
    for (const [service, host] of Object.entries(values.domains)) {
      if (host && !DOMAIN_PATTERN.test(host)) {
        errors[`domain:${service}`] = 'domain'
      }
    }
  }
  return errors
}

/** buildInstallPayload drops empty fields so the server applies its defaults and generators. */
export function buildInstallPayload(values: FormValues): TemplateInstallInput {
  const pick = (m: Record<string, string>) =>
    Object.fromEntries(Object.entries(m).filter(([, v]) => v.trim() !== ''))
  return {
    name: values.name || undefined,
    env: pick(values.env),
    domains: values.customDomain ? pick(values.domains) : {},
  }
}

/** randomSecret makes a client-side value the user can see; the server fills any field left empty. */
export function randomSecret(length = 24): string {
  const bytes = new Uint8Array(length)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0'))
    .join('')
    .slice(0, length)
}
