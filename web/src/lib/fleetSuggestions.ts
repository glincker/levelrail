import type { Tone } from '@/components/kit'
import type { AttentionItem } from './attention'

export type ActionSpec =
  | { kind: 'restart'; app: string }
  | { kind: 'redeploy'; app: string; image: string }
  | { kind: 'logs'; app: string }
  | { kind: 'deploys'; app: string }
  | { kind: 'node'; id: string }
  | { kind: 'domains' }
  | { kind: 'system' }
  | { kind: 'cleanup' }

export interface SuggestionActionSpec {
  label: string
  spec: ActionSpec
  primary: boolean
}

export interface SuggestionSpec {
  id: string
  tone: Tone
  title: string
  detail: string
  actions: SuggestionActionSpec[]
}

function act(
  label: string,
  spec: ActionSpec,
  primary = false,
): SuggestionActionSpec {
  return { label, spec, primary }
}

function actionsFor(item: AttentionItem): SuggestionActionSpec[] {
  const { target } = item
  switch (target.kind) {
    case 'app':
      return [
        act('Restart', { kind: 'restart', app: target.name }, true),
        act('Open logs', { kind: 'logs', app: target.name }),
      ]
    case 'deploy': {
      const rollback = target.lastGoodImage
      return [
        rollback
          ? act(
              'Roll back',
              { kind: 'redeploy', app: target.app, image: rollback },
              true,
            )
          : act(
              'Redeploy',
              { kind: 'redeploy', app: target.app, image: target.image },
              true,
            ),
        act('Open logs', { kind: 'logs', app: target.app }),
      ]
    }
    case 'node':
      return [act('Open node', { kind: 'node', id: target.id }, true)]
    case 'domain':
      return [act('Review certificate', { kind: 'domains' }, true)]
    case 'system':
      return item.id === 'disk'
        ? [act('Clean up Docker', { kind: 'cleanup' }, true)]
        : [act('Fix', { kind: 'system' }, true)]
  }
}

function titleFor(item: AttentionItem): string {
  return item.target.kind === 'app' ? `${item.title} is failing` : item.title
}

export function attentionToSuggestions(
  items: AttentionItem[],
): SuggestionSpec[] {
  return items.map((item) => ({
    id: item.id,
    tone: item.severity === 'critical' ? 'danger' : 'warning',
    title: titleFor(item),
    detail: item.detail,
    actions: actionsFor(item),
  }))
}
