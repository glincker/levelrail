import { connectRepoAsSource } from '../../queries/gitConnections'
import { connectGitHubRepoAsSource } from '../../queries/githubApp'
import { setGitSource } from '../../queries/gitSources'
import type { GitSourceResource } from '../../types/gitSource'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'

// A fresh pick connects with auto-detected builds: a provider pick uses that
// provider's use-as-source endpoint (registers the push webhook), a pasted
// URL uses the generic PUT, same split GitSourceCard makes.
export async function connectPickedRepo(
  appName: string,
  value: GitRepoSourceValue,
): Promise<GitSourceResource> {
  const req = {
    app_name: appName,
    branch: value.branch.trim() || undefined,
    build_type: 'railpack' as const,
  }
  const ref = value.providerRef
  if (ref?.kind === 'github') {
    const {
      webhook_registered: _r,
      webhook_error: _e,
      ...resource
    } = await connectGitHubRepoAsSource(ref.owner, ref.repo, req)
    void _r
    void _e
    return resource
  }
  if (ref) {
    return connectRepoAsSource(ref.connectionId, { ...req, repo: ref.repo })
  }
  return setGitSource(appName, {
    repo_url: value.repoUrl.trim(),
    branch: value.branch.trim() || undefined,
    build_type: 'railpack',
    token: value.token?.trim() || undefined,
  })
}
