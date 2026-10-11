import type { ImportAnalysis } from '../queries/importAnalyze'

export function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response
}

/** What the analyze endpoint returns for a Next.js app with .env.example and pg. */
export const nextAnalysis: ImportAnalysis = {
  repo_url: 'https://github.com/acme/shop.git',
  branch: 'main',
  monorepo: false,
  name: {
    value: 'shop',
    evidence: 'derived from the repository name shop',
  },
  stack: {
    runtime: 'node',
    framework: 'Next.js',
    evidence: 'next dependency found, built with Railpack',
  },
  build: {
    type: 'railpack',
    evidence: 'no Dockerfile found, the framework is auto-detected',
    alternatives: [],
  },
  port: { value: 3000, evidence: 'Next.js default port' },
  health: { value: '/', evidence: 'Next.js default health path' },
  memory: { bytes: 768 * 1024 * 1024, evidence: '768 MiB is a safe start' },
  domain: {
    value: 'shop.apps.example.com',
    evidence: '<app>.apps.example.com from the apps base domain',
  },
  env: [
    {
      key: 'DATABASE_URL',
      required: false,
      secret: true,
      source: '.env.example',
      provided_by: 'database:shop-postgres',
      has_default: true,
    },
    {
      key: 'NEXTAUTH_SECRET',
      required: true,
      secret: true,
      source: '.env.example',
      has_default: false,
    },
    {
      key: 'NEXT_PUBLIC_URL',
      required: false,
      secret: false,
      source: '.env.example',
      has_default: true,
    },
  ],
  databases: [
    {
      engine: 'postgres',
      version: '16',
      suggested_name: 'shop-postgres',
      env_var: 'DATABASE_URL',
      field: 'url',
      evidence:
        'DATABASE_URL declared in .env.example, pg found in the dependency manifest',
    },
  ],
  warnings: [],
  files_read: 3,
  tree_truncated: false,
}
