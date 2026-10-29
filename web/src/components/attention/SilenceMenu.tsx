import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BellOffGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toast, toastAction } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import {
  SILENCE_DURATIONS,
  useCreateSilence,
  useDeleteSilence,
  type SilenceDuration,
} from '../../queries/silences'

// Silences one status issue on the server for 1h, 1d, 1w, 30d or forever.
// Forever and critical issues ask first, and the server asks again for
// security and data-loss issues (409), which reopens the same dialog.
export function SilenceMenu({
  issueKey,
  severity,
  title,
  scope,
}: {
  issueKey: string
  severity: string
  title: string
  scope?: string
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const create = useCreateSilence()
  const remove = useDeleteSilence()
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState<SilenceDuration | null>(null)
  const [error, setError] = useState<string | null>(null)

  const submit = (duration: SilenceDuration, confirm: boolean) => {
    setError(null)
    create.mutate(
      { issueKey, duration, severity, scope, reason: reason.trim(), confirm },
      {
        onSuccess: (s) => {
          setPending(null)
          setOpen(false)
          setReason('')
          toast.add({
            title: t('silence.done', { title }),
            type: 'success',
            actionProps: toastAction(t('silence.undo'), () => {
              remove.mutate(s.id)
            }),
          })
        },
        onError: (err) => {
          if (err instanceof ApiError && err.status === 409) {
            setPending(duration)
            return
          }
          setError(err.message)
          toast.add({ title: err.message, type: 'error' })
        },
      },
    )
  }

  const choose = (duration: SilenceDuration) => {
    if (duration === 'forever' || severity === 'critical') {
      setPending(duration)
      return
    }
    submit(duration, false)
  }

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type="button"
              size="sm"
              variant="ghost"
              aria-label={t('silence.menuLabel', { title })}
            />
          }
        >
          <BellOffGlassIcon size="1.2em" />
          {t('silence.button')}
        </PopoverTrigger>
        <PopoverContent align="end" className="w-60">
          <div className="space-y-2">
            <Input
              value={reason}
              maxLength={500}
              placeholder={t('silence.reasonPlaceholder')}
              aria-label={t('silence.reason')}
              onChange={(e) => {
                setReason(e.target.value)
              }}
            />
            <ul className="space-y-0.5">
              {SILENCE_DURATIONS.map((d) => (
                <li key={d}>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="w-full justify-start"
                    disabled={create.isPending}
                    onClick={() => {
                      choose(d)
                    }}
                  >
                    {t(`silence.for.${d}`)}
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        </PopoverContent>
      </Popover>
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(next) => {
          if (!next) setPending(null)
        }}
        title={t('silence.confirmTitle', { title })}
        description={t(
          pending === 'forever'
            ? 'silence.confirmForever'
            : 'silence.confirmCritical',
        )}
        confirmLabel={t('silence.confirm')}
        tone="destructive"
        pending={create.isPending}
        error={error}
        onConfirm={() => {
          if (pending) submit(pending, true)
        }}
      />
    </>
  )
}
