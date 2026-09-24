import { describe, expect, it } from 'vitest'
import {
  logoIdForBackupTarget,
  logoIdForFramework,
  logoIdForIntegration,
} from './brandLogos'
import { TEMPLATE_LOGO_LOADERS } from './templateLogos'

describe('logoIdForFramework', () => {
  it.each([
    ['Next.js', 'nextdotjs'],
    ['nextjs', 'nextdotjs'],
    ['Python (Django)', 'django'],
    ['Ruby on Rails', 'ruby-on-rails'],
    ['Go', 'go'],
    ['Node.js', 'nodedotjs'],
    ['Nuxt', 'nuxt'],
  ])('%s -> %s', (input, want) => {
    expect(logoIdForFramework(input)).toBe(want)
  })

  it('returns undefined for unknown or empty input', () => {
    expect(logoIdForFramework('Cobol')).toBeUndefined()
    expect(logoIdForFramework('')).toBeUndefined()
    expect(logoIdForFramework(undefined)).toBeUndefined()
  })
})

describe('logoIdForIntegration', () => {
  it('maps catalog keys and skips unknown ones', () => {
    expect(logoIdForIntegration('posthog')).toBe('posthog')
    expect(logoIdForIntegration('betterstack')).toBe('better-stack')
    expect(logoIdForIntegration('logsnag')).toBeUndefined()
  })
})

describe('logoIdForBackupTarget', () => {
  it('maps providers and endpoint hosts', () => {
    expect(logoIdForBackupTarget('aws')).toBe('aws-s3')
    expect(
      logoIdForBackupTarget('r2', 'https://x.r2.cloudflarestorage.com'),
    ).toBe('cloudflare')
    expect(
      logoIdForBackupTarget('custom', 'https://s3.us-west-004.backblazeb2.com'),
    ).toBe('backblaze')
    expect(
      logoIdForBackupTarget('custom', 'https://storage.googleapis.com'),
    ).toBe('google-cloud-storage')
    expect(logoIdForBackupTarget('custom', 'http://minio.local:9000')).toBe(
      'minio',
    )
    expect(logoIdForBackupTarget('custom', 'https://s3.example.com')).toBe(
      undefined,
    )
    expect(logoIdForBackupTarget('custom')).toBeUndefined()
  })
})

describe('every mapped id has a loader', () => {
  it('resolves', () => {
    const ids = [
      ...[
        'Next.js',
        'Django',
        'Rails',
        'Laravel',
        'Flask',
        'FastAPI',
        'Express',
        'Nuxt',
        'Remix',
        'Astro',
        'Svelte',
        'Angular',
        'Vue',
        'Vite',
        'React',
        'Node',
        'Go',
        'Python',
        'Ruby',
        'PHP',
        'Rust',
        'Deno',
        'Bun',
        'Java',
      ].map(logoIdForFramework),
      ...[
        'sentry',
        'posthog',
        'datadog',
        'axiom',
        'betterstack',
        'bugsnag',
        'newrelic',
      ].map(logoIdForIntegration),
      logoIdForBackupTarget('aws'),
      logoIdForBackupTarget('r2'),
      logoIdForBackupTarget('custom', 'backblazeb2.com'),
      logoIdForBackupTarget('custom', 'googleapis.com'),
      logoIdForBackupTarget('custom', 'minio'),
    ]
    for (const id of ids) {
      expect(id).toBeDefined()
      expect(TEMPLATE_LOGO_LOADERS).toHaveProperty(id as string)
    }
  })
})
