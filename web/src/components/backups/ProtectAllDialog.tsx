import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  backupCoverageKeys,
  postProtectAll,
  useProtectAll,
} from '../../queries/backupCoverage'
import type {
  BackupDefaults,
  CoverageTarget,
  ProtectPlanItem,
} from '../../types/backupCoverage'
import { TargetSetupForm } from './TargetSetupForm'

const SCHEDULE_CHOICES = [
  { id: 'daily', cron: '0 3 * * *' },
  { id: 'sixHours', cron: '0 */6 * * *' },
  { id: 'weekly', cron: '0 3 * * 0' },
] as const

function itemLabel(item: ProtectPlanItem): string {
  return item.app_name ? `${item.app_name}/${item.name}` : item.name
}

/** ProtectAllDialog picks a destination and schedule, previews the plan and applies it to every unprotected resource. */
export function ProtectAllDialog({
  open,
  onOpenChange,
  targets,
  defaults,
  scratchCount,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  targets: CoverageTarget[]
  defaults: BackupDefaults
  scratchCount: number
}) {
  const { t } = useTranslation('backups')
  const apply = useProtectAll()
  const [targetId, setTargetId] = useState('')
  const [adding, setAdding] = useState(false)
  const [schedule, setSchedule] = useState('')
  const [retain, setRetain] = useState('')
  const [includeScratch, setIncludeScratch] = useState(false)

  const effectiveTarget = targetId || targets[0]?.id || ''
  const effectiveSchedule = schedule || defaults.schedule
  const retainValue = retain === '' ? defaults.retain : Number(retain)
  const retainValid = Number.isInteger(retainValue) && retainValue >= 0
  const request = {
    target_id: effectiveTarget,
    schedule: effectiveSchedule,
    retain: retainValue,
    include_scratch: includeScratch,
  }

  const plan = useQuery({
    queryKey: [...backupCoverageKeys.all, 'plan', request],
    queryFn: () => postProtectAll({ ...request, apply: false }),
    enabled: open && !adding && effectiveTarget !== '' && retainValid,
    staleTime: 0,
  })

  function close(next: boolean) {
    onOpenChange(next)
    if (!next) {
      apply.reset()
      setAdding(false)
    }
  }

  function run() {
    apply.mutate(
      { ...request, apply: true },
      {
        onSuccess: (res) => {
          const failed = res.failed?.length ?? 0
          toast.add({
            title: t('protectAll.done', { count: res.done?.length ?? 0 }),
            description:
              failed > 0
                ? t('protectAll.someFailed', { count: failed })
                : undefined,
            type: failed > 0 ? 'warning' : 'success',
          })
          if (failed === 0) close(false)
        },
      },
    )
  }

  const items = plan.data?.plan ?? []
  const addingFirst = targets.length === 0 || adding

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheckGlassIcon
              className="size-4.5 text-tone-success"
              aria-hidden="true"
            />
            {t('protectAll.title')}
          </DialogTitle>
          <DialogDescription>{t('protectAll.description')}</DialogDescription>
        </DialogHeader>

        <section className="space-y-2" aria-labelledby="protect-destination">
          <h3 id="protect-destination" className="text-sm font-medium">
            {t('protectAll.destination')}
          </h3>
          {addingFirst ? (
            <TargetSetupForm
              onCreated={(target) => {
                setTargetId(target.id)
                setAdding(false)
              }}
            />
          ) : (
            <div className="space-y-2">
              <div
                role="radiogroup"
                aria-labelledby="protect-destination"
                className="space-y-1"
              >
                {targets.map((target) => (
                  <label
                    key={target.id}
                    className="flex cursor-pointer items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
                  >
                    <input
                      type="radio"
                      name="protect-target"
                      checked={target.id === effectiveTarget}
                      onChange={() => {
                        setTargetId(target.id)
                      }}
                    />
                    <span className="font-medium">{target.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {target.provider}
                    </span>
                  </label>
                ))}
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setAdding(true)
                }}
              >
                {t('protectAll.addDestination')}
              </Button>
            </div>
          )}
        </section>

        {!addingFirst ? (
          <>
            <section className="space-y-2" aria-labelledby="protect-schedule">
              <h3 id="protect-schedule" className="text-sm font-medium">
                {t('protectAll.schedule')}
              </h3>
              <div className="flex flex-wrap gap-2">
                {SCHEDULE_CHOICES.map((c) => (
                  <Button
                    key={c.id}
                    type="button"
                    size="sm"
                    variant={
                      effectiveSchedule === c.cron ? 'default' : 'outline'
                    }
                    aria-pressed={effectiveSchedule === c.cron}
                    onClick={() => {
                      setSchedule(c.cron)
                    }}
                  >
                    {t(`protectAll.choice.${c.id}`)}
                  </Button>
                ))}
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="protect-cron">{t('protectAll.cron')}</Label>
                  <Input
                    id="protect-cron"
                    className="font-mono"
                    value={effectiveSchedule}
                    onChange={(e) => {
                      setSchedule(e.target.value)
                    }}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="protect-retain">
                    {t('protectAll.retain')}
                  </Label>
                  <Input
                    id="protect-retain"
                    type="number"
                    min={0}
                    value={retain === '' ? String(defaults.retain) : retain}
                    onChange={(e) => {
                      setRetain(e.target.value)
                    }}
                  />
                </div>
              </div>
              {scratchCount > 0 ? (
                <div className="flex items-start gap-2">
                  <Checkbox
                    id="protect-scratch"
                    checked={includeScratch}
                    onCheckedChange={(next) => {
                      setIncludeScratch(next === true)
                    }}
                    className="mt-0.5"
                  />
                  <Label htmlFor="protect-scratch" className="font-normal">
                    {t('protectAll.includeScratch', { count: scratchCount })}
                  </Label>
                </div>
              ) : null}
            </section>

            <section className="space-y-2" aria-labelledby="protect-plan">
              <h3 id="protect-plan" className="text-sm font-medium">
                {t('protectAll.plan')}
              </h3>
              {plan.isError ? (
                <p role="alert" className="text-sm text-destructive">
                  {plan.error.message}
                </p>
              ) : plan.isPending ? (
                <p className="text-sm text-muted-foreground">
                  {t('protectAll.planning')}
                </p>
              ) : items.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('protectAll.nothingToDo')}
                </p>
              ) : (
                <ul className="max-h-40 space-y-1 overflow-y-auto text-sm">
                  {items.map((item) => (
                    <li
                      key={`${item.kind}:${itemLabel(item)}`}
                      className="flex items-center justify-between gap-2 rounded border border-border px-2 py-1"
                    >
                      <span className="truncate font-medium">
                        {itemLabel(item)}
                      </span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {t(`protectAll.kind.${item.kind}`)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              {apply.data?.failed?.length ? (
                <ul role="alert" className="space-y-1 text-sm text-destructive">
                  {apply.data.failed.map((f) => (
                    <li key={itemLabel(f)}>
                      {itemLabel(f)}: {f.error}
                    </li>
                  ))}
                </ul>
              ) : null}
              {apply.isError ? (
                <p role="alert" className="text-sm text-destructive">
                  {apply.error.message}
                </p>
              ) : null}
            </section>
          </>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              close(false)
            }}
          >
            {t('protectAll.cancel')}
          </Button>
          <Button
            type="button"
            disabled={
              addingFirst ||
              apply.isPending ||
              plan.isPending ||
              items.length === 0 ||
              !retainValid
            }
            onClick={run}
          >
            {apply.isPending
              ? t('protectAll.applying')
              : t('protectAll.apply', { count: items.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
