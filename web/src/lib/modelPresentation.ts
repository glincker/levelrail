import type { Tone } from '../components/kit/tone'
import type { ModelResource } from '../types/models'
import { modelPhase } from './models'

export function modelTone(model: ModelResource): Tone {
  switch (modelPhase(model)) {
    case 'ready':
      return 'success'
    case 'progress':
      return 'warning'
    case 'deleting':
      return 'neutral'
    default:
      return 'danger'
  }
}

export function modelStatusLabel(model: ModelResource): string {
  return modelPhase(model) === 'ready' ? 'Loaded' : model.status.reason
}

export function chatCurlExample(baseUrl: string, modelRef: string): string {
  const base = baseUrl.replace(/\/+$/, '')
  const body = JSON.stringify({
    model: modelRef,
    messages: [{ role: 'user', content: 'Hello' }],
  })
  return [
    `curl ${base}/chat/completions`,
    '  -H "Authorization: Bearer $API_KEY"',
    '  -H "Content-Type: application/json"',
    `  -d '${body}'`,
  ].join(' \\\n')
}

export const MODEL_TABS = ['overview', 'keys', 'usage', 'logs'] as const
export type ModelTab = (typeof MODEL_TABS)[number]

export function parseModelTab(value: unknown): ModelTab {
  return MODEL_TABS.find((t) => t === value) ?? 'overview'
}
