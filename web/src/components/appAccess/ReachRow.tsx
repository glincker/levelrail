import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import type { ProbeTarget } from '../../queries/appExposure'
import { ProbeNote, ScopeChip, SecurityChip } from './ReachParts'
import { useCopyAddress } from './useCopyAddress'
import { resolveSecurity, type ReachAddress } from './reachModel'

interface ReachRowProps {
  addr: ReachAddress
  appName: string
  probe?: ProbeTarget
  testing: boolean
  onTest: () => void
  onAllow: () => void
}

/** One address with its scope and security chips, plus test, copy and open. */
export function ReachRow({
  addr,
  appName,
  probe,
  testing,
  onTest,
  onAllow,
}: ReachRowProps) {
  const { t } = useTranslation('appAccess')
  const { copied, copy } = useCopyAddress()
  const what = t(`id.${addr.kind}`)
  const showChange = addr.kind === 'public'
  return (
    <li className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-sm font-medium">{what}</span>
          <ScopeChip scope={addr.scope} />
          <SecurityChip security={resolveSecurity(addr, probe)} />
        </div>
        {addr.display ? (
          <code className="block break-all font-mono text-sm">
            {addr.display}
          </code>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t(`note.${addr.note}`, { address: addr.value })}
          {showChange ? (
            <>
              {' '}
              <Link
                to="/apps/$name/network"
                params={{ name: appName }}
                className="text-primary underline underline-offset-2"
              >
                {t('actions.change')}
              </Link>
            </>
          ) : null}
        </p>
        {probe ? <ProbeNote probe={probe} /> : null}
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-1">
        {addr.allowNetwork ? (
          <Button size="sm" variant="outline" onClick={onAllow}>
            <TriangleAlertGlassIcon
              className="size-5 text-tone-warning"
              aria-hidden="true"
            />
            {t('actions.allowNetwork')}
          </Button>
        ) : null}
        {addr.href ? (
          <Button size="sm" variant="ghost" disabled={testing} onClick={onTest}>
            {testing ? t('actions.testing') : t('actions.test')}
          </Button>
        ) : null}
        {addr.value ? (
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={
              copied ? t('actions.copied') : t('actions.copy', { what })
            }
            onClick={() => copy(addr.value)}
          >
            {copied ? (
              <CheckGlassIcon className="size-5" aria-hidden="true" />
            ) : (
              <CopyGlassIcon className="size-5" aria-hidden="true" />
            )}
          </Button>
        ) : null}
        {addr.href ? (
          <a
            href={addr.href}
            target="_blank"
            rel="noreferrer noopener"
            aria-label={t('actions.open', { what })}
            className={buttonVariants({ size: 'icon-sm', variant: 'ghost' })}
          >
            <ExternalLinkGlassIcon className="size-5" aria-hidden="true" />
          </a>
        ) : null}
      </div>
    </li>
  )
}
