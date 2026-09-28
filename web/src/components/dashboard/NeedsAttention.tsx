import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  BroomIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonList, SuggestionList } from '@/components/kit'
import { useAttentionItems } from '../../queries/attention'
import { attentionToSuggestions } from '../../lib/fleetSuggestions'
import type { SuggestionSpec } from '../../lib/fleetSuggestions'
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
    <section aria-label="Needs attention" className="space-y-3">
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-foreground">
          Needs attention
        </h2>
        {specs.length > MAX_SHOWN ? (
          <Link
            to="/status"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            See all {specs.length}
          </Link>
        ) : null}
      </div>
      <SuggestionList items={items} />
    </section>
  )
}

export function NeedsAttention() {
  const { items, isLoading } = useAttentionItems()
  const run = useSuggestionRunner()
  return (
    <NeedsAttentionView
      specs={attentionToSuggestions(items)}
      loading={isLoading}
      onRun={run}
    />
  )
}
