import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  emailSecretStepUpScope,
  useRevealEmailSecret,
  type EmailSecretField,
} from '../../queries/emailSecretReveal'
import { EnvApiError } from '../../queries/envProtection'
import { StepUpDialog } from '../env/StepUpDialog'

export const REVEAL_HIDE_AFTER_MS = 30_000

interface Props {
  providerId: string
  field: EmailSecretField
}

/** RevealEmailSecret shows one stored credential for 30 seconds after a step-up with two-factor. */
export function RevealEmailSecret({ providerId, field }: Readonly<Props>) {
  const { t } = useTranslation('settings')
  const [value, setValue] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const timer = useRef<number | null>(null)
  const mutation = useRevealEmailSecret(providerId)
  const { mutateAsync, reset } = mutation

  const hide = useCallback(() => {
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = null
    setValue(null)
    setCopied(false)
  }, [])

  useEffect(() => hide, [hide])

  const onGateError = useCallback(
    (e: unknown) => {
      if (e instanceof EnvApiError && e.code === 'two_factor_required') {
        setFailure(t('emailDelivery.reveal.needsTwoFactor'))
        return
      }
      setFailure(
        e instanceof Error ? e.message : t('emailDelivery.reveal.failed'),
      )
    },
    [t],
  )
  const gate = useStepUpGate(onGateError)

  const reveal = async () => {
    setFailure(null)
    await gate.guard(emailSecretStepUpScope(providerId), async () => {
      const revealed = await mutateAsync(field)
      reset()
      setValue(revealed)
      if (timer.current !== null) window.clearTimeout(timer.current)
      timer.current = window.setTimeout(hide, REVEAL_HIDE_AFTER_MS)
    })
  }

  const copy = () => {
    if (!value) return
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 2000)
    })
  }

  return (
    <div className="mt-1.5 space-y-2">
      {value ? (
        <>
          <div className="flex items-start gap-2">
            <code
              className="min-w-0 flex-1 break-all rounded-md bg-muted px-2 py-1.5 font-mono text-xs"
              data-testid="revealed-email-secret"
            >
              {value}
            </code>
            <Button type="button" variant="outline" size="sm" onClick={copy}>
              {copied ? (
                <CheckGlassIcon size="1.2em" aria-hidden="true" />
              ) : (
                <CopyGlassIcon size="1.2em" aria-hidden="true" />
              )}
              {copied
                ? t('emailDelivery.reveal.copied')
                : t('emailDelivery.reveal.copy')}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={hide}>
              <EyeOffGlassIcon size="1.2em" aria-hidden="true" />
              {t('emailDelivery.reveal.hide')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('emailDelivery.reveal.autoHide')}
          </p>
        </>
      ) : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            void reveal()
          }}
        >
          <EyeGlassIcon size="1.2em" aria-hidden="true" />
          {t('emailDelivery.reveal.show')}
        </Button>
      )}
      {failure ? (
        <Alert variant="destructive">
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      ) : null}
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        description={t('emailDelivery.reveal.stepUp')}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </div>
  )
}
