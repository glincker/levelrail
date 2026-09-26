import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  ProhibitIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useModelKeys,
  useRevokeModelKey,
  useRotateModelKey,
} from '../queries/modelKeys'
import type { ModelKey } from '../types/models'
import { CreateModelKeyDialog } from './CreateModelKeyDialog'
import { ModelKeyRevealDialog } from './ModelKeyRevealDialog'

function limitLabel(k: ModelKey): string {
  const parts: string[] = []
  if (k.rpm > 0) parts.push(`${String(k.rpm)}/min`)
  if (k.tpm > 0) parts.push(`${String(k.tpm)} tok/min`)
  if (k.max_parallel > 0) parts.push(`${String(k.max_parallel)} parallel`)
  if (k.allow_paths.length > 0)
    parts.push(`${String(k.allow_paths.length)} paths`)
  if (k.allow_models.length > 0)
    parts.push(`${String(k.allow_models.length)} models`)
  return parts.length > 0 ? parts.join(', ') : 'no limits'
}

function when(iso?: string): string {
  return iso ? new Date(iso).toLocaleString() : 'never'
}

function statusVariant(status: ModelKey['status']): 'success' | 'muted' {
  return status === 'active' ? 'success' : 'muted'
}

export function ModelKeysPanel({
  modelName,
  baseUrl,
}: {
  modelName: string
  baseUrl?: string
}) {
  const keys = useModelKeys(modelName)
  const revoke = useRevokeModelKey(modelName)
  const rotate = useRotateModelKey(modelName)
  const [revealed, setRevealed] = useState<string | null>(null)

  function fail(title: string) {
    return (error: Error) => {
      toast.add({ title, description: error.message, type: 'error' })
    }
  }

  return (
    <section aria-label="API keys" className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">API keys</h3>
        <CreateModelKeyDialog modelName={modelName} onCreated={setRevealed} />
      </div>
      {keys.isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : keys.error ? (
        <p className="text-sm text-destructive">{keys.error.message}</p>
      ) : (
        <ul className="divide-y divide-border rounded-md border border-border">
          {keys.data.map((k) => {
            const inactive = k.status === 'revoked' || k.status === 'expired'
            return (
              <li
                key={k.id}
                className="flex items-center gap-3 px-3 py-2 text-xs"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {k.name}
                    </span>
                    <code className="font-mono text-muted-foreground">
                      {k.key_prefix}
                    </code>
                    <Badge variant={statusVariant(k.status)}>{k.status}</Badge>
                  </div>
                  <p className="truncate text-muted-foreground">
                    {limitLabel(k)}, last used {when(k.last_used_at)}
                    {k.expires_at ? `, expires ${when(k.expires_at)}` : ''}
                  </p>
                </div>
                {inactive || k.status === 'rotating' ? null : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Rotate key ${k.name}`}
                    title="Rotate, old key keeps working for the grace window"
                    disabled={rotate.isPending}
                    onClick={() => {
                      rotate.mutate(
                        { id: k.id },
                        {
                          onSuccess: (res) => {
                            setRevealed(res.api_key)
                          },
                          onError: fail('Could not rotate the key.'),
                        },
                      )
                    }}
                  >
                    <ArrowsClockwiseIcon aria-hidden="true" />
                  </Button>
                )}
                {k.status === 'revoked' ? null : (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Revoke key ${k.name}`}
                    title="Revoke now"
                    disabled={revoke.isPending}
                    onClick={() => {
                      revoke.mutate(k.id, {
                        onError: fail('Could not revoke the key.'),
                      })
                    }}
                  >
                    <ProhibitIcon aria-hidden="true" />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
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
