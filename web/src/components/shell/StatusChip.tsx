import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { StatusPill, type Tone } from '@/components/kit'
import { CleanUpDockerDialog } from '../CleanUpDockerDialog'
import type { AttentionItem } from '../../lib/attention'
import { attentionHref, type AttentionLevel } from '../../lib/attentionSummary'
import { SilenceMenu } from '../attention/SilenceMenu'
import { unsnoozeNow } from './snoozeStore'
import { useAttentionSummary } from './useAttentionSummary'

const LEVEL_TONE: Record<AttentionLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

function IssueRow({ issue }: { issue: AttentionItem }) {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const critical = issue.severity === 'critical'
  const Icon = critical ? WarningCircleIcon : WarningIcon
  return (
    <li className="flex items-start gap-2.5 rounded-lg px-2 py-2">
      <Icon
        aria-hidden="true"
        className={`mt-0.5 size-4 shrink-0 ${
          critical ? 'text-destructive' : 'text-amber-600 dark:text-amber-400'
        }`}
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium" title={issue.title}>
          {issue.title}
        </p>
        <p
          className="line-clamp-3 text-xs break-words text-muted-foreground"
          title={issue.detail}
        >
          {issue.detail}
        </p>
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
          {issue.id === 'disk' ? (
            <CleanUpDockerDialog key="cleanup" />
          ) : (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void navigate({ to: attentionHref(issue.target) })}
            >
              {t('shell.status.view')}
            </Button>
          )}
          <SilenceMenu
            issueKey={issue.id}
            severity={issue.severity}
            title={issue.title}
          />
        </div>
      </div>
    </li>
  )
}

export function StatusChip() {
  const { t } = useTranslation()
  const { level, count, visible, snoozed } = useAttentionSummary()
  const navigate = useNavigate()
  const label =
    level === 'ok' ? t('shell.status.ok') : t('shell.status.issues', { count })

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            aria-label={t('shell.status.triggerLabel', { label })}
            className="rounded-full outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
          />
        }
      >
        <StatusPill
          tone={LEVEL_TONE[level]}
          label={label}
          live={level === 'critical'}
          size="md"
        />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-96 max-w-[calc(100vw-1rem)]"
        glass
      >
        <PopoverHeader>
          <PopoverTitle>{t('shell.status.title')}</PopoverTitle>
        </PopoverHeader>
        {visible.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 px-2 py-6 text-center">
            <CheckCircleIcon
              aria-hidden="true"
              className="size-6 text-emerald-600 dark:text-emerald-400"
            />
            <p className="text-sm text-muted-foreground">
              {t('shell.status.healthy')}
            </p>
          </div>
        ) : (
          <ul className="flex flex-col">
            {visible.map((i) => (
              <IssueRow key={i.id} issue={i} />
            ))}
          </ul>
        )}
        {snoozed.length > 0 ? (
          <div className="mt-1 flex items-center justify-between border-t border-border px-2 pt-2 text-xs text-muted-foreground">
            <span>
              {t('shell.status.snoozedCount', { count: snoozed.length })}
            </span>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                snoozed.forEach((i) => {
                  unsnoozeNow(i.id)
                })
                void navigate({ to: '/status' })
              }}
            >
              {t('shell.status.showAgain')}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
