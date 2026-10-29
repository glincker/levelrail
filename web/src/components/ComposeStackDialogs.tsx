import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  RotateCcwGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  useComposeDown,
  useComposeRevisions,
  useRollbackCompose,
} from '../queries/composeStack'
import { formatAge } from '../lib/format'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

export function ComposeRollbackDialog({
  appId,
  currentRevision,
}: {
  appId: string
  currentRevision?: number
}) {
  const { t } = useTranslation('compose')
  const [open, setOpen] = useState(false)
  const [picked, setPicked] = useState<string | null>(null)
  const revisions = useComposeRevisions(appId, open)
  const rollback = useRollbackCompose(appId)
  const earlier = (revisions.data ?? []).filter(
    (r) => r.revision !== currentRevision,
  )
  const target = picked ?? (earlier[0] ? String(earlier[0].revision) : null)

  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          setOpen(true)
        }}
      >
        <RotateCcwGlassIcon size="1.4em" />
        {t('rollback.button')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('rollback.title')}</DialogTitle>
            <DialogDescription>{t('rollback.description')}</DialogDescription>
          </DialogHeader>
          {revisions.isSuccess && earlier.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('rollback.none')}
            </p>
          ) : (
            <Field>
              <FieldLabel htmlFor="compose-rollback-revision">
                {t('rollback.revisionLabel')}
              </FieldLabel>
              <Select
                value={target}
                onValueChange={(v) => {
                  setPicked(v)
                }}
              >
                <SelectTrigger
                  id="compose-rollback-revision"
                  className="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {earlier.map((r) => (
                    <SelectItem key={r.revision} value={String(r.revision)}>
                      {t('rollback.revisionOption', {
                        revision: r.revision,
                        source: r.source,
                        when: formatAge(r.created_at),
                      })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
          {rollback.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
              />
              <AlertDescription>{rollback.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              disabled={!target || rollback.isPending}
              onClick={() => {
                rollback.mutate(Number(target), {
                  onSuccess: (res) => {
                    toast.add({
                      title: t('rollback.done', {
                        revision: res.rolled_back_to,
                      }),
                      type: 'success',
                    })
                    setOpen(false)
                  },
                })
              }}
            >
              {rollback.isPending
                ? t('rollback.working')
                : t('rollback.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

export function ComposeDownDialog({ appId }: { appId: string }) {
  const { t } = useTranslation('compose')
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [removeVolumes, setRemoveVolumes] = useState(false)
  const down = useComposeDown(appId)

  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="destructive"
        onClick={() => {
          setOpen(true)
        }}
      >
        <TrashGlassIcon size="1.4em" />
        {t('down.button')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('down.title', { app: appId })}</DialogTitle>
            <DialogDescription>{t('down.description')}</DialogDescription>
          </DialogHeader>
          <Field orientation="horizontal">
            <Checkbox
              id="compose-down-volumes"
              checked={removeVolumes}
              onCheckedChange={(checked) => {
                setRemoveVolumes(checked === true)
              }}
            />
            <FieldLabel htmlFor="compose-down-volumes">
              {t('down.removeVolumes')}
            </FieldLabel>
          </Field>
          {down.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
              />
              <AlertDescription>{down.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setOpen(false)
              }}
            >
              {t('down.cancel')}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={down.isPending}
              onClick={() => {
                down.mutate(removeVolumes, {
                  onSuccess: (res) => {
                    toast.add({
                      title: t('down.done', { count: res.removed.length }),
                      description: res.volumes_kept?.length
                        ? t('down.volumesKept', {
                            volumes: res.volumes_kept.join(', '),
                          })
                        : undefined,
                      type: 'success',
                    })
                    setOpen(false)
                    void navigate({ to: '/apps' })
                  },
                })
              }}
            >
              {down.isPending ? t('down.working') : t('down.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
