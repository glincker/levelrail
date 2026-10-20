import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useMarkNodeLost,
  useNodeFailure,
  useResolveNodeFailure,
  useSetNodeFailurePolicy,
} from '../queries/nodeFailure'
import type { NodeFailureAction, NodeFailureStatus } from '../types/nodeFailure'

const MODES = ['off', 'notify', 'auto'] as const

function ConsequenceRow({
  action,
  canAct,
  nodeId,
}: {
  action: NodeFailureAction
  canAct: boolean
  nodeId: string
}) {
  const { t } = useTranslation('nodes')
  const resolve = useResolveNodeFailure(nodeId)
  const open =
    action.class === 'needs_decision' &&
    action.status !== 'moved' &&
    action.status !== 'fenced'
  const run = (kind: 'wait' | 'restore') => {
    resolve.mutate(
      { kind: action.kind, name: action.name, action: kind },
      {
        onSuccess: () => {
          toast.add({ title: t(`failure.resolved.${kind}`), type: 'success' })
        },
        onError: (e) => {
          toast.add({ title: e.message, type: 'error' })
        },
      },
    )
  }
  return (
    <li className="space-y-2 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium text-foreground">
          {action.name}{' '}
          <span className="text-xs font-normal text-muted-foreground">
            {t(`failure.kind.${action.kind}`)}
          </span>
        </p>
        <Badge variant={open ? 'destructive' : 'muted'}>
          {t(`failure.status.${action.status}`)}
        </Badge>
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`failure.class.${action.class}`)}
        {action.target
          ? `, ${t('failure.target', { node: action.target })}`
          : ''}
      </p>
      {action.reason ? (
        <p className="text-xs text-muted-foreground">{action.reason}</p>
      ) : null}
      {open && canAct ? (
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={resolve.isPending}
            onClick={() => {
              run('wait')
            }}
          >
            {t('failure.wait')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={resolve.isPending}
            onClick={() => {
              run('restore')
            }}
          >
            {t('failure.restore')}
          </Button>
        </div>
      ) : null}
      {open && canAct ? (
        <p className="text-xs text-muted-foreground">
          {t('failure.restoreHint')}
        </p>
      ) : null}
    </li>
  )
}

function PolicyForm({
  nodeId,
  status,
}: {
  nodeId: string
  status: NodeFailureStatus
}) {
  const { t } = useTranslation('nodes')
  const save = useSetNodeFailurePolicy(nodeId)
  const [mode, setMode] = useState(status.policy.mode)
  const [grace, setGrace] = useState(String(status.policy.grace_seconds))
  const graceNum = Number(grace)
  const valid = Number.isInteger(graceNum) && (graceNum === 0 || graceNum >= 30)
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field>
        <FieldLabel htmlFor="failure-mode">{t('failure.mode')}</FieldLabel>
        <Select
          value={mode || 'default'}
          onValueChange={(v) => {
            setMode(v === 'default' || v === null ? '' : v)
          }}
        >
          <SelectTrigger id="failure-mode" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="default">
              {t('failure.modeDefault', { mode: status.mode })}
            </SelectItem>
            {MODES.map((m) => (
              <SelectItem key={m} value={m}>
                {t(`failure.modes.${m}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field>
        <FieldLabel htmlFor="failure-grace">{t('failure.grace')}</FieldLabel>
        <Input
          id="failure-grace"
          inputMode="numeric"
          value={grace}
          onChange={(e) => {
            setGrace(e.target.value)
          }}
          aria-invalid={!valid}
        />
        <p className="text-xs text-muted-foreground">
          {t('failure.graceHint')}
        </p>
      </Field>
      <div className="sm:col-span-2">
        <Button
          type="button"
          size="sm"
          disabled={!valid || save.isPending}
          onClick={() => {
            save.mutate(
              { mode, grace_seconds: graceNum },
              {
                onSuccess: () => {
                  toast.add({ title: t('failure.saved'), type: 'success' })
                },
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              },
            )
          }}
        >
          {t('failure.save')}
        </Button>
      </div>
    </div>
  )
}

// Failure state, grace timer, per-app consequences and settings of one node.
export function NodeFailureCard({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const failure = useNodeFailure(nodeId)
  const lost = useMarkNodeLost(nodeId)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const status = failure.data
  const down =
    status !== undefined &&
    (status.phase === 'unreachable' || status.phase === 'lost')

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('failure.title')}</CardTitle>
        <CardDescription>{t('failure.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {failure.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : failure.isError ? (
          <div className="space-y-2 text-sm">
            <p>
              {t('failure.loadFailed')} {failure.error.message}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void failure.refetch()}
            >
              {t('failure.retry')}
            </Button>
          </div>
        ) : status ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={down ? 'destructive' : 'muted'}>
                {t(
                  `failure.phase.${status.phase === '' ? 'healthy' : status.phase}`,
                )}
              </Badge>
              {status.phase === 'grace' ? (
                <span className="text-sm text-muted-foreground">
                  {t('failure.graceTimer', {
                    silent: status.silent_seconds,
                    left: status.grace_remaining_seconds,
                  })}
                </span>
              ) : null}
              {status.reason ? (
                <span className="text-sm text-muted-foreground">
                  {status.reason}
                </span>
              ) : null}
            </div>
            {status.consequences.length > 0 ? (
              <ul className="divide-y divide-border">
                {status.consequences.map((a) => (
                  <ConsequenceRow
                    key={`${a.kind}/${a.name}`}
                    action={a}
                    canAct={down}
                    nodeId={nodeId}
                  />
                ))}
              </ul>
            ) : status.phase === '' ? (
              <p className="text-sm text-muted-foreground">
                {t('failure.healthy')}
              </p>
            ) : null}
            {status.phase !== '' &&
            status.phase !== 'lost' &&
            status.phase !== 'recovering' ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setConfirmOpen(true)
                }}
              >
                {t('failure.markLost')}
              </Button>
            ) : null}
            <PolicyForm
              key={`${status.policy.mode}-${status.policy.grace_seconds}`}
              nodeId={nodeId}
              status={status}
            />
          </>
        ) : null}
      </CardContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(next) => {
          setConfirmOpen(next)
          if (!next) lost.reset()
        }}
        title={t('failure.lostTitle')}
        description={t('failure.lostDescription')}
        consequences={(status?.consequences ?? []).map((a) =>
          a.class === 'reschedule'
            ? t('failure.planMove', { name: a.name, node: a.target ?? '?' })
            : t('failure.planDecide', { name: a.name }),
        )}
        confirmLabel={t('failure.markLost')}
        pendingLabel={t('failure.saving')}
        tone="destructive"
        pending={lost.isPending}
        error={lost.isError ? lost.error.message : null}
        onConfirm={() => {
          lost.mutate(
            { dry_run: false },
            {
              onSuccess: () => {
                setConfirmOpen(false)
                toast.add({ title: t('failure.lostDone'), type: 'success' })
              },
            },
          )
        }}
      />
    </Card>
  )
}
