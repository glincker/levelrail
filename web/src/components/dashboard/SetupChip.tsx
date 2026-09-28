import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRightIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { useSetupChecklistInput } from '../../queries/setupChecklist'
import { computeSetupChecklist } from '../../lib/setupChecklist'
import type { SetupChecklist } from '../../lib/setupChecklist'
import { ITEM_COPY } from '../setupItemCopy'

const DISMISSED_KEY = 'dashboard-setup-checklist-dismissed'

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(DISMISSED_KEY) === '1'
  } catch {
    return false
  }
}

export function SetupChipView({
  checklist,
  onDismiss,
}: {
  checklist: SetupChecklist
  onDismiss: () => void
}) {
  const next = checklist.items.find((i) => i.state === 'todo')
  if (!next) return null
  const copy = ITEM_COPY[next.id]
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-card py-0.5 pr-1 pl-2.5 text-xs">
      <span className="tabular-nums text-muted-foreground">
        Setup {checklist.done}/{checklist.total}
      </span>
      <Link
        to={copy.to}
        className="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-medium text-foreground hover:bg-muted"
      >
        {copy.cta}
        <ArrowRightIcon className="size-3" aria-hidden="true" />
      </Link>
      <button
        type="button"
        aria-label="Dismiss setup progress"
        onClick={onDismiss}
        className="rounded-full p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        <XIcon className="size-3" />
      </button>
    </span>
  )
}

export function SetupChip({
  firstAppName,
}: {
  firstAppName: string | undefined
}) {
  const [dismissed, setDismissed] = useState(readDismissed)
  const input = useSetupChecklistInput(!dismissed, firstAppName)
  const checklist = computeSetupChecklist(input)
  if (dismissed || checklist.loading || checklist.complete) return null
  return (
    <SetupChipView
      checklist={checklist}
      onDismiss={() => {
        try {
          window.localStorage.setItem(DISMISSED_KEY, '1')
        } catch {
          // A blocked store only means the chip reappears next session.
        }
        setDismissed(true)
      }}
    />
  )
}
