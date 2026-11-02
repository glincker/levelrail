import type { GitRepoSourceValue } from '@/components/GitRepoSourcePicker'
import type {
  GitSourceBuild,
  GitSourceBuildType,
  GitSourceResource,
  GitSourceService,
  GitSourceTriggerMode,
} from '../types/gitSource'

export const BUILD_PACKS: { value: GitSourceBuildType; label: string }[] = [
  { value: 'railpack', label: 'Auto-detect (Railpack)' },
  { value: 'dockerfile', label: 'Dockerfile' },
  { value: 'static', label: 'Static site' },
]

export const TRIGGER_MODES: {
  value: GitSourceTriggerMode
  label: string
  description: string
}[] = [
  {
    value: 'push',
    label: 'Every push',
    description: 'Deploy on every push to the target branch.',
  },
  {
    value: 'release',
    label: 'Tags & releases',
    description:
      'Deploy only on a tag push, or (GitHub only) a published release. Branch pushes never deploy.',
  },
]

// Rows are an array, not a Record, so a name can be edited mid-typing without key collisions.
export interface AdditionalServiceRow {
  serviceName: string
  buildType: GitSourceBuildType
  buildPath: string
}

// Like AdditionalServiceRow plus a port, since a services: entry fully describes a service.
export interface ServiceSpecRow {
  serviceName: string
  buildType: GitSourceBuildType
  buildPath: string
  port: string
}

// The two fan-out mechanisms are mutually exclusive server-side, so the form submits one.
export type FanoutMode = 'additional' | 'services'

export interface FormState {
  repoUrl: string
  branch: string
  buildType: GitSourceBuildType
  buildPath: string
  baseDirectory: string
  triggerMode: GitSourceTriggerMode
  token: string
  // Set only by a fresh GitRepoSourcePicker pick from a connected
  // provider row, cleared on every startEdit/cancelEdit: see
  // connectGitSource's own doc comment for how this decides which
  // endpoint submit() calls.
  providerRef?: GitRepoSourceValue['providerRef']
  fanoutMode: FanoutMode
  additionalServices: AdditionalServiceRow[]
  services: ServiceSpecRow[]
}

// Auto-detect is the recommended default: most repos need no build config.
export function emptyForm(): FormState {
  return {
    repoUrl: '',
    branch: '',
    buildType: 'railpack',
    buildPath: '',
    baseDirectory: '',
    triggerMode: 'push',
    token: '',
    providerRef: undefined,
    fanoutMode: 'additional',
    additionalServices: [],
    services: [],
  }
}

export function formFromResource(g: GitSourceResource): FormState {
  const hasServices = Object.keys(g.services ?? {}).length > 0
  return {
    repoUrl: g.repo_url,
    branch: g.branch,
    buildType: g.build_type,
    buildPath: g.build_path ?? '',
    baseDirectory: g.base_directory ?? '',
    triggerMode: g.trigger_mode,
    token: '',
    providerRef: undefined,
    fanoutMode: hasServices ? 'services' : 'additional',
    additionalServices: Object.entries(g.additional_services ?? {}).map(
      ([serviceName, build]) => ({
        serviceName,
        buildType: build.build_type,
        buildPath: build.build_path ?? '',
      }),
    ),
    services: Object.entries(g.services ?? {}).map(([serviceName, svc]) => ({
      serviceName,
      buildType: isGitSourceBuildType(svc.build.type)
        ? svc.build.type
        : 'dockerfile',
      buildPath: svc.build.path ?? '',
      port: svc.port ? String(svc.port) : '',
    })),
  }
}

// Narrows a services: entry's build.type (also image/compose) to the three offered here.
function isGitSourceBuildType(value: string): value is GitSourceBuildType {
  return value === 'dockerfile' || value === 'railpack' || value === 'static'
}

// A row is incomplete only once touched (path or type changed) while its name is empty.
export function incompleteRowIndexes(rows: AdditionalServiceRow[]): number[] {
  return rows
    .map((row, index) => ({ row, index }))
    .filter(
      ({ row }) =>
        !row.serviceName.trim() &&
        (row.buildPath.trim() !== '' || row.buildType !== 'dockerfile'),
    )
    .map(({ index }) => index)
}

export function additionalServicesPayload(
  rows: AdditionalServiceRow[],
): Record<string, GitSourceBuild> | undefined {
  const entries = rows
    .filter((row) => row.serviceName.trim())
    .map((row): [string, GitSourceBuild] => [
      row.serviceName.trim(),
      {
        build_type: row.buildType,
        build_path: row.buildPath.trim() || undefined,
      },
    ])
  return entries.length > 0 ? Object.fromEntries(entries) : undefined
}

// A filled-in port becomes a number; an empty one is omitted rather than sent as 0.
export function servicesPayload(
  rows: ServiceSpecRow[],
): Record<string, GitSourceService> | undefined {
  const entries = rows
    .filter((row) => row.serviceName.trim())
    .map((row): [string, GitSourceService] => [
      row.serviceName.trim(),
      {
        build: { type: row.buildType, path: row.buildPath.trim() || undefined },
        port: row.port.trim() ? Number(row.port.trim()) : undefined,
      },
    ])
  return entries.length > 0 ? Object.fromEntries(entries) : undefined
}

// Duplicate names would collapse into one YAML key, so they are caught before validating.
export function duplicateServiceNames(rows: ServiceSpecRow[]): Set<string> {
  const counts = new Map<string, number>()
  for (const row of rows) {
    const name = row.serviceName.trim()
    if (!name) continue
    counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  return new Set([...counts].filter(([, n]) => n > 1).map(([name]) => name))
}

// Renders the rows as a minimal app.yaml for live spec validation; empty when
// nothing is named or names collide (reported locally instead).
export function servicesSpecYaml(rows: ServiceSpecRow[]): string {
  const named = rows.filter((row) => row.serviceName.trim())
  if (named.length === 0 || duplicateServiceNames(rows).size > 0) return ''
  const lines = ['version: 1', 'services:']
  for (const row of named) {
    lines.push(`  ${JSON.stringify(row.serviceName.trim())}:`)
    lines.push('    build:')
    lines.push(`      type: ${JSON.stringify(row.buildType)}`)
    if (row.buildPath.trim()) {
      lines.push(`      path: ${JSON.stringify(row.buildPath.trim())}`)
    }
    if (row.port.trim()) {
      lines.push(`    port: ${Number(row.port.trim()) || 0}`)
    }
  }
  return lines.join('\n')
}

// Builds the "Row 1, 2 are missing a service name." message shown when
// rows were started but never named.
export function incompleteRowsMessage(incomplete: number[]): string {
  const rowLabel = incomplete.length === 1 ? 'Row' : 'Rows'
  const rowNumbers = incomplete.map((index) => index + 1).join(', ')
  return `${rowLabel} ${rowNumbers} ${incomplete.length === 1 ? 'is' : 'are'} missing a service name.`
}

export function isBuildTypeValue(v: unknown): v is GitSourceBuildType {
  return v === 'railpack' || v === 'dockerfile' || v === 'static'
}
