import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { iamSuggestionsQueryOptions } from '../../queries/iamBuilder'
import type { IamTokenSuggestion } from '../../queries/iamBuilder'
import { useRevokeToken } from '../../queries/tokens'

/** SuggestionsSection lists tokens holding abilities the audit log never saw them use, with a revoke fix for idle ones. */
export function SuggestionsSection({
  onOpenPrincipal,
}: {
  onOpenPrincipal: (key: string) => void
}) {
  const { t } = useTranslation('iam')
  const q = useQuery(iamSuggestionsQueryOptions())
  const revoke = useRevokeToken()
  const [target, setTarget] = useState<IamTokenSuggestion | null>(null)
  const [error, setError] = useState<string | null>(null)

  if (!q.data) return null
  const items = q.data.tokens.filter((s) => s.status !== 'ok')

  async function confirmRevoke() {
    if (!target) return
    setError(null)
    try {
      await revoke.mutateAsync(target.token_id)
      toast.add({
        title: t('suggestions.revoked', { name: target.name }),
        type: 'success',
      })
      setTarget(null)
      void q.refetch()
    } catch (e) {
      setError(e instanceof Error ? e.message : t('errors.generic'))
    }
  }

  return (
    <section className="space-y-3 rounded-xl border border-border bg-card p-4">
      <header>
        <h2 className="text-sm font-medium text-foreground">
          {t('suggestions.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('suggestions.description', { days: q.data.window_days })}
        </p>
      </header>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('suggestions.empty')}
        </p>
      ) : (
        <ul className="space-y-2">
          {items.map((s) => (
            <li
              key={s.token_id}
              className="flex flex-wrap items-center gap-3 rounded-lg border border-border p-3"
            >
              <div className="min-w-0 flex-1 space-y-1">
                <p className="flex items-center gap-2 text-sm font-medium text-foreground">
                  <span className="truncate">{s.name}</span>
                  <Badge
                    variant={s.risk === 'high' ? 'destructive' : 'warning'}
                  >
                    {t(`suggestions.status.${s.status}`)}
                  </Badge>
                </p>
                <p className="text-sm text-muted-foreground">{s.reason}</p>
                <p className="text-xs break-words text-muted-foreground">
                  {t('suggestions.granted')}: {s.granted.join(', ')}
                  {s.status === 'narrow'
                    ? ` | ${t('suggestions.suggested')}: ${s.suggested_abilities.join(', ')}`
                    : ''}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => onOpenPrincipal(`token:${s.token_id}`)}
              >
                {t('analyzer.openPrincipal')}
                <ArrowRightGlassIcon size="1.4em" />
              </Button>
              {s.status === 'idle' ? (
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  onClick={() => setTarget(s)}
                >
                  {t('suggestions.revoke')}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs text-muted-foreground">{t('suggestions.note')}</p>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(o) => {
          if (!o) setTarget(null)
        }}
        title={t('suggestions.confirmTitle', { name: target?.name ?? '' })}
        description={t('suggestions.confirmDescription')}
        confirmLabel={t('suggestions.revoke')}
        pendingLabel={t('suggestions.pending')}
        tone="destructive"
        pending={revoke.isPending}
        error={error}
        onConfirm={() => void confirmRevoke()}
      />
    </section>
  )
}
