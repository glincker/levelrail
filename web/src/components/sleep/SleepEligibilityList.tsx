import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { SleepFix, SleepIssue } from '../../queries/appSleep'

type Kind = 'blocker' | 'warning'

function Row({
  kind,
  issue,
  pendingCode,
  onFix,
}: {
  kind: Kind
  issue: SleepIssue
  pendingCode: string | null
  onFix: (fix: SleepFix) => void
}) {
  const { t } = useTranslation('deploys')
  const Icon = kind === 'blocker' ? BanGlassIcon : TriangleAlertGlassIcon
  const { title, detail } = issue
  return (
    <li className="flex flex-wrap items-start gap-3 py-2">
      <Icon
        aria-hidden="true"
        className={`mt-0.5 size-4 shrink-0 ${kind === 'blocker' ? 'text-tone-danger' : 'text-tone-warning'}`}
      />
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="text-sm font-medium text-foreground">
          <span className="sr-only">
            {t(
              kind === 'blocker'
                ? 'appSleep.blockedPrefix'
                : 'appSleep.warnPrefix',
            )}
          </span>
          {title}
        </p>
        {detail ? (
          <p className="text-xs text-muted-foreground">
            {kind === 'warning' ? (
              <span className="font-medium">
                {t('appSleep.whatThisMeans')}{' '}
              </span>
            ) : null}
            {detail}
          </p>
        ) : null}
      </div>
      {issue.fix ? (
        <Button
          type="button"
          size="sm"
          variant={kind === 'blocker' ? 'default' : 'outline'}
          disabled={pendingCode !== null}
          onClick={() => onFix(issue.fix as SleepFix)}
        >
          {issue.fix.label}
        </Button>
      ) : null}
    </li>
  )
}

/** SleepEligibilityList is the pre-click checklist of what blocks sleep and what only deserves a warning. */
export function SleepEligibilityList({
  blockers,
  warnings,
  pendingCode,
  onFix,
}: {
  blockers: SleepIssue[]
  warnings: SleepIssue[]
  pendingCode: string | null
  onFix: (fix: SleepFix, code: string) => void
}) {
  const { t } = useTranslation('deploys')
  return (
    <ul
      className="divide-y divide-border rounded-md border border-border px-3"
      aria-label={t('appSleep.checklistLabel')}
    >
      {blockers.map((b) => (
        <Row
          key={b.code}
          kind="blocker"
          issue={b}
          pendingCode={pendingCode}
          onFix={(fix) => onFix(fix, b.code)}
        />
      ))}
      {warnings.map((w) => (
        <Row
          key={w.code}
          kind="warning"
          issue={w}
          pendingCode={pendingCode}
          onFix={(fix) => onFix(fix, w.code)}
        />
      ))}
      {blockers.length === 0 ? (
        <li className="flex items-center gap-3 py-2">
          <CircleCheckGlassIcon
            aria-hidden="true"
            className="size-4.5 shrink-0 text-tone-success"
          />
          <p className="text-sm text-foreground">{t('appSleep.allClear')}</p>
        </li>
      ) : null}
    </ul>
  )
}
