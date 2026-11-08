// Pure helpers for the compose import dialog: limits and variable parsing.

const DEFAULT_IMPORT_MAX_BYTES = 256 * 1024

// Mirrors the server's body cap; override at build time without a code change.
export function importTemplateMaxBytes(): number {
  const raw = Number(
    (import.meta.env as Record<string, string | undefined>)
      .VITE_TEMPLATE_IMPORT_MAX_BYTES,
  )
  return Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_IMPORT_MAX_BYTES
}

export const TEMPLATE_ID_RE = /^[a-z0-9][a-z0-9-]{1,62}$/
export const TEMPLATE_VAR_RE = /^[A-Z][A-Z0-9_]{0,63}$/

export function parseVariables(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((v) => v.trim())
    .filter((v) => v !== '')
}

export function slugifyTemplateId(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
}
