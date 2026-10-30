import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { StatusBadge } from '@/components/ui/status-badge'
import { toast } from '@/components/ui/toast'
import {
  useMarketplaceInstall,
  useMarketplacePreview,
  type MarketplaceEntry,
} from '../../queries/pluginMarketplace'

function Detail({ label, children }: { label: string; children: string }) {
  return (
    <div className="space-y-0.5">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-sm break-all">{children}</p>
    </div>
  )
}

export function MarketplaceInstallDialog({
  entry,
  onClose,
}: Readonly<{ entry: MarketplaceEntry | null; onClose: () => void }>) {
  if (!entry) return null
  return (
    <ConfirmSheet
      key={`${entry.source_id}/${entry.id}`}
      entry={entry}
      onClose={onClose}
    />
  )
}

function ConfirmSheet({
  entry,
  onClose,
}: Readonly<{ entry: MarketplaceEntry; onClose: () => void }>) {
  const { t } = useTranslation('plugins')
  const preview = useMarketplacePreview()
  const install = useMarketplaceInstall()
  const [trust, setTrust] = useState(false)
  const { mutate: loadPreview } = preview
  const request = { source_id: entry.source_id, id: entry.id }

  useEffect(() => {
    loadPreview({ source_id: entry.source_id, id: entry.id })
  }, [loadPreview, entry.source_id, entry.id])

  const pv = preview.data
  const needsTrust = pv ? !pv.key_trusted : false
  const upgrade = pv?.action === 'upgrade'

  const confirm = () => {
    if (!pv) return
    install.mutate(
      {
        ...request,
        version: pv.entry.version,
        digest: pv.entry.digest,
        abilities: pv.entry.abilities,
        trust_key_id: needsTrust ? pv.key_id : undefined,
      },
      {
        onSuccess: (res) => {
          toast.add({
            type: 'success',
            title: t(
              res.action === 'upgrade'
                ? 'marketplace.updated'
                : 'marketplace.installed',
              { name: entry.name },
            ),
          })
          onClose()
        },
      },
    )
  }

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t(
              upgrade
                ? 'marketplace.sheet.updateTitle'
                : 'marketplace.sheet.title',
              {
                name: entry.name,
              },
            )}
          </DialogTitle>
          <DialogDescription>
            {t('marketplace.sheet.description')}
          </DialogDescription>
        </DialogHeader>
        {preview.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t('marketplace.sheet.failed')}</AlertTitle>
            <AlertDescription>
              {preview.error.message}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-2"
                onClick={() => loadPreview(request)}
              >
                {t('marketplace.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : !pv ? (
          <div className="space-y-2" aria-busy="true">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : (
          <div className="space-y-4">
            <div>
              <p className="font-medium">
                {pv.entry.name} {pv.entry.version}
              </p>
              <p className="text-sm text-muted-foreground">
                {t('publisher', { publisher: pv.entry.publisher })}
              </p>
              {upgrade ? (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('review.upgradeFrom', { version: pv.installed_version })}
                </p>
              ) : null}
            </div>
            <Detail label={t('marketplace.sheet.source')}>
              {pv.source_name}
            </Detail>
            <Detail label={t('marketplace.sheet.key')}>
              {`${pv.key_name} (${pv.key_id})`}
            </Detail>
            <Detail label={t('marketplace.sheet.digest')}>
              {pv.entry.digest}
            </Detail>
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">
                {t('marketplace.sheet.abilities')}
              </legend>
              <ul className="space-y-1">
                {pv.entry.abilities.map((a) => (
                  <li
                    key={a}
                    className="flex flex-wrap items-start gap-2 text-sm"
                  >
                    <code className="rounded bg-muted px-1 py-0.5 text-xs">
                      {a}
                    </code>
                    <span className="text-muted-foreground">
                      {t(
                        a === 'read:sensitive'
                          ? 'abilities.readSensitive'
                          : 'abilities.read',
                      )}
                    </span>
                    {pv.added_abilities.includes(a) && upgrade ? (
                      <StatusBadge
                        tone="warning"
                        icon={TriangleAlertGlassIcon}
                        label={t('review.newAbility')}
                      />
                    ) : null}
                  </li>
                ))}
              </ul>
            </fieldset>
            <Detail label={t('approve.network')}>
              {pv.entry.outbound_hosts.length === 0
                ? t('approve.networkNone')
                : t('approve.networkHosts', {
                    hosts: pv.entry.outbound_hosts.join(', '),
                  })}
            </Detail>
            {pv.entry.outbound_hosts.length > 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('approve.networkNote')}
              </p>
            ) : null}
            <p className="text-sm text-muted-foreground">
              {t('marketplace.sheet.exactly')}
            </p>
            {needsTrust ? (
              <div className="flex items-start gap-2">
                <Checkbox
                  id="marketplace-trust"
                  checked={trust}
                  onCheckedChange={setTrust}
                />
                <Label htmlFor="marketplace-trust" className="block">
                  {t('marketplace.sheet.trust', { name: pv.key_name })}
                  <span className="block text-sm font-normal text-muted-foreground">
                    {t('marketplace.sheet.trustHint')}
                  </span>
                </Label>
              </div>
            ) : null}
            {install.error ? (
              <p role="alert" className="text-sm text-destructive">
                {install.error.message}
              </p>
            ) : null}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('marketplace.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!pv || (needsTrust && !trust) || install.isPending}
            onClick={confirm}
          >
            {install.isPending
              ? t('marketplace.sheet.pending')
              : t(
                  upgrade
                    ? 'marketplace.sheet.update'
                    : 'marketplace.sheet.install',
                )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
