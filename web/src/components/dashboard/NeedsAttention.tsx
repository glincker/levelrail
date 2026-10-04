import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BroomIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonList, SuggestionList } from '@/components/kit'
import { useAttentionItems } from '../../queries/attention'
import { attentionToSuggestions } from '../../lib/fleetSuggestions'
import type { SuggestionSpec } from '../../lib/fleetSuggestions'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { isFeatureVisible } from '../../lib/experimental'
import { useSuggestionRunner } from './useSuggestionRunner'

const MAX_SHOWN = 4

export function NeedsAttentionView({
  specs,
  loading,
  onRun,
}: {
  specs: SuggestionSpec[]
  loading: boolean
  onRun: (spec: SuggestionSpec['actions'][number]['spec']) => void
}) {
  const { t } = useTranslation('dashboard')
  const [pending, setPending] = useState<string | null>(null)
  if (loading) return <SkeletonList rows={2} />
  if (specs.length === 0) return null
  const items = specs.slice(0, MAX_SHOWN).map((s) => ({
    id: s.id,
    tone: s.tone,
    icon:
      s.tone === 'danger' ? (
        <WarningCircleIcon className="size-5" weight="fill" />
      ) : s.id === 'disk' ? (
        <BroomIcon className="size-5" />
      ) : (
        <WarningIcon className="size-5" weight="fill" />
      ),
    title: s.title,
    detail: s.detail,
    actions: s.actions.map((a) => ({
      label: a.label,
      kind: a.primary ? ('primary' as const) : ('secondary' as const),
      pending: pending === `${s.id}:${a.label}`,
      onClick: () => {
        const key = `${s.id}:${a.label}`
        setPending(key)
        onRun(a.spec)
        window.setTimeout(() => {
          setPending((cur) => (cur === key ? null : cur))
        }, 1500)
      },
    })),
  }))
  return (
    <section aria-label={t('needsAttention.title')} className="space-y-3">
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-foreground">
          {t('needsAttention.title')}
        </h2>
        {specs.length > MAX_SHOWN ? (
          <Link
            to="/status"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            {t('needsAttention.seeAll', { count: specs.length })}
          </Link>
        ) : null}
      </div>
      <SuggestionList items={items} />
    </section>
  )
}

export function NeedsAttention() {
  const { t } = useTranslation('dashboard')
  const { items, isLoading } = useAttentionItems()
  const run = useSuggestionRunner()
  const features = useExperimentalFeatures()
  const aiChatEnabled = isFeatureVisible('ai-chat', features)
  const specs = attentionToSuggestions(items, t).map((spec) =>
    aiChatEnabled
      ? spec
      : {
          ...spec,
          actions: spec.actions.filter((a) => a.spec.kind !== 'ask-ai'),
        },
  )
  return <NeedsAttentionView specs={specs} loading={isLoading} onRun={run} />
}
