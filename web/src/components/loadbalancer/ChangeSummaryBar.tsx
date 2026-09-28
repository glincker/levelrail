import { useEffect, useState } from 'react'
import { ArrowRightIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Kbd } from '@/components/kit'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ChangeChip } from './changes'

interface Props {
  chips: ChangeChip[]
  effect: string
  blocker?: string
  saving: boolean
  creating: boolean
  onSave: () => void
  onRevert: () => void
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  )
}

export function ChangeSummaryBar({
  chips,
  effect,
  blocker,
  saving,
  creating,
  onSave,
  onRevert,
}: Props) {
  const [confirm, setConfirm] = useState(false)
  const risks = chips.filter((c) => c.risky)
  const canSave = !blocker && !saving && chips.length > 0

  function requestSave() {
    if (!canSave) return
    if (risks.length > 0) setConfirm(true)
    else onSave()
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== 's' || e.metaKey || e.ctrlKey || e.altKey)
        return
      if (isTyping(e.target)) return
      e.preventDefault()
      requestSave()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  return (
    <>
      <div
        role="region"
        aria-label="Unsaved changes"
        className="sticky bottom-3 z-20 space-y-2 rounded-2xl border bg-popover p-3 shadow-floating kit-enter"
      >
        <ul className="flex flex-wrap gap-1.5">
          {chips.map((c) => (
            <li
              key={c.key}
              data-chip={c.key}
              className="inline-flex items-center gap-1.5 rounded-full border bg-muted/50 px-2.5 py-1 text-xs"
            >
              {c.risky ? (
                <WarningIcon
                  className="size-3.5 text-tone-warning"
                  aria-label="Risky change"
                />
              ) : null}
              <span className="text-muted-foreground">{c.label}</span>
              <span className="tabular-nums line-through decoration-muted-foreground/60">
                {c.before}
              </span>
              <ArrowRightIcon className="size-3" aria-hidden />
              <span className="font-medium tabular-nums">{c.after}</span>
            </li>
          ))}
        </ul>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">
            {blocker ?? effect}
          </p>
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={onRevert}>
              {creating ? 'Cancel' : 'Revert'}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!canSave}
              onClick={requestSave}
            >
              {saving ? 'Saving' : creating ? 'Create' : 'Save'}
              <Kbd keys={['S']} />
            </Button>
          </div>
        </div>
      </div>
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Save risky changes?</DialogTitle>
            <DialogDescription>
              These take effect on live traffic right away.
            </DialogDescription>
          </DialogHeader>
          <ul className="space-y-2 text-sm">
            {risks.map((c) => (
              <li key={c.key} className="flex gap-2">
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0 text-tone-warning"
                  aria-hidden
                />
                <span>
                  <span className="font-medium">{c.label}:</span> {c.risky}
                </span>
              </li>
            ))}
          </ul>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setConfirm(false)}
            >
              Keep editing
            </Button>
            <Button
              type="button"
              onClick={() => {
                setConfirm(false)
                onSave()
              }}
            >
              Save anyway
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
