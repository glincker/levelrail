import { isBrandIconName, type BrandIconName } from '../../BrandIcon'
import { logoIdForFramework } from '../../../lib/brandLogos'
import type { DeploymentStep } from '../../../types/deployment'
import type { ImportAnalysis } from '../../../queries/importAnalyze'

export interface StaticDetection {
  /** Real framework label from the analysis, empty for plain HTML. */
  framework: string
  logo: BrandIconName | null
  buildCommand: string
  outputDir: string
  spa: boolean
  /** False when the analysis says the repo needs a running server. */
  isStatic: boolean
}

const EXTRA_LOGOS: Array<[RegExp, string]> = [
  [/hugo/, 'hugo'],
  [/jekyll/, 'jekyll'],
  [/eleventy|11ty/, 'eleventy'],
]

function logoFor(label: string): BrandIconName | null {
  const lower = label.toLowerCase()
  const id =
    EXTRA_LOGOS.find(([re]) => re.test(lower))?.[1] ?? logoIdForFramework(label)
  return id && isBrandIconName(id) ? id : null
}

/** Reads the repo analysis into the few fields a static site needs. */
export function staticDetection(a: ImportAnalysis): StaticDetection {
  const framework = a.stack.framework || a.detection?.framework || ''
  return {
    framework,
    logo: framework ? logoFor(framework) : null,
    buildCommand: a.detection?.build_command ?? '',
    outputDir: a.detection?.output_dir ?? '',
    spa: a.detection?.spa === true,
    isStatic: a.build.type === 'static',
  }
}

export type GitStage = 'cloning' | 'building' | 'ready' | 'failed'

export const GIT_STEP_NAMES = [
  'cloning',
  'building',
  'publishing',
  'live',
] as const
export type GitStepName = (typeof GIT_STEP_NAMES)[number]
export type GitStepState = DeploymentStep['state']
export interface GitStep {
  name: GitStepName
  state: GitStepState
}

const KNOWN_STATES: readonly string[] = [
  'pending',
  'running',
  'done',
  'failed',
  'unknown',
]

/** Always four rows in order; a step the server did not name stays pending, and an unrecognised state is neutral. */
export function gitSteps(
  items: readonly { name: string; state: string }[] | undefined,
  status: string | null,
): GitStep[] {
  return GIT_STEP_NAMES.map((name) => {
    const hit = items?.find((i) => i.name === name)
    if (!hit) {
      return { name, state: status === 'ready' ? 'done' : 'pending' }
    }
    const state = KNOWN_STATES.includes(hit.state)
      ? (hit.state as GitStepState)
      : 'unknown'
    return { name, state }
  })
}

/** Coarse stage from the deployment status, used for success and failure; the step rows carry the detail. */
export function stageFromDeployment(
  status: string | null,
  doneSteps: number,
): GitStage {
  if (status === 'ready') return 'ready'
  if (
    status === 'failed' ||
    status === 'refused' ||
    status === 'canceled' ||
    status === 'rolled_back'
  ) {
    return 'failed'
  }
  return status === 'building' && doneSteps > 0 ? 'building' : 'cloning'
}

/** Site name suggestion from a repository name, kept to the allowed pattern. */
export function siteNameFromRepo(repoName: string): string {
  return repoName
    .toLowerCase()
    .replace(/\.git$/, '')
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
}
