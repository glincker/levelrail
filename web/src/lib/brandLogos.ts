import type { BackupProvider } from '../types/backupTarget'

const FRAMEWORK_MATCHERS: Array<[RegExp, string]> = [
  [/next\.?js/, 'nextdotjs'],
  [/django/, 'django'],
  [/rails/, 'ruby-on-rails'],
  [/laravel/, 'laravel'],
  [/flask/, 'flask'],
  [/fastapi/, 'fastapi'],
  [/express/, 'express'],
  [/nuxt/, 'nuxt'],
  [/remix/, 'remix'],
  [/astro/, 'astro'],
  [/svelte/, 'svelte'],
  [/angular/, 'angular'],
  [/vue/, 'vue'],
  [/vite/, 'vite'],
  [/react/, 'react'],
  [/node/, 'nodedotjs'],
  [/^go(lang)?\b/, 'go'],
  [/python/, 'python'],
  [/ruby/, 'ruby'],
  [/php/, 'php'],
  [/rust/, 'rust'],
  [/deno/, 'deno'],
  [/bun/, 'bun'],
  [/^java(?!script)/, 'openjdk'],
]

// Maps a detected framework label ("Next.js", "Python (Django)", "nextjs")
// to a logo id, or undefined when no mark exists.
export function logoIdForFramework(
  framework: string | undefined,
): string | undefined {
  const name = framework?.trim().toLowerCase()
  if (!name) return undefined
  return FRAMEWORK_MATCHERS.find(([re]) => re.test(name))?.[1]
}

const INTEGRATION_LOGOS: Record<string, string> = {
  sentry: 'sentry',
  posthog: 'posthog',
  datadog: 'datadog',
  axiom: 'axiom',
  betterstack: 'better-stack',
  bugsnag: 'bugsnag',
  newrelic: 'new-relic',
}

// Maps an integration catalog key to a logo id. logsnag has no mark.
export function logoIdForIntegration(key: string): string | undefined {
  return INTEGRATION_LOGOS[key.toLowerCase()]
}

// Chooses a backup target's mark from its provider, refined by endpoint
// host for the S3-compatible "custom" provider (B2, MinIO, GCS).
export function logoIdForBackupTarget(
  provider: BackupProvider,
  endpoint?: string,
): string | undefined {
  if (provider === 'aws') return 'aws-s3'
  if (provider === 'r2') return 'cloudflare'
  const host = (endpoint ?? '').toLowerCase()
  if (host.includes('backblazeb2.com')) return 'backblaze'
  if (host.includes('googleapis.com')) return 'google-cloud-storage'
  if (host.includes('minio')) return 'minio'
  if (host.includes('r2.cloudflarestorage.com')) return 'cloudflare'
  if (host.includes('amazonaws.com')) return 'aws-s3'
  return undefined
}
