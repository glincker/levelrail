import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BanGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { InfoTip, RelativeTime, StatusPill, type Tone } from '@/components/kit'
import {
  useModelKeys,
  useRevokeModelKey,
  useRotateModelKey,
} from '../queries/modelKeys'
import type { ModelKey } from '../types/models'
import { CreateModelKeyDialog } from './CreateModelKeyDialog'
import { ModelKeyConfirmDialog, type KeyAction } from './ModelKeyConfirmDialog'
import { ModelKeyRevealDialog } from './ModelKeyRevealDialog'

const STATUS_TONE: Record<ModelKey['status'], Tone> = {
  active: 'success',
  rotating: 'warning',
  expired: 'neutral',
  revoked: 'neutral',
}

function limitLabel(k: ModelKey): string {
  const parts: string[] = []
  if (k.rpm > 0) parts.push(`${String(k.rpm)}/min`)
  if (k.tpm > 0) parts.push(`${String(k.tpm)} tok/min`)
  if (k.tpd > 0) parts.push(`${String(k.tpd)} tok/day`)
  if (k.max_parallel > 0) parts.push(`${String(k.max_parallel)} parallel`)
  if (k.allow_paths.length > 0)
    parts.push(`${String(k.allow_paths.length)} paths`)
  return parts.length > 0 ? parts.join(', ') : 'no limits'
}

function modelsLabel(k: ModelKey): string {
  return k.allow_models.length > 0 ? k.allow_models.join(', ') : 'all'
}

const COLUMNS: ResponsiveColumn<ModelKey>[] = [
  {
    id: 'name',
    header: 'Name',
    primary: true,
    cell: (k) => (
      <div className="text-xs">
        <div className="font-medium">{k.name}</div>
        <code className="font-mono text-muted-foreground">{k.key_prefix}</code>
        {k.created_by ? (
          <div className="text-muted-foreground">by {k.created_by}</div>
        ) : null}
      </div>
    ),
  },
  {
    id: 'status',
    header: 'Status',
    cell: (k) => (
      <StatusPill tone={STATUS_TONE[k.status]} label={k.status} size="sm" />
    ),
  },
  {
    id: 'models',
    header: 'Models',
    priority: 2,
    cell: (k) => (
      <span className="block truncate text-xs">{modelsLabel(k)}</span>
    ),
  },
  {
    id: 'limits',
    header: 'Limits',
    priority: 2,
    cell: (k) => <span className="text-xs">{limitLabel(k)}</span>,
  },
  {
    id: 'lastUsed',
    header: 'Last used',
    priority: 3,
    cell: (k) => (
      <span className="text-xs">
        {k.last_used_at ? <RelativeTime at={k.last_used_at} /> : 'never'}
      </span>
    ),
  },
  {
    id: 'expires',
    header: 'Expires',
    priority: 3,
    cell: (k) => (
      <span className="text-xs">
        {k.expires_at ? <RelativeTime at={k.expires_at} /> : 'never'}
      </span>
    ),
  },
]

interface PendingAction {
  action: KeyAction
  key: ModelKey
}

export function ModelKeysPanel({
  modelName,
  baseUrl,
}: {
  modelName: string
  baseUrl?: string
}) {
  const { t } = useTranslation('common')
  const keys = useModelKeys(modelName)
  const revoke = useRevokeModelKey(modelName)
  const rotate = useRotateModelKey(modelName)
  const [revealed, setRevealed] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<PendingAction | null>(null)

  function fail(title: string) {
    return (error: Error) => {
      toast.add({ title, description: error.message, type: 'error' })
      setConfirm(null)
    }
  }

  function confirmAction(graceSeconds?: number) {
    if (!confirm) return
    if (confirm.action === 'revoke') {
      revoke.mutate(confirm.key.id, {
        onSuccess: () => {
          setConfirm(null)
        },
        onError: fail('Could not revoke the key.'),
      })
      return
    }
    rotate.mutate(
      { id: confirm.key.id, graceSeconds },
      {
        onSuccess: (res) => {
          setConfirm(null)
          setRevealed(res.api_key)
        },
        onError: fail('Could not rotate the key.'),
      },
    )
  }

  return (
    <section aria-label="API keys" className="space-y-2">
      <div className="flex flex-wrap items-center justify-between">
        <h3 className="flex items-center gap-1 text-sm font-semibold">
          API keys
          <InfoTip label="About API keys">
            Each client gets its own named key with its own limits. Only a hash
            is stored, so a key is shown once when it is created.
          </InfoTip>
        </h3>
        <CreateModelKeyDialog modelName={modelName} onCreated={setRevealed} />
      </div>
      {keys.isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : keys.error ? (
        <p className="text-sm text-destructive">{keys.error.message}</p>
      ) : (
        <ResponsiveTable
          rows={keys.data}
          columns={COLUMNS}
          getRowId={(k) => k.id}
          ariaLabel={t('tableLabels.modelKeys')}
          renderActions={(k) => {
            const inactive = k.status === 'revoked' || k.status === 'expired'
            return (
              <span className="flex items-center whitespace-nowrap">
                {inactive || k.status === 'rotating' ? null : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Rotate key ${k.name}`}
                    title="Rotate, old key keeps working for the grace window"
                    onClick={() => {
                      setConfirm({ action: 'rotate', key: k })
                    }}
                  >
                    <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
                  </Button>
                )}
                {k.status === 'revoked' ? null : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Revoke key ${k.name}`}
                    title="Revoke now"
                    onClick={() => {
                      setConfirm({ action: 'revoke', key: k })
                    }}
                  >
                    <BanGlassIcon size="1.4em" aria-hidden="true" />
                  </Button>
                )}
              </span>
            )
          }}
        />
      )}
      {confirm ? (
        <ModelKeyConfirmDialog
          action={confirm.action}
          target={confirm.key}
          pending={revoke.isPending || rotate.isPending}
          onConfirm={confirmAction}
          onClose={() => {
            setConfirm(null)
          }}
        />
      ) : null}
      {revealed ? (
        <ModelKeyRevealDialog
          modelName={modelName}
          apiKey={revealed}
          baseUrl={baseUrl}
          onClose={() => {
            setRevealed(null)
          }}
        />
      ) : null}
    </section>
  )
}
