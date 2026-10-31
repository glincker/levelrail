import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  BotGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { CopyCommand } from '@/components/settings/CopyCommand'
import { useIsRoot } from '@/hooks/useIsRoot'
import { devModeQueryOptions } from '../../queries/devMode'
import {
  useCreateMcpClient,
  useDeleteMcpClient,
  useMcpClients,
  type McpClient,
} from '../../queries/mcpOauth'

const MCP_FEATURE = 'mcp-http'

function splitUris(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((u) => u.trim())
    .filter((u) => u !== '')
}

/** ConnectedClientsCard lists pre-registered AI clients; root only, hidden while mcp-http is off. */
export function ConnectedClientsCard() {
  const isRoot = useIsRoot()
  const { data: dev } = useQuery(devModeQueryOptions())
  const on = isRoot && !!dev?.experimental.includes(MCP_FEATURE)
  if (!on) return null
  return <ClientsBody />
}

function ClientsBody() {
  const { t } = useTranslation('mcpConsent')
  const clients = useMcpClients(true)
  const create = useCreateMcpClient()
  const remove = useDeleteMcpClient()
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [uris, setUris] = useState('')
  const [created, setCreated] = useState<McpClient | null>(null)
  const [doomed, setDoomed] = useState<McpClient | null>(null)

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    create.mutate(
      { name: name.trim(), redirect_uris: splitUris(uris) },
      {
        onSuccess: (c) => {
          setCreated(c)
          setAdding(false)
          setName('')
          setUris('')
        },
      },
    )
  }

  return (
    <section className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-medium">
            <BotGlassIcon className="size-5" aria-hidden="true" />
            {t('clients.title')}
          </h2>
          <p className="text-sm text-muted-foreground">{t('clients.body')}</p>
        </div>
        {adding ? null : (
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <PlusGlassIcon aria-hidden="true" />
            {t('clients.add')}
          </Button>
        )}
      </div>

      {created ? (
        <Banner variant="success" title={t('clients.created')}>
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1">
              <CopyCommand label={t('clients.clientId')} command={created.id} />
            </div>
            <Button size="sm" variant="ghost" onClick={() => setCreated(null)}>
              <XGlassIcon aria-hidden="true" />
              {t('clients.dismiss')}
            </Button>
          </div>
        </Banner>
      ) : null}

      {adding ? (
        <form
          onSubmit={onSubmit}
          className="grid items-end gap-2 sm:grid-cols-[1fr_2fr_auto]"
        >
          <Field>
            <FieldLabel htmlFor="mcp-client-name">
              {t('clients.name')}
            </FieldLabel>
            <Input
              id="mcp-client-name"
              value={name}
              maxLength={64}
              required
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mcp-client-uris">
              {t('clients.uris')}
            </FieldLabel>
            <Input
              id="mcp-client-uris"
              value={uris}
              required
              placeholder={t('clients.urisPlaceholder')}
              onChange={(e) => setUris(e.target.value)}
            />
          </Field>
          <div className="flex gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={create.isPending || splitUris(uris).length === 0}
            >
              {t('clients.create')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setAdding(false)}
            >
              {t('clients.cancel')}
            </Button>
          </div>
        </form>
      ) : null}
      {create.error ? (
        <Banner variant="critical" title={create.error.message} />
      ) : null}

      {clients.error ? (
        <Banner variant="critical" title={t('clients.loadError')} />
      ) : null}
      {clients.isPending ? <ListSkeleton rows={2} /> : null}
      {clients.data && clients.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('clients.empty')}</p>
      ) : null}
      {clients.data && clients.data.length > 0 ? (
        <ul className="divide-y divide-border text-sm">
          {clients.data.map((c) => (
            <li
              key={c.id}
              className="flex items-start justify-between gap-3 py-2"
            >
              <div className="min-w-0">
                <p className="font-medium">{c.name}</p>
                <p className="font-mono text-xs break-all text-muted-foreground">
                  {c.redirect_uris.join(', ')}
                </p>
                <p className="text-xs text-muted-foreground">
                  {c.id} · {new Date(c.created_at).toLocaleDateString()}
                </p>
              </div>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={t('clients.remove', { name: c.name })}
                onClick={() => setDoomed(c)}
              >
                <TrashGlassIcon
                  className="size-5 hover:text-destructive"
                  aria-hidden="true"
                />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}

      <ConfirmDialog
        open={doomed !== null}
        onOpenChange={(o) => {
          if (!o) setDoomed(null)
        }}
        title={t('clients.confirmTitle', { name: doomed?.name ?? '' })}
        description={t('clients.confirmBody')}
        confirmLabel={t('clients.confirm')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          if (doomed)
            remove.mutate(doomed.id, { onSuccess: () => setDoomed(null) })
        }}
      />
    </section>
  )
}
