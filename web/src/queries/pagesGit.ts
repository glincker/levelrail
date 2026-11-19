import { editsFromAnalysis, connectSource } from '../lib/newApp'
import { triggerBuild } from './builds'
import { setGitBuild } from './gitBuildSettings'
import { createPagesSite } from './pagesSites'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'
import type { ImportAnalysis } from './importAnalyze'

export interface GitSiteInput {
  name: string
  source: GitRepoSourceValue
  analysis: ImportAnalysis
  buildCommand: string
  outputDir: string
  spa: boolean
  /** True when the site record already exists from an earlier attempt. */
  resume: boolean
}

export type GitSiteStep = 'site' | 'source' | 'build' | 'deploy'

export class GitSiteError extends Error {
  readonly step: GitSiteStep
  constructor(step: GitSiteStep, cause: unknown) {
    super(cause instanceof Error ? cause.message : 'Unknown error')
    this.name = 'GitSiteError'
    this.step = step
  }
}

/** Static site from git in four calls: site record, git source, build settings, first build. Returns the deploy id. */
export async function createGitSite(input: GitSiteInput): Promise<string> {
  const { name, source, analysis } = input
  let step: GitSiteStep = 'site'
  try {
    if (!input.resume) {
      const route = await createPagesSite(name)
      if (route === 'fallback') {
        throw new Error('this control plane cannot create static sites')
      }
    }
    step = 'source'
    const edits = {
      ...editsFromAnalysis(analysis),
      name,
      buildType: 'static' as const,
      spa: input.spa,
    }
    await connectSource(name, source, edits)
    step = 'build'
    await setGitBuild(name, {
      build_type: 'static',
      build_path: '',
      base_directory: edits.baseDirectory,
      build_command: input.buildCommand,
      publish_dir: input.outputDir,
      ...(input.spa ? { static_spa: true } : {}),
    })
    step = 'deploy'
    const built = await triggerBuild(name, {
      repoUrl: source.repoUrl,
      ref: source.branch || analysis.branch,
      buildType: 'static',
      baseDirectory: edits.baseDirectory || undefined,
      detectedFramework: analysis.stack.framework,
    })
    if (!built.id) throw new Error('the build did not return a deploy id')
    return built.id
  } catch (err) {
    throw new GitSiteError(step, err)
  }
}
