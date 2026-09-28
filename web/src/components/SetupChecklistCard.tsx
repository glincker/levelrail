import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  CircleIcon,
  MinusCircleIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { useSetupChecklistInput } from '../queries/setupChecklist'
import { computeSetupChecklist } from '../lib/setupChecklist'
import { ITEM_COPY } from './setupItemCopy'
import type { SetupChecklist, SetupItemState } from '../lib/setupChecklist'

const DISMISSED_STORAGE_KEY = 'dashboard-setup-checklist-dismissed'

const STATE_LABEL: Record<SetupItemState, string> = {
  done: 'Done',
  todo: 'To do',
  unavailable: 'Unavailable',
}

function StateIcon({ state }: { state: SetupItemState }) {
  const cls = 'size-5 shrink-0'
  if (state === 'done') {
    return <CheckCircleIcon weight="fill" className={`${cls} text-green-600`} />
  }
  if (state === 'unavailable') {
    return <MinusCircleIcon className={`${cls} text-muted-foreground`} />
  }
  return <CircleIcon className={`${cls} text-muted-foreground`} />
}

function readDismissed(): boolean {
  try {
    return window.localStorage.getItem(DISMISSED_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

function writeDismissed(): void {
  try {
    window.localStorage.setItem(DISMISSED_STORAGE_KEY, '1')
  } catch {
    // A blocked store only means the card reappears next session.
  }
}

export function SetupChecklistView({
  checklist,
  onDismiss,
}: {
  checklist: SetupChecklist
  onDismiss: () => void
}) {
  const percent =
    checklist.total === 0
      ? 0
      : Math.round((checklist.done / checklist.total) * 100)
  return (
    <Card size="sm" aria-label="Get set up checklist">
      <CardContent className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-medium text-foreground">Get set up</h2>
            <p className="text-xs text-muted-foreground">
              {checklist.done} of {checklist.total} steps done
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Dismiss setup checklist"
            onClick={onDismiss}
          >
            <XIcon />
          </Button>
        </div>
        <Progress value={percent} aria-label="Setup progress" />
        <ul className="divide-y divide-border">
          {checklist.items.map((item) => {
            const copy = ITEM_COPY[item.id]
            return (
              <li
                key={item.id}
                data-state={item.state}
                className="flex items-center gap-3 py-2"
              >
                <StateIcon state={item.state} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm text-foreground">
                    {copy.title}
                    <span className="sr-only">
                      {' '}
                      ({STATE_LABEL[item.state]})
                    </span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {item.state === 'unavailable'
                      ? 'Not available to your account or this control plane.'
                      : copy.why}
                  </p>
                </div>
                {item.state === 'todo' ? (
                  <Button
                    size="sm"
                    variant="outline"
                    render={<Link to={copy.to} />}
                    nativeButton={false}
                  >
                    {copy.cta}
                  </Button>
                ) : null}
              </li>
            )
          })}
        </ul>
      </CardContent>
    </Card>
  )
}

export function SetupChecklistCard({
  firstAppName,
}: {
  firstAppName: string | undefined
}) {
  const [dismissed, setDismissed] = useState(readDismissed)
  const input = useSetupChecklistInput(!dismissed, firstAppName)
  const checklist = computeSetupChecklist(input)
  if (dismissed || checklist.loading || checklist.complete) return null
  if (checklist.total === 0) return null
  return (
    <SetupChecklistView
      checklist={checklist}
      onDismiss={() => {
        writeDismissed()
        setDismissed(true)
      }}
    />
  )
}
