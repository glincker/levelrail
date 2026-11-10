import { setGitSource } from '../queries/gitSources'
import { connectGitHubRepoAsSource } from '../queries/githubApp'
import { connectRepoAsSource } from '../queries/gitConnections'
import type { GitSourceResource } from '../types/gitSource'
import {
  additionalServicesPayload,
  servicesPayload,
  type FormState,
} from './gitSourceForm'

export interface ConnectGitSourceResult {
  resource: GitSourceResource
  autoRegistered: boolean
  webhookError?: string
}

// A fresh provider pick uses its use-as-source endpoint (auto webhook); fan-out
// rows force the generic PUT, which is the only endpoint that accepts them.
export async function connectGitSource(
  appName: string,
  form: FormState,
): Promise<ConnectGitSourceResult> {
  const branch = form.branch.trim() || undefined
  const buildPath = form.buildPath.trim() || undefined
  const baseDirectory = form.baseDirectory.trim() || undefined
  const additionalServices =
    form.fanoutMode === 'additional'
      ? additionalServicesPayload(form.additionalServices)
      : undefined
  const services =
    form.fanoutMode === 'services' ? servicesPayload(form.services) : undefined

  if (form.providerRef && !additionalServices && !services) {
    if (form.providerRef.kind === 'github') {
      const {
        webhook_registered: autoRegistered,
        webhook_error: webhookError,
        ...resource
      } = await connectGitHubRepoAsSource(
        form.providerRef.owner,
        form.providerRef.repo,
        {
          app_name: appName,
          branch,
          build_type: form.buildType,
          build_path: buildPath,
          base_directory: baseDirectory,
          trigger_mode: form.triggerMode,
        },
      )
      return { resource, autoRegistered, webhookError }
    }
    const resource = await connectRepoAsSource(form.providerRef.connectionId, {
      app_name: appName,
      repo: form.providerRef.repo,
      branch,
      build_type: form.buildType,
      build_path: buildPath,
      base_directory: baseDirectory,
      trigger_mode: form.triggerMode,
    })
    return { resource, autoRegistered: true }
  }

  const resource = await setGitSource(appName, {
    repo_url: form.repoUrl.trim(),
    branch,
    build_type: form.buildType,
    build_path: buildPath,
    base_directory: baseDirectory,
    token: form.token.trim() || undefined,
    additional_services: additionalServices,
    services,
    trigger_mode: form.triggerMode,
  })
  return { resource, autoRegistered: false }
}
