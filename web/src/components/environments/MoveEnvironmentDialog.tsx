import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsLeftRightIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useGlobalEnvironments,
  useMoveEnvironment,
  type MoveTarget,
} from '../../queries/globalEnvironments'

const NONE = '__none__'

// Moves an app or database between environments. A move that touches a
// protected environment needs an explicit acknowledgement and then waits
// for another person's approval.
export function MoveEnvironmentDialog({
  kind,
  name,
  currentEnvironmentId,
}: {
  kind: MoveTarget
  name: string
  currentEnvironmentId?: string
}) {
  const { t } = useTranslation('environments')
  const [open, setOpen] = useState(false)
  const [target, setTarget] = useState(NONE)
  const [acknowledged, setAcknowledged] = useState(false)
  const { data: environments = [] } = useGlobalEnvironments()
  const move = useMoveEnvironment(kind)

  const resolved = target === NONE ? '' : target
  const current = currentEnvironmentId ?? ''
  const source = environments.find((e) => e.id === current)
  const dest = environments.find((e) => e.id === resolved)
  const gate = dest?.protected ? dest : source?.protected ? source : undefined
  const noop = resolved === current

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setTarget(current === '' ? NONE : current)
      setAcknowledged(false)
    } else {
      move.reset()
    }
  }

  function submit() {
    move.mutate(
      { name, environmentId: resolved, confirm: gate !== undefined },
      {
        onSuccess: (result) => {
          setOpen(false)
          toast.add({
            title: t(result.pending_approval ? 'move.pending' : 'move.moved', {
              name,
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('move.failed', { name }),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsLeftRightIcon className="size-3.5" aria-hidden="true" />
        {t('move.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('move.title', { name })}</DialogTitle>
          <DialogDescription>{t('move.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <label
            htmlFor="move-environment-target"
            className="text-sm font-medium text-foreground"
          >
            {t('move.field')}
          </label>
          <Select
            value={target}
            onValueChange={(next) => {
              if (next) {
                setTarget(next)
                setAcknowledged(false)
              }
            }}
          >
            <SelectTrigger id="move-environment-target" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>{t('move.none')}</SelectItem>
              {environments
                .filter((e) => e.kind !== 'preview')
                .map((e) => (
                  <SelectItem key={e.id} value={e.id}>
                    {e.name}
                    {e.id === current ? ` ${t('move.current')}` : ''}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
        </div>
        {gate && !noop ? (
          <div className="space-y-3 rounded-lg border border-border bg-muted/40 p-3 text-sm">
            <p className="flex items-start gap-2 text-foreground">
              <WarningIcon
                className="mt-0.5 size-4 shrink-0 text-amber-600"
                aria-hidden="true"
              />
              {t('move.protectedNotice', { environment: gate.name })}
            </p>
            <label className="flex items-center gap-2 text-muted-foreground">
              <Checkbox
                checked={acknowledged}
                onCheckedChange={setAcknowledged}
                aria-label={t('move.confirmLabel')}
              />
              {t('move.confirmLabel')}
            </label>
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('move.cancel')}
          </Button>
          <Button
            type="button"
            disabled={
              move.isPending || noop || (gate !== undefined && !acknowledged)
            }
            onClick={submit}
          >
            {move.isPending
              ? t('move.saving')
              : t(gate ? 'move.request' : 'move.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
