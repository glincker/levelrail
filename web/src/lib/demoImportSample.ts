import type { ImportPlan } from '../queries/imports'

// Canned import plan for the read-only demo: the real planner needs a git host, so any
// pasted repo URL gets a believable sample derived from its name. Deploy stays blocked.
export function repoNameFromUrl(text: string): string {
  const cleaned = text
    .trim()
    .replace(/[?#].*$/, '')
    .replace(/\.git$/, '')
    .replace(/\/+$/, '')
  const last = cleaned.split(/[/:]/).pop() ?? ''
  const slug = last
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/, '')
  return slug || 'my-app'
}

export function buildDemoRepoPlan(text: string, ref?: string): ImportPlan {
  const name = repoNameFromUrl(text)
  const repoUrl = /^https?:\/\/|^git@/.test(text.trim())
    ? text.trim()
    : `https://${text.trim()}`
  return {
    source: 'repo',
    suggested_name: name,
    deploy: 'build',
    repo_url: repoUrl,
    ref: ref ?? 'main',
    services: [
      {
        name,
        build: 'nixpacks',
        build_reason: 'Detected a Node.js project with a start script.',
        port: 3000,
        health_path: '/',
        env: [
          {
            key: 'NODE_ENV',
            value: 'production',
            required: false,
            has_default: true,
            secret: false,
          },
        ],
      },
    ],
    domain_suggestion: `${name}.example.com`,
    warnings: [],
    missing_required_env: [],
    resources: [{ kind: 'app', name, detail: 'web service' }],
    sample: {
      buildLog: [
        `Cloning ${repoUrl} (main)`,
        'Detected Node.js 22, package manager npm',
        'npm ci',
        'added 312 packages in 14s',
        'npm run build',
        'Build finished in 21s',
        `Image ${name}:sample built, 142 MB`,
        'Starting container, waiting for health check on /',
        'Healthy after 3s, traffic switched',
      ],
    },
  }
}
