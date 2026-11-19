import { isBrandIconName, type BrandIconName } from '../../BrandIcon'
import { logoIdForFramework } from '../../../lib/brandLogos'
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

/** The wire has step counts but no step names, so cloning means nothing has finished yet. */
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
