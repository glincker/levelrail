import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  EyeIcon,
  EyeSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  credentialsStepUpScope,
  revealDatabasePassword,
} from '../../queries/databaseCredentials'
import { StepUpDialog } from '../env/StepUpDialog'

const HIDE_AFTER_MS = 30_000

export function RevealCredentialCard({
  databaseName,
}: Readonly<{ databaseName: string }>) {
  const { t } = useTranslation('databaseAccess')
  const [password, setPassword] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  const timer = useRef<number | null>(null)

  const hide = useCallback(() => {
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = null
    setPassword(null)
    setCopied(false)
  }, [])

  useEffect(() => hide, [hide])

  const onGateError = useCallback(
    (e: unknown) => {
      const message = e instanceof Error ? e.message : t('reveal.failed')
      setFailure(message)
      toast.add({ title: message, type: 'error' })
    },
    [t],
  )
  const gate = useStepUpGate(onGateError)

  const reveal = async () => {
    setFailure(null)
    await gate.guard(credentialsStepUpScope(databaseName), async () => {
      const value = await revealDatabasePassword(databaseName)
      setPassword(value)
      if (timer.current !== null) window.clearTimeout(timer.current)
      timer.current = window.setTimeout(hide, HIDE_AFTER_MS)
    })
  }

  const copy = () => {
    if (!password) return
    void navigator.clipboard.writeText(password).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <CardTitle>{t('reveal.title')}</CardTitle>
            <CardDescription>{t('reveal.description')}</CardDescription>
          </div>
          {password ? (
            <Button variant="outline" size="sm" onClick={hide}>
              <EyeSlashIcon className="size-3.5" aria-hidden="true" />
              {t('reveal.hide')}
            </Button>
          ) : (
            <Button variant="outline" size="sm" onClick={() => void reveal()}>
              <EyeIcon className="size-3.5" aria-hidden="true" />
              {t('reveal.show')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {password ? (
          <>
            <div className="flex items-start gap-2">
              <code className="min-w-0 flex-1 break-all rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
                {password}
              </code>
              <Button variant="outline" size="sm" onClick={copy}>
                {copied ? (
                  <CheckIcon aria-hidden="true" />
                ) : (
                  <CopyIcon aria-hidden="true" />
                )}
                {copied ? t('credential.copied') : t('credential.copy')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('reveal.autoHide')}
            </p>
          </>
        ) : null}
        {failure ? (
          <Alert variant="destructive">
            <AlertDescription>{failure}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </Card>
  )
}
