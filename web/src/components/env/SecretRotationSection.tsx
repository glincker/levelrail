import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { useSecretKeys } from '../../queries/secrets'
import {
  useAppRotationPolicies,
  useDeleteRotationPolicy,
  useRotateNow,
  useSetRotationPolicy,
  type RotationPolicy,
  type RotationStatus,
} from '../../queries/secretRotation'
import { formatDate } from '../../lib/format'
import { RotationPolicyDialog } from './RotationPolicyDialog'

const PREVIOUS_SUFFIX = '_PREVIOUS'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'muted'

const STATUS_VARIANT: Record<RotationStatus, BadgeVariant> = {
  ok: 'success',
  due_soon: 'warning',
  overdue: 'destructive',
  unknown: 'muted',
}

export function RotationStatusBadge({ policy }: { policy: RotationPolicy }) {
  const { t } = useTranslation('secretRotation')
  if (policy.state === 'restart_failed' || policy.state === 'failed') {
    return <Badge variant="destructive">{t(`state.${policy.state}`)}</Badge>
  }
  if (policy.state === 'restart_pending') {
    return <Badge variant="warning">{t('state.restart_pending')}</Badge>
  }
  return (
    <Badge variant={STATUS_VARIANT[policy.status]}>
      {t(`status.${policy.status}`)}
    </Badge>
  )
}

export function SecretRotationSection({ appName }: { appName: string }) {
  const { t } = useTranslation('secretRotation')
  const keys = useSecretKeys(appName)
  const policies = useAppRotationPolicies(appName)
  const save = useSetRotationPolicy(appName)
  const remove = useDeleteRotationPolicy(appName)
  const rotate = useRotateNow(appName)
  const [editing, setEditing] = useState<string | null>(null)
  const [rotating, setRotating] = useState<string | null>(null)

  const byKey = useMemo(
    () => new Map((policies.data ?? []).map((p) => [p.key, p])),
    [policies.data],
  )
  const secretKeys = (keys.data ?? [])
    .map((k) => k.key)
    .filter((k) => !k.endsWith(PREVIOUS_SUFFIX))

  const rotateMessage = (key: string) =>
    toast.add({
      title: t('toast.rotated', { key, app: appName }),
      type: 'success',
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {secretKeys.length === 0 ? (
          <EmptyState
            icon={<ArrowsClockwiseIcon />}
            title={t('empty.title')}
            description={t('empty.description')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {secretKeys.map((key) => {
              const policy = byKey.get(key)
              return (
                <li
                  key={key}
                  className="flex flex-wrap items-center justify-between gap-3 px-3 py-2"
                >
                  <div className="min-w-0 space-y-0.5">
                    <p className="truncate font-mono text-sm">{key}</p>
                    <p className="text-xs text-muted-foreground">
                      {policy
                        ? `${t('policySummary', { days: policy.interval_days })} · ${
                            policy.auto_rotate
                              ? t('autoSummary', { hours: policy.grace_hours })
                              : t('manualSummary')
                          } · ${t('columns.nextDue')}: ${formatDate(policy.next_due_at, t('unknownDate'))}`
                        : t('noPolicy')}
                    </p>
                    {policy?.last_error ? (
                      <p role="alert" className="text-xs text-destructive">
                        {policy.last_error}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {policy ? <RotationStatusBadge policy={policy} /> : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setEditing(key)
                      }}
                    >
                      {policy
                        ? t('actions.editPolicy')
                        : t('actions.setPolicy')}
                    </Button>
                    {policy?.auto_rotate ? (
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => {
                          setRotating(key)
                        }}
                      >
                        {policy.state === 'restart_failed'
                          ? t('actions.retry')
                          : t('actions.rotateNow')}
                      </Button>
                    ) : null}
                    {policy ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={remove.isPending}
                        onClick={() => {
                          remove.mutate(key, {
                            onSuccess: () =>
                              toast.add({
                                title: t('toast.removed', { key }),
                                type: 'success',
                              }),
                            onError: (e) =>
                              toast.add({ title: e.message, type: 'error' }),
                          })
                        }}
                      >
                        {t('actions.remove')}
                      </Button>
                    ) : null}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>

      {editing ? (
        <RotationPolicyDialog
          key={editing}
          secretKey={editing}
          policy={byKey.get(editing)}
          pending={save.isPending}
          error={save.error ? save.error.message : null}
          onClose={() => {
            save.reset()
            setEditing(null)
          }}
          onSave={(input) => {
            const key = editing
            save.mutate(
              { key, ...input },
              {
                onSuccess: () => {
                  setEditing(null)
                  toast.add({
                    title: t('toast.saved', { key }),
                    type: 'success',
                  })
                },
              },
            )
          }}
        />
      ) : null}

      <ConfirmDialog
        open={rotating !== null}
        onOpenChange={(open) => {
          if (!open) {
            rotate.reset()
            setRotating(null)
          }
        }}
        title={t('confirm.title', { key: rotating ?? '' })}
        description={t('confirm.description')}
        consequences={[
          t('confirm.consequenceRestart'),
          t('confirm.consequenceGrace', { key: rotating ?? '' }),
        ]}
        confirmLabel={t('confirm.confirm')}
        pendingLabel={t('confirm.pending')}
        pending={rotate.isPending}
        error={rotate.error ? rotate.error.message : null}
        onConfirm={() => {
          if (!rotating) return
          const key = rotating
          rotate.mutate(key, {
            onSuccess: () => {
              setRotating(null)
              rotateMessage(key)
            },
          })
        }}
      />
    </Card>
  )
}
