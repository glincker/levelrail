import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  FingerprintIcon,
  PlayIcon,
  PlusIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  isPasskeySupported,
  usePasskeys,
  useTestPasskey,
} from '../queries/passkeys'
import { classifyPasskeyError } from '@/lib/passkeyErrors'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { AddPasskeyDialog } from './passkeys/AddPasskeyDialog'
import { PasskeyRow } from './passkeys/PasskeyRow'

export function PasskeysCard() {
  const { t } = useTranslation('passkeys')
  const { data: passkeys } = usePasskeys()
  const [addOpen, setAddOpen] = useState(false)
  const test = useTestPasskey()
  const supported = isPasskeySupported()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FingerprintIcon className="size-4 text-muted-foreground" />
          {t('card.title')}
        </CardTitle>
        <CardDescription>{t('card.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {passkeys.length === 0 ? (
          <EmptyState
            compact
            icon={<FingerprintIcon className="size-5" />}
            title={t('empty.title')}
            description={t('empty.body')}
            action={
              <Button
                type="button"
                disabled={!supported}
                onClick={() => {
                  setAddOpen(true)
                }}
              >
                <PlusIcon />
                {t('empty.action')}
              </Button>
            }
          />
        ) : (
          <>
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <CheckCircleIcon
                className="size-4 text-tone-success"
                weight="fill"
              />
              {t('card.statusEnabled', { count: passkeys.length })}
            </p>
            <ul className="divide-y divide-border">
              {passkeys.map((p) => (
                <PasskeyRow
                  key={p.id}
                  passkey={p}
                  isLast={passkeys.length === 1}
                  onAddAnother={() => {
                    setAddOpen(true)
                  }}
                />
              ))}
            </ul>
            {passkeys.length === 1 ? (
              <Alert>
                <ShieldCheckIcon />
                <AlertTitle>{t('nudge.backupTitle')}</AlertTitle>
                <AlertDescription className="space-y-2">
                  <p>{t('nudge.backupBody')}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setAddOpen(true)
                    }}
                  >
                    <PlusIcon />
                    {t('nudge.backupAction')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : null}
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={!supported}
                onClick={() => {
                  setAddOpen(true)
                }}
              >
                <PlusIcon />
                {t('card.add')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                disabled={!supported || test.isPending}
                onClick={() => {
                  test.reset()
                  test.mutate()
                }}
              >
                <PlayIcon />
                {test.isPending ? t('card.testing') : t('card.test')}
              </Button>
            </div>
            {test.isSuccess ? (
              <Alert role="status">
                <CheckCircleIcon />
                <AlertDescription>{t('card.testOk')}</AlertDescription>
              </Alert>
            ) : null}
            {test.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertTitle>{t('card.testFailedTitle')}</AlertTitle>
                <AlertDescription className="space-y-2">
                  <p>{t(`errors.${classifyPasskeyError(test.error)}`)}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      test.reset()
                      test.mutate()
                    }}
                  >
                    {t('card.retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : null}
          </>
        )}
        {!supported ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{t('errors.unsupported')}</AlertDescription>
          </Alert>
        ) : null}
        <AddPasskeyDialog open={addOpen} onOpenChange={setAddOpen} />
      </CardContent>
    </Card>
  )
}
