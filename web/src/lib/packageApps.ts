import { analyzeImport, type WorkspacePackage } from '../queries/importAnalyze'
import { setGitDeploySettings } from '../queries/gitDeploySettings'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'
import { applyNewApp, editsFromAnalysis, type ApplyResult } from './newApp'

export type PackageAppStatus = 'idle' | 'creating' | 'done' | 'failed'

export interface PackageAppState {
  status: PackageAppStatus
  appName?: string
  deployId?: string
  message?: string
}

/**
 * Creates one app for a workspace package: analyzes it from its own
 * directory, applies the same flow as a single import, then limits pushes
 * to the package's watch paths so a sibling's change does not redeploy it.
 */
export async function createPackageApp(
  source: GitRepoSourceValue,
  pkg: WorkspacePackage,
): Promise<ApplyResult> {
  const analysis = await analyzeImport({
    repo_url: source.repoUrl,
    ref: source.branch || undefined,
    token: source.token,
    base_directory: pkg.dir,
    dockerfile_path: pkg.dockerfile || undefined,
    build_type: pkg.dockerfile ? 'dockerfile' : undefined,
  })
  const edits = editsFromAnalysis(analysis)
  const result = await applyNewApp(analysis, source, edits)
  if (pkg.watch_paths.length > 0) {
    await setGitDeploySettings(result.appName, {
      deploy_paths: pkg.watch_paths,
    })
  }
  return result
}
