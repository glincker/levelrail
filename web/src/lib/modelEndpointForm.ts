import type { EndpointApiType } from '../types/modelEndpoints'

const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$/
const OLLAMA_PORT = '11434'

function parseUrl(raw: string): URL | null {
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'http:' || u.protocol === 'https:' ? u : null
  } catch {
    return null
  }
}

export function isValidEndpointUrl(raw: string): boolean {
  const u = parseUrl(raw)
  return u !== null && u.hostname !== '' && u.username === ''
}

// Ollama's default port is a strong hint, anything else is probably OpenAI-compatible.
export function inferApiType(raw: string): EndpointApiType {
  return parseUrl(raw)?.port === OLLAMA_PORT ? 'ollama' : 'openai'
}

// A name derived from the host, so the field can be left empty.
export function suggestEndpointName(raw: string): string {
  const host = parseUrl(raw)?.hostname.replace(/^\[|\]$/g, '')
  if (!host) return ''
  const slug = host
    .toLowerCase()
    .replace(/\.local$/, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 36)
  if (slug === '') return ''
  return /^[0-9]/.test(slug) ? `ep-${slug}` : slug
}

export function isValidEndpointName(name: string): boolean {
  return NAME_PATTERN.test(name)
}

export interface EndpointFormErrors {
  name?: true
  url?: true
}

export function endpointFormErrors(form: {
  name: string
  url: string
}): EndpointFormErrors {
  const errors: EndpointFormErrors = {}
  const name = form.name.trim() || suggestEndpointName(form.url)
  if (!isValidEndpointName(name)) errors.name = true
  if (!isValidEndpointUrl(form.url)) errors.url = true
  return errors
}
