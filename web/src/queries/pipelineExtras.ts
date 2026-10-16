// Artifacts and inbound webhook triggers of a pipeline
// (internal/api/pipelines_artifacts.go and pipelines_webhook.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  PipelineArtifact,
  PipelineRun,
  PipelineWebhook,
  PipelineWebhookDelivery,
  PipelineWebhookRequest,
} from '../types/pipelines'
import { jsonInit, pipelineKeys, requestJson } from './pipelines'

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

export const pipelineExtraKeys = {
  artifacts: (app: string, runId: string) =>
    [...pipelineKeys.all(app), 'artifacts', runId] as const,
  webhook: (app: string, name: string) =>
    [...pipelineKeys.all(app), 'webhook', name] as const,
  deliveries: (app: string, name: string) =>
    [...pipelineKeys.all(app), 'webhook-deliveries', name] as const,
}

export function artifactDownloadUrl(
  app: string,
  runId: string,
  artifactId: string,
): string {
  return `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/artifacts/${encodeURIComponent(artifactId)}/download`
}

export function usePipelineArtifacts(app: string, runId: string) {
  return useQuery({
    queryKey: pipelineExtraKeys.artifacts(app, runId),
    queryFn: () =>
      requestJson<PipelineArtifact[]>(
        `${base(app)}/pipeline-runs/${encodeURIComponent(runId)}/artifacts`,
        'fetch pipeline artifacts',
      ),
  })
}

function webhookPath(app: string, name: string): string {
  return `${base(app)}/pipelines/${encodeURIComponent(name)}/webhook`
}

export function usePipelineWebhook(app: string, name: string, enabled = true) {
  return useQuery({
    enabled,
    queryKey: pipelineExtraKeys.webhook(app, name),
    queryFn: () =>
      requestJson<PipelineWebhook>(
        webhookPath(app, name),
        'fetch pipeline webhook',
      ),
  })
}

export function usePipelineWebhookDeliveries(
  app: string,
  name: string,
  enabled = true,
) {
  return useQuery({
    enabled,
    queryKey: pipelineExtraKeys.deliveries(app, name),
    queryFn: () =>
      requestJson<PipelineWebhookDelivery[]>(
        `${webhookPath(app, name)}/deliveries`,
        'fetch webhook deliveries',
      ),
  })
}

export function useSetPipelineWebhook(app: string, name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: PipelineWebhookRequest) =>
      requestJson<PipelineWebhook>(
        webhookPath(app, name),
        'update pipeline webhook',
        jsonInit('PUT', req),
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: pipelineExtraKeys.webhook(app, name) }),
  })
}

export function useRotatePipelineWebhook(app: string, name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      requestJson<PipelineWebhook>(
        `${webhookPath(app, name)}/rotate`,
        'rotate pipeline webhook',
        jsonInit('POST'),
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: pipelineExtraKeys.webhook(app, name) }),
  })
}

export function useDeletePipelineWebhook(app: string, name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      requestJson<void>(
        webhookPath(app, name),
        'delete pipeline webhook',
        jsonInit('DELETE'),
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: pipelineExtraKeys.webhook(app, name) }),
  })
}

export function useReplayWebhookDelivery(app: string, name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deliveryId: string) =>
      requestJson<PipelineRun>(
        `${webhookPath(app, name)}/deliveries/${encodeURIComponent(deliveryId)}/replay`,
        'replay webhook delivery',
        jsonInit('POST'),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({
        queryKey: pipelineExtraKeys.deliveries(app, name),
      })
      return qc.invalidateQueries({ queryKey: pipelineKeys.runs(app) })
    },
  })
}
