import { z } from 'zod'
import type { TriggerBuildInput } from '../queries/builds'
import { setGitSource } from '../queries/gitSources'
import { connectGitHubRepoAsSource } from '../queries/githubApp'
import { connectRepoAsSource } from '../queries/gitConnections'
import type { GitSourceBuildType, GitSourceResource } from '../types/gitSource'
import { HEALTH_CHECK_DEFAULT_PATH } from './healthCheckDefaults'
import type { GitRepoSourceValue } from '@/components/GitRepoSourcePicker'

// Build packs this form offers, matching GitBuildSourceFields' own tab
// picker for what POST /api/v1/apps/{name}/builds actually supports:
// dockerfile, railpack, static, image. See that component's doc comment
// for why Nixpacks and compose are never offered.
const BUILD_TYPES = ['dockerfile', 'railpack', 'static', 'image'] as const

// Same pattern DomainEditor.tsx validates a domain row against; kept as
// a local copy rather than exported/imported from there since this form
// only ever needs one domain, not that component's add/remove list.
const DOMAIN_PATTERN =
  /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i

// Mirrors validateAppResource plus the git fields POST .../builds requires.
// repoUrl/ref are required for every build type except image, and image is the reverse.
export const createAppFromGitSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    port: z.coerce
      .number({ error: 'Port is required' })
      .int('Port must be a whole number')
      .positive('Port must be a positive integer'),
    repoUrl: z.string().trim(),
    ref: z.string().trim(),
    imageRepo: z.string().trim(),
    buildType: z.enum(BUILD_TYPES),
    dockerfilePath: z.string().trim(),
    baseDirectory: z.string().trim(),
    // Dockerfile build-time ARGs, buildType: 'dockerfile' only. A blank
    // key is dropped in buildInputFrom below, not rejected here.
    buildArgs: z.array(z.object({ key: z.string(), value: z.string() })),
    image: z.string().trim(),
    domain: z
      .string()
      .trim()
      .refine(
        (v) => v === '' || DOMAIN_PATTERN.test(v),
        'Enter a valid domain, e.g. app.example.com',
      ),
    healthCheckEnabled: z.boolean(),
    healthCheckPath: z.string().trim(),
  })
  .superRefine((values, ctx) => {
    if (values.buildType === 'image') {
      if (!values.image.trim()) {
        ctx.addIssue({
          code: 'custom',
          message: 'Image reference is required',
          path: ['image'],
        })
      }
      return
    }
    if (!values.repoUrl.trim()) {
      ctx.addIssue({
        code: 'custom',
        message: 'Repository URL is required',
        path: ['repoUrl'],
      })
    }
    if (!values.ref.trim()) {
      ctx.addIssue({
        code: 'custom',
        message: 'Branch, tag, or commit is required',
        path: ['ref'],
      })
    }
    if (values.buildType === 'dockerfile') {
      const seen = new Set<string>()
      values.buildArgs.forEach((arg, index) => {
        const key = arg.key.trim()
        if (!key) return
        if (seen.has(key)) {
          ctx.addIssue({
            code: 'custom',
            message: 'Duplicate key',
            path: ['buildArgs', index, 'key'],
          })
        }
        seen.add(key)
      })
    }
  })
  .superRefine((values, ctx) => {
    if (!values.healthCheckEnabled) return
    if (!values.healthCheckPath) {
      ctx.addIssue({
        code: 'custom',
        message: 'Health check path is required',
        path: ['healthCheckPath'],
      })
    } else if (!values.healthCheckPath.startsWith('/')) {
      ctx.addIssue({
        code: 'custom',
        message: 'Path must start with /',
        path: ['healthCheckPath'],
      })
    }
  })

export type FormInput = z.input<typeof createAppFromGitSchema>
export type FormOutput = z.output<typeof createAppFromGitSchema>

export const DEFAULT_VALUES: FormInput = {
  name: '',
  port: '',
  repoUrl: '',
  ref: '',
  imageRepo: '',
  buildType: 'railpack',
  dockerfilePath: '',
  baseDirectory: '',
  buildArgs: [],
  image: '',
  domain: '',
  // Mirrors the CLI's interactive wizard default
  // (apps_create_interactive.go): on by default at the documented
  // /healthz convention.
  healthCheckEnabled: true,
  healthCheckPath: HEALTH_CHECK_DEFAULT_PATH,
}

// build.path is dockerfile-only and image_repo is skipped for static, as the
// server rejects them otherwise; image builds send only the image.
export function buildInputFrom(
  values: FormOutput,
  detectedFramework: string | undefined,
): TriggerBuildInput {
  if (values.buildType === 'image') {
    return {
      repoUrl: '',
      ref: '',
      buildType: 'image',
      image: values.image.trim(),
    }
  }
  return {
    repoUrl: values.repoUrl.trim(),
    ref: values.ref.trim(),
    imageRepo:
      values.buildType === 'static'
        ? undefined
        : values.imageRepo.trim() || undefined,
    buildType: values.buildType,
    buildPath:
      values.buildType === 'dockerfile'
        ? values.dockerfilePath.trim() || undefined
        : undefined,
    baseDirectory: values.baseDirectory.trim() || undefined,
    buildArgs:
      values.buildType === 'dockerfile'
        ? buildArgsRecord(values.buildArgs)
        : undefined,
    detectedFramework,
  }
}

// Blank-key rows are dropped, not rejected, matching the schema duplicate check.
function buildArgsRecord(
  args: FormOutput['buildArgs'],
): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const { key, value } of args) {
    const trimmedKey = key.trim()
    if (trimmedKey) out[trimmedKey] = value
  }
  return Object.keys(out).length > 0 ? out : undefined
}

// Suggests an app name from a clone URL; only used to prefill a blank Name.
export function repoSlugFrom(repoUrl: string): string {
  const cleaned = repoUrl
    .trim()
    .replace(/\.git$/i, '')
    .replace(/\/+$/, '')
  const segments = cleaned.split(/[/:]/).filter(Boolean)
  const last = segments[segments.length - 1] ?? ''
  return last
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// Mirrors the build-pack choice into git_source so webhook builds match the first one.
function gitSourceBuildFields(values: FormOutput): {
  buildType: GitSourceBuildType
  buildPath?: string
} {
  const buildType = values.buildType as GitSourceBuildType
  return {
    buildType,
    buildPath:
      buildType === 'dockerfile'
        ? values.dockerfilePath.trim() || undefined
        : undefined,
  }
}

// Connects a git_source row between app creation and the first build, using the
// provider endpoint (webhook auto-registered) only if the form still matches the pick.
export async function connectGitSourceFor(
  name: string,
  values: FormOutput,
  source: GitRepoSourceValue | null,
): Promise<{
  resource: GitSourceResource
  autoRegistered: boolean
  webhookError?: string
}> {
  const repoUrl = values.repoUrl.trim()
  const branch = values.ref.trim()
  const { buildType, buildPath } = gitSourceBuildFields(values)
  const effective =
    source && source.repoUrl === repoUrl && source.branch === branch
      ? source
      : null

  if (effective?.providerRef?.kind === 'github') {
    const {
      webhook_registered: webhookRegistered,
      webhook_error: webhookError,
      ...resource
    } = await connectGitHubRepoAsSource(
      effective.providerRef.owner,
      effective.providerRef.repo,
      {
        app_name: name,
        branch,
        build_type: buildType,
        build_path: buildPath,
      },
    )
    return { resource, autoRegistered: webhookRegistered, webhookError }
  }
  if (effective?.providerRef) {
    const resource = await connectRepoAsSource(
      effective.providerRef.connectionId,
      {
        app_name: name,
        repo: effective.providerRef.repo,
        branch,
        build_type: buildType,
        build_path: buildPath,
      },
    )
    return { resource, autoRegistered: true }
  }
  const resource = await setGitSource(name, {
    repo_url: repoUrl,
    branch,
    build_type: buildType,
    build_path: buildPath,
    token: effective?.provider === 'manual' ? effective.token : undefined,
  })
  return { resource, autoRegistered: false }
}
