import { useTranslation } from 'react-i18next'
import { parseVersion } from '@/lib/versionLabel'

export function VersionLabel({ value }: { value: string }) {
  const { t } = useTranslation('updates')
  const p = parseVersion(value)
  return (
    <span title={value} className="min-w-0 break-words text-xs">
      {p.kind === 'dev'
        ? t('selfUpgrade.version.dev', {
            count: p.commits,
            commits: p.commits,
            base: p.base,
            sha: p.sha.slice(0, 9),
          })
        : value || t('selfUpgrade.version.unknown')}
    </span>
  )
}
