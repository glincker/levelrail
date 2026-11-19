import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DeploymentSteps } from '../types/deployment'

export interface GitSiteInput {
  name: string
  repoUrl: string
  branch: string
  /** Provider connection id the repository was picked from. */
  accountId?: string
  buildCommand: string
  outputDir: string
  deployOnPush: boolean
}

export interface GitSiteResult {
  /** Omitted by the server when the clone is slow: follow latest_deploy then. */
  deployId?: string
  steps: DeploymentSteps | null
  status: string | null
}

interface CreateResponse {
  deploy_id?: string
  latest_deploy?: { id?: string; status?: string; steps?: DeploymentSteps }
}

/** One call creates the site, connects the repo and starts the first deploy. The server removes the site it made when the call fails, so a retry starts clean. */
export async function createGitSite(
  input: GitSiteInput,
): Promise<GitSiteResult> {
  const res = await fetch('/api/v1/static-sites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: input.name,
      git: {
        repo_url: input.repoUrl,
        ...(input.branch ? { branch: input.branch } : {}),
        ...(input.accountId ? { account_id: input.accountId } : {}),
        build_command: input.buildCommand,
        output_dir: input.outputDir,
        deploy_on_push: input.deployOnPush,
      },
      deploy: true,
    }),
  })
  if (res.status === 404 || res.status === 405) {
    throw new ApiError(
      res.status,
      'this control plane cannot create static sites from git',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create site failed: ${res.status}`),
    )
  }
  const body = (await res.json().catch(() => ({}))) as CreateResponse
  return {
    deployId: body.deploy_id ?? body.latest_deploy?.id,
    steps: body.latest_deploy?.steps ?? null,
    status: body.latest_deploy?.status ?? null,
  }
}
