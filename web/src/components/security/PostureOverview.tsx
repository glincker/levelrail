import { useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  QuestionIcon,
  ShieldWarningIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBrand } from '../../hooks/useBrand'
import { cliCommand } from '../../lib/cliCommand'
import {
  type PostureItem,
  type SecurityPolicyUpdate,
  type SecurityPosture,
  useUpdateSecurityPolicy,
} from '../../queries/securityCenter'
import { useUpdateCodeLoginSettings } from '../../queries/signIn'
import { postureItemKey } from './postureCopy'

const SEVERITY_VARIANT = {
  critical: 'destructive',
  high: 'destructive',
  medium: 'warning',
  low: 'muted',
} as const

function scoreTone(score: number): string {
  if (score >= 90) return 'text-green-700 dark:text-green-400'
  if (score >= 50) return 'text-amber-700 dark:text-amber-400'
  return 'text-destructive'
}

function policyBody(params: Record<string, string>): SecurityPolicyUpdate {
  const body: SecurityPolicyUpdate = {}
  if (
    params.approval_scope === 'all_methods' ||
    params.approval_scope === 'password_only'
  ) {
    body.approval_scope = params.approval_scope
  }
  for (const key of [
    'max_token_lifetime_days',
    'warn_unused_days',
    'disable_unused_days',
  ] as const) {
    const n = Number.parseInt(params[key] ?? '', 10)
    if (Number.isFinite(n)) body[key] = n
  }
  return body
}

export function PostureScoreCard({ posture }: { posture: SecurityPosture }) {
  const { t } = useTranslation('security')
  const c = posture.counts
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-6 py-6">
        <div className="text-center">
          <p className="text-xs text-muted-foreground">{t('score.label')}</p>
          <p
            className={`font-mono text-5xl font-semibold ${scoreTone(posture.score)}`}
            data-testid="posture-score"
          >
            {posture.score}
          </p>
          <p className="text-sm font-medium">
            {t('score.grade', { grade: posture.grade })}
          </p>
        </div>
        <div className="min-w-0 flex-1 space-y-1 text-sm">
          <p>
            {t('score.failing', {
              critical: c.critical,
              high: c.high,
              medium: c.medium,
              low: c.low,
            })}
          </p>
          <p className="text-muted-foreground">
            {t('score.passing', { count: c.passing })}
            {c.unknown > 0
              ? `, ${t('score.unknown', { count: c.unknown })}`
              : ''}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('score.generated', {
              time: new Date(posture.generated_at).toLocaleTimeString(),
            })}
          </p>
          {posture.full ? null : (
            <p className="text-xs text-muted-foreground">
              {t('score.summaryOnly')}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function StatusIcon({ item }: { item: PostureItem }) {
  if (item.status === 'pass') {
    return (
      <CheckCircleIcon
        className="size-5 shrink-0 text-green-600"
        aria-hidden="true"
      />
    )
  }
  if (item.status === 'unknown') {
    return (
      <QuestionIcon
        className="size-5 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
    )
  }
  return item.severity === 'critical' || item.severity === 'high' ? (
    <ShieldWarningIcon
      className="size-5 shrink-0 text-destructive"
      aria-hidden="true"
    />
  ) : (
    <WarningCircleIcon
      className="size-5 shrink-0 text-amber-600"
      aria-hidden="true"
    />
  )
}

function FixButton({
  item,
  onReviewSessions,
}: {
  item: PostureItem
  onReviewSessions: () => void
}) {
  const { t } = useTranslation('security')
  const router = useRouter()
  const setPolicy = useUpdateSecurityPolicy()
  const codeLogin = useUpdateCodeLoginSettings()
  const fix = item.fix
  if (!fix || item.status !== 'fail') return null
  const done = {
    onSuccess: () =>
      toast.add({ title: t('checklist.applied'), type: 'success' }),
    onError: (e: Error) =>
      toast.add({
        title: e.message || t('checklist.applyError'),
        type: 'error',
      }),
  }
  const action = fix.kind === 'action' ? fix.action : undefined
  if (action === 'set_policy' || action === 'disable_code_login_admins') {
    return (
      <Button
        type="button"
        size="sm"
        disabled={setPolicy.isPending || codeLogin.isPending}
        onClick={() => {
          if (action === 'set_policy') {
            setPolicy.mutate(policyBody(fix.params ?? {}), done)
          } else {
            codeLogin.mutate({ admins: false }, done)
          }
        }}
      >
        {t(`actions.${action}`)}
      </Button>
    )
  }
  if (action === 'review_sessions') {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onReviewSessions}
      >
        {t('actions.review_sessions')}
      </Button>
    )
  }
  if (!fix.link) return null
  const link = fix.link
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={() => {
        router.history.push(link)
      }}
    >
      {t('checklist.open')}
    </Button>
  )
}

function ChecklistRow({
  item,
  onReviewSessions,
}: {
  item: PostureItem
  onReviewSessions: () => void
}) {
  const { t } = useTranslation('security')
  const brand = useBrand()
  const key = postureItemKey(item.id)
  const title = key
    ? item.status === 'pass'
      ? t(`items.${key}.pass`, { count: item.count })
      : t(`items.${key}.title`, { detail: item.detail ?? '' })
    : item.id
  return (
    <li
      className="flex flex-wrap items-start gap-3 py-3"
      data-testid={`posture-${item.id}`}
    >
      <StatusIcon item={item} />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium">{title}</p>
          <Badge variant={SEVERITY_VARIANT[item.severity]}>
            {t(`severity.${item.severity}`)}
          </Badge>
          {item.status === 'fail' && item.count > 0 ? (
            <Badge variant="outline">
              {t('checklist.count', { count: item.count })}
            </Badge>
          ) : null}
          {item.status === 'unknown' ? (
            <Badge variant="muted">{t('status.unknown')}</Badge>
          ) : null}
        </div>
        {item.status === 'pass' || !key ? null : (
          <p className="text-sm text-muted-foreground">
            {t(`items.${key}.why`)}
          </p>
        )}
        {item.status === 'fail' && item.subjects && item.subjects.length > 0 ? (
          <p className="truncate text-xs text-muted-foreground">
            {item.subjects.join(', ')}
          </p>
        ) : null}
        {item.status === 'fail' && item.fix?.cli ? (
          <p className="font-mono text-xs text-muted-foreground">
            {t('checklist.cli')}: {cliCommand(brand.BinaryName, item.fix.cli)}
          </p>
        ) : null}
      </div>
      <FixButton item={item} onReviewSessions={onReviewSessions} />
    </li>
  )
}

export function PostureChecklist({
  title,
  items,
  onReviewSessions,
}: {
  title: string
  items: PostureItem[]
  onReviewSessions: () => void
}) {
  const { t } = useTranslation('security')
  const [showPassing, setShowPassing] = useState(false)
  const open = items.filter((i) => i.status !== 'pass')
  const passing = items.filter((i) => i.status === 'pass')
  const shown = showPassing ? [...open, ...passing] : open
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {items.length === 0 ? (
          <CardDescription>{t('checklist.empty')}</CardDescription>
        ) : null}
      </CardHeader>
      <CardContent>
        <ul className="divide-y divide-border">
          {shown.map((item) => (
            <ChecklistRow
              key={item.id}
              item={item}
              onReviewSessions={onReviewSessions}
            />
          ))}
        </ul>
        {passing.length > 0 ? (
          <Button
            type="button"
            variant="link"
            size="sm"
            onClick={() => {
              setShowPassing((v) => !v)
            }}
          >
            {showPassing
              ? t('checklist.hidePassing')
              : t('checklist.showPassing')}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
