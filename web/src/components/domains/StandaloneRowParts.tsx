import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  EllipsisGlassIcon,
  FileSearchGlassIcon,
  PencilGlassIcon,
  RefreshCwGlassIcon,
  StethoscopeGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type { Domain } from '../../queries/domains'
import {
  useDeleteStandaloneDomain,
  useStandaloneDomains,
} from '../../queries/standaloneDomains'
import { StandaloneDomainForm } from './StandaloneDomainForm'

// StandaloneTargetCell fills the App column for a domain not linked to an app.
export function StandaloneTargetCell({ domain }: { domain: Domain }) {
  const { t } = useTranslation('domains')
  return (
    <span
      className="flex min-w-0 items-center gap-1.5"
      title={t('standalone.unlinkedHint')}
    >
      <Badge variant="outline">{t('standalone.unlinked')}</Badge>
      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {domain.target}
      </span>
    </span>
  )
}

export function EditStandaloneDialog({
  domain,
  open,
  onOpenChange,
}: {
  domain: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('domains')
  const { data } = useStandaloneDomains()
  const current = data?.find((d) => d.domain === domain)
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('standalone.editTitle', { domain })}</DialogTitle>
          <DialogDescription>{t('standalone.description')}</DialogDescription>
        </DialogHeader>
        {open && current ? (
          <StandaloneDomainForm
            initial={current}
            onSaved={() => {
              toast.add({
                title: t('standalone.updated', { domain }),
                type: 'success',
              })
              onOpenChange(false)
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

export function StandaloneRowActions({
  domain,
  onShowRecord,
  onRecheck,
  onDiagnose,
}: {
  domain: Domain
  onShowRecord: () => void
  onRecheck: () => void
  onDiagnose: () => void
}) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  const [editing, setEditing] = useState(false)
  const [removing, setRemoving] = useState(false)
  const remove = useDeleteStandaloneDomain()
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="justify-self-end"
              aria-label={t('page.row.actions', { domain: domain.domain })}
            />
          }
        >
          <EllipsisGlassIcon size="1.4em" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            onClick={() => {
              setEditing(true)
            }}
          >
            <PencilGlassIcon size="1.4em" />
            {t('standalone.edit')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onShowRecord}>
            <FileSearchGlassIcon size="1.4em" />
            {t('page.row.dnsRecord')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onDiagnose}>
            <StethoscopeGlassIcon size="1.4em" />
            {t('page.row.diagnose')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onRecheck}>
            <RefreshCwGlassIcon size="1.4em" />
            {t('page.row.recheck')}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => {
              copy(domain.domain)
            }}
          >
            {copied ? (
              <CheckGlassIcon size="1.4em" />
            ) : (
              <CopyGlassIcon size="1.4em" />
            )}
            {copied ? t('page.row.copied') : t('page.row.copy')}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => {
              setRemoving(true)
            }}
          >
            <TrashGlassIcon size="1.4em" />
            {t('standalone.remove')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <EditStandaloneDialog
        domain={domain.domain}
        open={editing}
        onOpenChange={setEditing}
      />
      <ConfirmDialog
        open={removing}
        onOpenChange={setRemoving}
        tone="destructive"
        title={t('standalone.removeTitle', { domain: domain.domain })}
        description={t('standalone.removeBody', {
          target: domain.target ?? '',
        })}
        confirmLabel={t('standalone.removeConfirm')}
        pendingLabel={t('standalone.removing')}
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() => {
          remove.mutate(domain.domain, {
            onSuccess: () => {
              setRemoving(false)
              toast.add({
                title: t('standalone.removed', { domain: domain.domain }),
                type: 'success',
              })
            },
          })
        }}
      />
    </>
  )
}
