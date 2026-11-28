import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useControlPlaneDr } from '../../queries/controlPlaneDr'
import {
  useConfirmSameInstance,
  useInstanceIdentity,
} from '../../queries/instance'

// Wire value the API requires, not display copy.
const CONFIRM_PHRASE = 'same instance'

/** CliServerCard shows the identity the CLI pins at login and the Fix for a restored instance. */
export function CliServerCard() {
  const { t } = useTranslation('settings')
  const { data } = useInstanceIdentity()
  const { data: backup } = useControlPlaneDr()
  const confirm = useConfirmSameInstance()
  const [typed, setTyped] = useState('')
  if (!data) return null
  const phrase = CONFIRM_PHRASE

  return (
    <section className="space-y-3 rounded-lg border border-border p-4">
      <div>
        <h2 className="text-sm font-medium">
          {t('cliAccessPage.server.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('cliAccessPage.server.body')}
        </p>
      </div>
      <dl className="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">
          {t('cliAccessPage.server.name')}
        </dt>
        <dd>{data.name}</dd>
        <dt className="text-muted-foreground">
          {t('cliAccessPage.server.id')}
        </dt>
        <dd className="flex items-center gap-2">
          <span className="font-mono text-xs break-all">
            {data.instance_id}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              void navigator.clipboard
                .writeText(data.instance_id)
                .then(() => {
                  toast.add({
                    title: t('cliAccessPage.server.copied'),
                    type: 'success',
                  })
                })
                .catch(() => undefined)
            }}
          >
            <CopyGlassIcon size="1.4em" />
            {t('cliAccessPage.server.copy')}
          </Button>
        </dd>
        <dt className="text-muted-foreground">
          {t('cliAccessPage.server.fingerprint')}
        </dt>
        <dd className="font-mono text-xs break-all">{data.fingerprint}</dd>
        {backup?.install_id ? (
          <>
            <dt className="text-muted-foreground">
              {t('cliAccessPage.server.backupSetId')}
            </dt>
            <dd className="font-mono text-xs break-all">{backup.install_id}</dd>
          </>
        ) : null}
      </dl>
      {data.moved ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertTitle>{t('cliAccessPage.server.movedTitle')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{t('cliAccessPage.server.movedBody')}</p>
            <label className="block text-xs" htmlFor="confirm-same-instance">
              {t('cliAccessPage.server.confirmLabel', { phrase })}
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <Input
                id="confirm-same-instance"
                className="max-w-56"
                value={typed}
                onChange={(e) => {
                  setTyped(e.target.value)
                }}
              />
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={typed !== phrase || confirm.isPending}
                onClick={() => {
                  confirm.mutate(phrase, {
                    onSuccess: () => {
                      setTyped('')
                      toast.add({
                        title: t('cliAccessPage.server.confirmed'),
                        type: 'success',
                      })
                    },
                    onError: (error) => {
                      toast.add({ title: error.message, type: 'error' })
                    },
                  })
                }}
              >
                {t('cliAccessPage.server.confirmAction')}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : null}
    </section>
  )
}
