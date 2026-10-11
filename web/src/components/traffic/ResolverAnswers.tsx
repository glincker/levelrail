import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  MinusCircleIcon,
  WarningCircleIcon,
  ArrowClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { DomainResolver } from '@/queries/domainCheck'
import { useAgeLabel } from './useAgeLabel'

import {
  resolverMatch,
  type ExpectedAnswers,
  type ResolverMatch,
} from '@/lib/trafficResolvers'

const MATCH_ICON: Record<ResolverMatch, Icon> = {
  matches: CheckCircleIcon,
  different: WarningCircleIcon,
  noAnswer: MinusCircleIcon,
}

const MATCH_TONE: Record<ResolverMatch, string> = {
  matches: 'text-green-700 dark:text-green-300',
  different: 'text-amber-800 dark:text-amber-300',
  noAnswer: 'text-muted-foreground',
}

export interface ResolverAnswersProps {
  resolvers: readonly DomainResolver[]
  expected: ExpectedAnswers
  checkedAt?: string
  onRecheck: () => void
  checking?: boolean
  className?: string
}

/** What each public resolver answered, with a timestamp and a live recheck. */
export function ResolverAnswers({
  resolvers,
  expected,
  checkedAt,
  onRecheck,
  checking = false,
  className,
}: Readonly<ResolverAnswersProps>) {
  const { t } = useTranslation('traffic')
  const ageLabel = useAgeLabel(checkedAt)

  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <p aria-live="polite">
          {checking
            ? t('resolvers.checking')
            : ageLabel
              ? t('resolvers.lastChecked', { age: ageLabel })
              : null}
        </p>
        <Button
          type="button"
          variant="outline"
          size="xs"
          disabled={checking}
          aria-busy={checking}
          onClick={onRecheck}
        >
          <ArrowClockwiseIcon
            className={cn(
              checking && 'animate-spin motion-reduce:animate-none',
            )}
          />
          {checking ? t('resolvers.checking') : t('resolvers.recheck')}
        </Button>
      </div>
      {resolvers.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('resolvers.empty')}</p>
      ) : (
        <table className="w-full text-left text-sm">
          <caption className="sr-only">{t('resolvers.caption')}</caption>
          <thead>
            <tr className="text-xs text-muted-foreground">
              <th scope="col" className="py-1 pr-3 font-medium">
                {t('resolvers.resolver')}
              </th>
              <th scope="col" className="py-1 pr-3 font-medium">
                {t('resolvers.answer')}
              </th>
              <th scope="col" className="py-1 font-medium">
                {t('resolvers.match')}
              </th>
            </tr>
          </thead>
          <tbody>
            {resolvers.map((r) => {
              const match = resolverMatch(r, expected)
              const MatchIcon = MATCH_ICON[match]
              return (
                <tr key={r.name} className="border-t border-border">
                  <th scope="row" className="py-1.5 pr-3 font-medium">
                    {r.name}
                  </th>
                  <td className="py-1.5 pr-3 font-mono text-[13px] break-all">
                    {r.addresses && r.addresses.length > 0
                      ? r.addresses.join(', ')
                      : (r.error ?? t('resolvers.noAnswer'))}
                  </td>
                  <td className={cn('py-1.5', MATCH_TONE[match])}>
                    <span className="inline-flex items-center gap-1">
                      <MatchIcon aria-hidden="true" className="size-4" />
                      {t(`resolvers.${match}`)}
                    </span>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
    </div>
  )
}
