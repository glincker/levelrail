import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { useApplyFinding } from '../../queries/resourceAdvisor'
import { sleepableFindings } from '../../lib/advisorSavings'
import type { AdvisorFinding } from '../../types/resourceAdvisor'

/** Enables sleep-when-idle for every idle app finding, after one confirm. */
export function SleepIdleButton({ findings }: { findings: AdvisorFinding[] }) {
  const { t } = useTranslation('resourceAdvisor')
  const [open, setOpen] = useState(false)
  const [running, setRunning] = useState(false)
  const [failed, setFailed] = useState<string[]>([])
  const apply = useApplyFinding()
  const idle = sleepableFindings(findings)
  if (idle.length === 0) return null

  async function run() {
    setRunning(true)
    const bad: string[] = []
    for (const f of idle) {
      try {
        await apply.mutateAsync(f.id)
      } catch {
        bad.push(f.resource.name)
      }
    }
    setRunning(false)
    setFailed(bad)
    if (bad.length === 0) {
      toast.add({
        type: 'success',
        title: t('sleepAll.done', { count: idle.length }),
      })
      setOpen(false)
    }
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          setFailed([])
          setOpen(true)
        }}
      >
        {t('sleepAll.button', { count: idle.length })}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('sleepAll.title')}</DialogTitle>
            <DialogDescription>
              {t('sleepAll.description', { count: idle.length })}
            </DialogDescription>
          </DialogHeader>
          <ul className="max-h-48 space-y-1 overflow-auto font-mono text-xs">
            {idle.map((f) => (
              <li key={f.id} className="rounded bg-muted px-2 py-1">
                {f.resource.name}
              </li>
            ))}
          </ul>
          {failed.length > 0 ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('sleepAll.failed', { names: failed.join(', ') })}
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setOpen(false)
              }}
            >
              {t('apply.cancel')}
            </Button>
            <Button
              disabled={running}
              onClick={() => {
                void run()
              }}
            >
              {running ? t('apply.pending') : t('sleepAll.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
