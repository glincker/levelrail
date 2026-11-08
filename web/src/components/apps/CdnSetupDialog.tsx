import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { CommandBlock } from '@/components/ui/code-block'
import { StatusBadge } from '@/components/ui/status-badge'
import { toast } from '@/components/ui/toast'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  CircleDashedGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import type { Tone } from '@/components/kit/tone'
import { useCdnSetup, type CdnStepStatus } from '../../queries/appEdge'

const STEP_VIEW: Record<CdnStepStatus, { tone: Tone; icon: UiIcon }> = {
  ok: { tone: 'success', icon: CircleCheckGlassIcon },
  warn: { tone: 'warning', icon: CircleAlertGlassIcon },
  fail: { tone: 'danger', icon: CircleXGlassIcon },
  skipped: { tone: 'neutral', icon: CircleDashedGlassIcon },
}

/** CdnSetupDialog runs a dry run first, lists the steps with copyable fixes, then applies. */
export function CdnSetupDialog({
  appName,
  open,
  onOpenChange,
}: Readonly<{
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('cdn')
  const setup = useCdnSetup(appName)
  const result = setup.data
  const dryRunDone = result !== undefined && result.dry_run
  const canApply = dryRunDone && result.status !== 'fail'
  const pendingDry = setup.isPending && setup.variables?.dry_run === true
  const pendingApply = setup.isPending && setup.variables?.dry_run === false

  function run(dryRun: boolean) {
    setup.mutate(
      { dry_run: dryRun },
      {
        onSuccess: (r) => {
          if (r.applied) {
            toast.add({ title: t('setup.applied'), type: 'success' })
          }
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('setup.title')}</DialogTitle>
          <DialogDescription>{t('setup.description')}</DialogDescription>
        </DialogHeader>
        {setup.error ? (
          <p role="alert" className="text-sm text-tone-danger">
            {t('setup.error', { message: setup.error.message })}
          </p>
        ) : null}
        {result ? (
          <div className="space-y-3">
            <p className="text-sm font-medium text-foreground" role="status">
              {t(`setup.result.${result.status}`)}
              <span className="ml-2 font-normal text-muted-foreground">
                {result.applied ? t('setup.applied') : t('setup.dryRunDone')}
              </span>
            </p>
            <ol className="divide-y divide-border">
              {result.steps.map((step) => {
                const view = STEP_VIEW[step.status]
                return (
                  <li key={step.id} className="space-y-1.5 py-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <StatusBadge
                        tone={view.tone}
                        icon={view.icon}
                        label={t(`setup.status.${step.status}`)}
                      />
                      <span className="text-sm font-medium text-foreground">
                        {t(`setup.step.${step.id}`, { defaultValue: step.id })}
                      </span>
                    </div>
                    <p className="text-sm text-muted-foreground">
                      {step.message}
                    </p>
                    {step.fix ? (
                      <CommandBlock code={step.fix} label={t('setup.fix')} />
                    ) : null}
                  </li>
                )
              })}
            </ol>
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('setup.close')}
          </Button>
          <Button
            type="button"
            variant={canApply ? 'outline' : 'default'}
            disabled={setup.isPending}
            onClick={() => run(true)}
          >
            {pendingDry ? t('setup.running') : t('setup.dryRun')}
          </Button>
          {canApply ? (
            <Button
              type="button"
              disabled={setup.isPending}
              onClick={() => run(false)}
            >
              {pendingApply ? t('setup.applying') : t('setup.apply')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
