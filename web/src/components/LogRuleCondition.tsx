import { useTranslation } from 'react-i18next'
import type { AlertRule } from '../types/alerts'

// One-line description of a log_pattern or log_level_rate rule's condition.
export function LogRuleCondition({ rule }: { rule: AlertRule }) {
  const { t } = useTranslation('observability')
  const cfg = rule.log_rule
  if (!cfg) {
    return null
  }
  const minutes = Math.max(1, Math.round((cfg.window_seconds ?? 300) / 60))
  if (rule.kind === 'log_level_rate') {
    return (
      <>
        {t('explorer.conditionRate', {
          rate: cfg.per_minute ?? 0,
          level: cfg.level ?? 'error',
          minutes,
        })}
      </>
    )
  }
  const what =
    cfg.regex || cfg.query || (cfg.fields ?? []).join(' ') || cfg.level
  return (
    <>
      {t('explorer.conditionPattern', {
        count: cfg.count ?? 0,
        what: what ?? '',
        minutes,
      })}
    </>
  )
}
