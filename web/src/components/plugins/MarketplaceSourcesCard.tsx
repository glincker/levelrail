import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useAddMarketplaceSource,
  useRefreshMarketplace,
  useRemoveMarketplaceSource,
  type MarketplaceSource,
} from '../../queries/pluginMarketplace'

export function SourceProblem({
  source,
}: Readonly<{ source: MarketplaceSource }>) {
  const { t } = useTranslation('plugins')
  const refresh = useRefreshMarketplace()
  if (source.status === 'ok') return null
  const kind = source.error_kind || 'fetch'
  return (
    <Alert variant={source.status === 'error' ? 'destructive' : 'default'}>
      <AlertTitle>
        {t(`marketplace.problem.${kind}`, { name: source.name })}
      </AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{source.error}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={refresh.isPending}
          onClick={() => refresh.mutate(source.id)}
        >
          <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
          {refresh.isPending
            ? t('marketplace.rechecking')
            : t('marketplace.recheck')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}

export function MarketplaceSourcesCard({
  sources,
}: Readonly<{ sources: MarketplaceSource[] }>) {
  const { t } = useTranslation('plugins')
  const add = useAddMarketplaceSource()
  const remove = useRemoveMarketplaceSource()
  const refresh = useRefreshMarketplace()
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [rootKey, setRootKey] = useState('')
  const [target, setTarget] = useState<MarketplaceSource | null>(null)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('marketplace.sources.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('marketplace.sources.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {sources.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('marketplace.sources.empty')}
          </p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {sources.map((s) => (
              <li
                key={s.id}
                className="flex flex-wrap items-center justify-between gap-2 p-3"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium">
                    {s.name}{' '}
                    <span className="font-normal text-muted-foreground">
                      {t(`marketplace.sources.status.${s.status}`)}
                    </span>
                  </p>
                  <p className="text-xs break-all text-muted-foreground">
                    {s.url}
                  </p>
                  <p className="text-xs break-all text-muted-foreground">
                    {t('marketplace.sources.rootKey', { key: s.root_key_id })}
                  </p>
                </div>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={refresh.isPending}
                    onClick={() => refresh.mutate(s.id)}
                  >
                    {t('marketplace.recheck')}
                  </Button>
                  {s.builtin ? null : (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => setTarget(s)}
                    >
                      {t('marketplace.sources.remove')}
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
        <form
          className="grid gap-3 sm:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault()
            add.mutate(
              { name, url, root_public_key: rootKey },
              {
                onSuccess: () => {
                  setName('')
                  setUrl('')
                  setRootKey('')
                  toast.add({
                    type: 'success',
                    title: t('marketplace.sources.added'),
                  })
                },
              },
            )
          }}
        >
          <Field>
            <FieldLabel htmlFor="mk-src-name">
              {t('marketplace.sources.name')}
            </FieldLabel>
            <Input
              id="mk-src-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mk-src-url">
              {t('marketplace.sources.url')}
            </FieldLabel>
            <Input
              id="mk-src-url"
              value={url}
              placeholder="https://"
              onChange={(e) => setUrl(e.target.value)}
              required
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mk-src-key">
              {t('marketplace.sources.rootKeyLabel')}
            </FieldLabel>
            <Input
              id="mk-src-key"
              value={rootKey}
              onChange={(e) => setRootKey(e.target.value)}
              required
            />
          </Field>
          <div className="sm:col-span-3">
            {add.error ? (
              <p role="alert" className="mb-2 text-sm text-destructive">
                {add.error.message}
              </p>
            ) : null}
            <Button type="submit" disabled={add.isPending}>
              {t('marketplace.sources.add')}
            </Button>
          </div>
        </form>
      </CardContent>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => (open ? undefined : setTarget(null))}
        title={t('marketplace.sources.removeTitle', { name: target?.name })}
        description={t('marketplace.sources.removeDescription')}
        confirmLabel={t('marketplace.sources.remove')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          if (target)
            remove.mutate(target.id, { onSuccess: () => setTarget(null) })
        }}
      />
    </Card>
  )
}
