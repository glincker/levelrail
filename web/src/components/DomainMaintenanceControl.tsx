import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon, WrenchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import type { SetDomainMaintenanceRequest } from '../queries/domainMaintenance'
import {
  useClearDomainMaintenance,
  useDomainMaintenance,
  useFleetMaintenance,
  useSetDomainMaintenance,
} from '../queries/domainMaintenance'
import type { DomainMaintenance } from '../queries/domainMaintenance'
import { SettingSummary } from './domains/SettingSummary'
import { EnforcedBadge, EnforcementNote } from './domains/EnforcementGate'
import { DomainMaintenanceForm } from './domains/DomainMaintenanceForm'
import { EnableMaintenanceDialog } from './maintenance/EnableMaintenanceDialog'
import { useDomainEnforcement } from '../hooks/useDomainEnforcement'
import { isEnforced } from '../lib/domainEnforcement'

function MaintenanceSummary({ m }: { m: DomainMaintenance }) {
  const { t, i18n } = useTranslation('domains')
  const bypass = m.bypass?.length ?? 0
  return (
    <ul className="mt-1 space-y-0.5 text-xs text-muted-foreground">
      {m.until ? (
        <li>
          {t('maintenance.ends', {
            date: new Date(m.until).toLocaleString(i18n.language),
          })}
        </li>
      ) : null}
      {m.auto_off_at && !m.until ? (
        <li>
          {t('maintenance.autoOffAt', {
            date: new Date(m.auto_off_at).toLocaleString(i18n.language),
          })}
        </li>
      ) : null}
      {!m.auto_off_at && !m.until ? (
        <li>{t('maintenance.noAutoOff')}</li>
      ) : null}
      {bypass > 0 ? (
        <li>{t('maintenance.bypassCount', { count: bypass })}</li>
      ) : null}
      {m.bypass_health_check ? <li>{t('maintenance.healthOpen')}</li> : null}
    </ul>
  )
}

// Maintenance mode for one saved domain. The "on" state is only shown when
// the ingress that visitors reach really serves the maintenance page.
export function DomainMaintenanceControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data: maintenance, isLoading } = useDomainMaintenance(appName, domain)
  const setMaintenance = useSetDomainMaintenance(appName, domain)
  const clearMaintenance = useClearDomainMaintenance(appName, domain)
  const enforcement = useDomainEnforcement(domain)
  const [open, setOpen] = useState(false)
  const [confirming, setConfirming] =
    useState<SetDomainMaintenanceRequest | null>(null)
  const { data: fleet } = useFleetMaintenance()

  if (isLoading) {
    return (
      <div
        className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const enabled = maintenance?.enabled ?? false
  const pending = setMaintenance.isPending || clearMaintenance.isPending
  const error = setMaintenance.isError
    ? setMaintenance.error.message
    : clearMaintenance.isError
      ? clearMaintenance.error.message
      : null

  return (
    <>
      <SettingSummary
        status={
          enabled ? (
            <EnforcedBadge domain={domain} control="maintenance">
              <Badge variant="warning" className="shrink-0">
                <WrenchGlassIcon className="size-4.5" />
                {t('maintenance.on')}
              </Badge>
            </EnforcedBadge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <WrenchGlassIcon className="size-4.5" />
              {t('maintenance.off')}
            </Badge>
          )
        }
        note={
          <>
            {enabled && maintenance ? (
              <MaintenanceSummary m={maintenance} />
            ) : null}
            {enabled && isEnforced(enforcement, 'maintenance') ? (
              <p className="mt-2 text-xs text-muted-foreground">
                {t('maintenance.activeHint', { domain })}
              </p>
            ) : null}
            <EnforcementNote
              domain={domain}
              control="maintenance"
              active={enabled}
            />
          </>
        }
        actions={
          enabled ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={pending}
              onClick={() => {
                clearMaintenance.mutate()
              }}
            >
              {clearMaintenance.isPending
                ? t('maintenance.ending')
                : t('maintenance.end')}
            </Button>
          ) : (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={pending}
              onClick={() => {
                setConfirming({})
              }}
            >
              {setMaintenance.isPending
                ? t('maintenance.enabling')
                : t('maintenance.enable')}
            </Button>
          )
        }
        editLabel={t('maintenance.options')}
        closeLabel={t('maintenance.hideOptions')}
        editing={open}
        onEditingChange={setOpen}
      >
        {open ? (
          <DomainMaintenanceForm
            key={maintenance?.window_id ?? String(enabled)}
            domain={domain}
            defaultMaxSeconds={fleet?.max_duration_seconds ?? 0}
            current={maintenance}
            enabled={enabled}
            pending={pending}
            onSubmit={(req) => {
              if (!enabled) {
                setConfirming(req)
                return
              }
              setMaintenance.mutate(req, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: t('maintenance.saved'), type: 'success' })
                },
              })
            }}
          />
        ) : null}
      </SettingSummary>
      <EnableMaintenanceDialog
        domain={domain}
        request={confirming}
        defaultMaxSeconds={fleet?.max_duration_seconds ?? 0}
        pending={setMaintenance.isPending}
        error={error}
        onCancel={() => {
          setConfirming(null)
        }}
        onConfirm={(req) => {
          setMaintenance.mutate(req, {
            onSuccess: () => {
              setConfirming(null)
              setOpen(false)
              toast.add({ title: t('maintenance.saved'), type: 'success' })
            },
          })
        }}
      />
      {error && confirming === null ? (
        <Alert variant="destructive" className="mt-2">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </>
  )
}
