import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { StackIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { isFeatureVisible } from '../../lib/experimental'
import {
  activateEnvironmentScope,
  setEnvironmentScope,
  useEnvironmentScope,
} from '../../lib/environmentScope'
import { useGlobalEnvironments } from '../../queries/globalEnvironments'

const ALL = '__all__'

// Dashboard wide environment filter. Renders nothing while the
// global-environments feature is off.
export function EnvironmentSwitcher() {
  const { t } = useTranslation('environments')
  const enabled = isFeatureVisible(
    'global-environments',
    useExperimentalFeatures(),
  )
  const scope = useEnvironmentScope()
  const { data: environments } = useGlobalEnvironments(enabled)

  useEffect(() => {
    activateEnvironmentScope(enabled)
  }, [enabled])

  if (!enabled || !environments || environments.length === 0) {
    return null
  }
  const selectable = environments.filter((e) => e.kind !== 'preview')
  return (
    <Select
      value={scope === '' ? ALL : scope}
      onValueChange={(next) => {
        setEnvironmentScope(!next || next === ALL ? '' : next)
      }}
    >
      <SelectTrigger
        size="sm"
        className="w-44"
        aria-label={t('switcher.aria')}
        id="environment-switcher"
      >
        <StackIcon aria-hidden="true" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{t('switcher.all')}</SelectItem>
        {selectable.map((e) => (
          <SelectItem key={e.id} value={e.id}>
            {e.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
