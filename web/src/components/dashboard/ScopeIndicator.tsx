import { useTranslation } from 'react-i18next'
import { FunnelGlassIcon, GlobeGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { useProjectScope } from '../../lib/projectScope'
import { clearScope, useScopeName } from './useScopeName'

/** ScopeIndicator names the active Project / Environment scope with a one-click clear; it renders nothing when no scope is picked. */
export function ScopeIndicator() {
  const { t } = useTranslation('dashboard')
  const name = useScopeName()
  if (name === '') return null
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-muted py-0.5 pr-0.5 pl-2 text-xs text-foreground">
      <FunnelGlassIcon aria-hidden="true" className="size-3.5" />
      <span className="max-w-48 truncate">{t('scope.label', { name })}</span>
      <button
        type="button"
        aria-label={t('scope.clear')}
        onClick={clearScope}
        className="inline-flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
      >
        <XGlassIcon aria-hidden="true" className="size-3.5" />
      </button>
    </span>
  )
}

/** InstanceWideLabel marks data that ignores the scope picker; it renders nothing while no scope is picked. */
export function InstanceWideLabel() {
  const { t } = useTranslation('dashboard')
  const environment = useEnvironmentScope()
  const project = useProjectScope()
  if (environment === '' && project === '') return null
  return (
    <span
      title={t('scope.instanceWideHint')}
      className="inline-flex items-center gap-1 text-xs text-muted-foreground"
    >
      <GlobeGlassIcon aria-hidden="true" className="size-3.5" />
      {t('scope.instanceWide')}
      <span className="sr-only">{t('scope.instanceWideHint')}</span>
    </span>
  )
}
