import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePurgeAppCache, type PurgeScope } from '../../queries/cdnCache'
import { PurgeScopePicker } from './PurgeScopePicker'

const PLACEHOLDER: Record<Exclude<PurgeScope, 'all'>, string> = {
  url: '/blog/post?id=1',
  prefix: '/static/',
  tag: 'blog',
}

/**
 * PurgePanel purges an app's cached responses, or one domain's when `domain`
 * is set. Purging everything asks for the app or domain name first.
 */
export function PurgePanel({
  app,
  domain,
}: Readonly<{ app: string; domain?: string }>) {
  const { t } = useTranslation('cdn')
  const purge = usePurgeAppCache(app)
  const [scope, setScope] = useState<PurgeScope>('url')
  const [value, setValue] = useState('')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const target = domain ?? app
  const needsValue = scope !== 'all'
  const ready = !needsValue || value.trim().length > 0

  function run() {
    purge.mutate(
      { scope, value: needsValue ? value.trim() : undefined, domain },
      {
        onSuccess: () => {
          setConfirmOpen(false)
          setValue('')
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <PurgeScopePicker
        value={scope}
        onChange={(next) => {
          setScope(next)
          purge.reset()
        }}
        disabled={purge.isPending}
      />
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (!ready) return
          if (scope === 'all') setConfirmOpen(true)
          else run()
        }}
      >
        {needsValue ? (
          <div className="min-w-48 flex-1 space-y-1.5">
            <Label htmlFor="cdn-purge-value">
              {t(`purge.scope.${scope}.field`)}
            </Label>
            <Input
              id="cdn-purge-value"
              value={value}
              placeholder={PLACEHOLDER[scope]}
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setValue(e.target.value)}
            />
          </div>
        ) : (
          <p className="min-w-48 flex-1 text-sm text-muted-foreground">
            {t('purge.allNote', { target })}
          </p>
        )}
        <Button
          type="submit"
          variant={scope === 'all' ? 'destructive' : 'default'}
          writeAction
          disabled={!ready || purge.isPending}
        >
          <BroomGlassIcon size="1.4em" aria-hidden="true" />
          {purge.isPending ? t('purge.purging') : t('purge.action')}
        </Button>
      </form>
      {purge.isSuccess ? (
        <Alert>
          <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          <AlertDescription aria-live="polite">
            {t('purge.done', { count: purge.data.removed })}
          </AlertDescription>
        </Alert>
      ) : null}
      {purge.isError && !confirmOpen ? (
        <Alert variant="destructive">
          <AlertDescription>{purge.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('purge.confirmTitle', { target })}
        description={t('purge.confirmBody')}
        consequences={[
          t('purge.consequenceOrigin'),
          t('purge.consequenceUndo'),
        ]}
        confirmLabel={t('purge.confirm')}
        pendingLabel={t('purge.purging')}
        tone="destructive"
        requireTyped={target}
        pending={purge.isPending}
        error={purge.isError ? purge.error.message : null}
        onConfirm={run}
      />
    </div>
  )
}
