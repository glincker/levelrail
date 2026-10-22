import type { TFunction } from 'i18next'
import type { Tone } from '@/components/kit'
import type { AttentionItem } from './attention'

type DashboardT = TFunction<'dashboard'>

export type ActionSpec =
  | { kind: 'restart'; app: string }
  | { kind: 'redeploy'; app: string; image: string }
  | { kind: 'logs'; app: string }
  | { kind: 'deploys'; app: string }
  | { kind: 'node'; id: string }
  | { kind: 'domains' }
  | { kind: 'route'; to: '/settings/cli-access' | '/approvals' }
  | { kind: 'path'; to: string }
  | { kind: 'copy-retry'; database: string }
  | { kind: 'system' }
  | { kind: 'cleanup' }
  | { kind: 'ask-ai'; message: string }

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
  // Items sharing a key are one root cause and render as one grouped row.
  groupKey?: string
  actions: SuggestionActionSpec[]
}

function act(
  label: string,
  spec: ActionSpec,
  primary = false,
): SuggestionActionSpec {
  return { label, spec, primary }
}

function targetLabel(item: AttentionItem): string | null {
  switch (item.target.kind) {
    case 'app':
      return item.target.name
    case 'deploy':
      return item.target.app
    case 'node':
      return item.target.id
    default:
      return null
  }
}

// Gives the assistant real specifics up front instead of making the
// user re-describe the problem from scratch.
export function seedMessageFor(item: AttentionItem): string {
  const target = targetLabel(item)
  const base = `Help me fix this: ${titleFor(item)}. ${item.detail}.`
  return target ? `${base} Target: ${target}.` : base
}

function actionsFor(
  item: AttentionItem,
  t: DashboardT,
): SuggestionActionSpec[] {
  const { target } = item
  const base: SuggestionActionSpec[] = (() => {
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
      case 'route':
        return [
          act(
            t('needsAttention.review'),
            { kind: 'route', to: target.to },
            true,
          ),
        ]
      case 'path':
        if (item.copy?.database) {
          return [
            act(
              t('needsAttention.retryCopy'),
              { kind: 'copy-retry', database: item.copy.database },
              true,
            ),
            act(t('needsAttention.openDatabase'), {
              kind: 'path',
              to: target.to,
            }),
          ]
        }
        return [
          act(
            item.action ?? t('needsAttention.review'),
            { kind: 'path', to: target.to },
            true,
          ),
        ]
      case 'system':
        return item.id === 'disk'
          ? [act('Clean up Docker', { kind: 'cleanup' }, true)]
          : [act('Fix', { kind: 'system' }, true)]
    }
  })()
  return [
    ...base,
    act(t('needsAttention.askAi'), {
      kind: 'ask-ai',
      message: seedMessageFor(item),
    }),
  ]
}

function titleFor(item: AttentionItem): string {
  return item.target.kind === 'app' ? `${item.title} is failing` : item.title
}

export function attentionToSuggestions(
  items: AttentionItem[],
  t: DashboardT,
): SuggestionSpec[] {
  const actionable = items.filter((i) => i.severity !== 'info')
  return actionable.map((item) => ({
    id: item.id,
    tone: item.severity === 'critical' ? 'danger' : 'warning',
    title: titleFor(item),
    detail: item.detail,
    groupKey: item.copy?.code ? `copy:${item.copy.code}` : undefined,
    actions: actionsFor(item, t),
  }))
}
