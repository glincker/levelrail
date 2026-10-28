import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { updatesQueryOptions } from '../../queries/updates'
import { displayVersion } from '../../lib/updateBehind'

export function SidebarVersionLinkView({
  current,
  latest,
  updateAvailable,
}: {
  current: string | undefined
  latest: string | null | undefined
  updateAvailable: boolean
}) {
  const { t } = useTranslation()
  const label = current
    ? `${t('shell.account.controlPlane')} ${displayVersion(current)}`
    : t('shell.account.controlPlane')
  const hint =
    updateAvailable && latest
      ? t('shell.account.updateAvailable', { version: displayVersion(latest) })
      : null
  return (
    <Link
      to="/settings/updates"
      aria-label={hint ? `${label}. ${hint}` : label}
      title={hint ?? label}
      className="mt-1 flex items-center gap-1.5 truncate rounded-md px-2 py-0.5 text-[11px] text-sidebar-foreground/60 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:outline-none group-data-[collapsible=icon]:hidden"
    >
      <span className="truncate">{label}</span>
      {hint ? (
        <span
          data-testid="update-dot"
          aria-hidden="true"
          className="size-1.5 shrink-0 rounded-full bg-tone-warning-solid"
        />
      ) : null}
    </Link>
  )
}

export function SidebarVersionLink() {
  const data = useQuery(updatesQueryOptions()).data
  return (
    <SidebarVersionLinkView
      current={data?.current_version}
      latest={data?.latest_version}
      updateAvailable={data?.update_available === true}
    />
  )
}
