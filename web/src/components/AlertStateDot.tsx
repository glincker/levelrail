import type { RuleState } from './alertRulesFormat'

// The solid dot color that pairs with STATE_BADGE_CLASS above, used both
// inside each row's state badge and in the header's firing/pending/ok
// summary counts, so a rule's state reads as a color before the label
// text is even parsed.
const STATE_DOT_CLASS: Record<RuleState, string> = {
  firing: 'bg-red-600 dark:bg-red-400',
  pending: 'bg-amber-600 dark:bg-amber-400',
  ok: 'bg-green-600 dark:bg-green-400',
}

// A firing rule gets a pulsing dot, not just a static one: it is the
// state most likely to need immediate attention, and a still dashboard
// full of quiet badges is exactly the "bolted on" observability feel
// this project set out to avoid, the kind Coolify/Dokploy have.
// Pending/ok stay static so the motion budget on this page is spent
// only where it is meaningful.
export function StateDot({ state }: { state: RuleState }) {
  return (
    <span className="relative inline-flex size-2 shrink-0" aria-hidden="true">
      {state === 'firing' ? (
        <span
          className={`absolute inline-flex size-full animate-ping rounded-full opacity-75 ${STATE_DOT_CLASS[state]}`}
        />
      ) : null}
      <span
        className={`relative inline-flex size-2 rounded-full ${STATE_DOT_CLASS[state]}`}
      />
    </span>
  )
}
