import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { toast } from '@/components/ui/toast'
import { useApprovePlugin, type Plugin } from '../../queries/plugins'

const READ = 'read'

function Detail({ label, children }: { label: string; children: string }) {
  return (
    <div className="space-y-0.5">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-sm break-all">{children}</p>
    </div>
  )
}

export function ApprovalDialog({
  plugin,
  onClose,
}: Readonly<{ plugin: Plugin | null; onClose: () => void }>) {
  if (!plugin) return null
  return <ApprovalForm key={plugin.name} plugin={plugin} onClose={onClose} />
}

function ApprovalForm({
  plugin,
  onClose,
}: Readonly<{ plugin: Plugin; onClose: () => void }>) {
  const { t } = useTranslation('plugins')
  const approve = useApprovePlugin()
  const [selected, setSelected] = useState<string[]>([])

  const toggle = (ability: string, on: boolean) =>
    setSelected((cur) =>
      on ? [...cur, ability] : cur.filter((a) => a !== ability),
    )
  const hasRead = selected.includes(READ)

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('approve.title', { name: plugin.name })}</DialogTitle>
          <DialogDescription>
            {t('approve.description', {
              name: plugin.name,
              version: plugin.version,
              publisher: plugin.publisher,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">
              {t('approve.requested')}
            </legend>
            {plugin.requested_abilities.map((ability) => (
              <div key={ability} className="flex items-start gap-2">
                <Checkbox
                  id={`ability-${ability}`}
                  checked={selected.includes(ability)}
                  onCheckedChange={(on) => toggle(ability, on)}
                />
                <Label htmlFor={`ability-${ability}`} className="block">
                  <code className="text-xs">{ability}</code>
                  <span className="block text-sm font-normal text-muted-foreground">
                    {ability === READ
                      ? t('abilities.read')
                      : t('abilities.readSensitive')}
                  </span>
                </Label>
              </div>
            ))}
          </fieldset>
          <Detail label={t('approve.network')}>
            {plugin.outbound_hosts.length === 0
              ? t('approve.networkNone')
              : t('approve.networkHosts', {
                  hosts: plugin.outbound_hosts.join(', '),
                })}
          </Detail>
          {plugin.outbound_hosts.length > 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('approve.networkNote')}
            </p>
          ) : null}
          <Detail label={t('approve.image')}>{plugin.image}</Detail>
          <Detail label={t('approve.signedBy')}>{plugin.key_id}</Detail>
          <p className="text-sm text-muted-foreground">{t('approve.never')}</p>
          {selected.length > 0 && !hasRead ? (
            <p role="alert" className="text-sm text-destructive">
              {t('approve.needsRead')}
            </p>
          ) : null}
          {approve.error ? (
            <p role="alert" className="text-sm text-destructive">
              {approve.error.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('approve.later')}
          </Button>
          <Button
            type="button"
            disabled={!hasRead || approve.isPending}
            onClick={() =>
              approve.mutate(
                { name: plugin.name, abilities: selected },
                {
                  onSuccess: () => {
                    toast.add({
                      type: 'success',
                      title: t('approve.approved', { name: plugin.name }),
                    })
                    onClose()
                  },
                },
              )
            }
          >
            {approve.isPending ? t('approve.pending') : t('approve.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
