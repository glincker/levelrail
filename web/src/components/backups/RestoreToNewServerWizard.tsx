import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useRestoreAppToNode } from '../../queries/backupProtection'
import { useNodeListOptional } from '../../queries/nodes'
import type { AppRestoreToNodeResponse } from '../../types/backupProtection'

type Step = 'choose' | 'review' | 'progress'

/** RestoreToNewServerWizard restores an app, its volumes and databases onto another node from the latest backups. */
export function RestoreToNewServerWizard({
  apps,
  open,
  onOpenChange,
}: {
  apps: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('backups')
  const nodes = useNodeListOptional()
  const [step, setStep] = useState<Step>('choose')
  const [app, setApp] = useState('')
  const [nodeId, setNodeId] = useState('')
  const [plan, setPlan] = useState<AppRestoreToNodeResponse | null>(null)
  const [started, setStarted] = useState(false)
  const restore = useRestoreAppToNode(app)

  const nodeList = nodes.data ?? []
  const nodeName = nodeList.find((n) => n.id === nodeId)?.name ?? nodeId
  const items = plan?.items ?? []
  const restorable = items.filter((i) => i.status !== 'skipped')

  function close(next: boolean) {
    onOpenChange(next)
    if (!next) {
      setStep('choose')
      setApp('')
      setNodeId('')
      setPlan(null)
      setStarted(false)
      restore.reset()
    }
  }

  function review() {
    restore.mutate(
      { node_id: nodeId, dry_run: true },
      {
        onSuccess: (res) => {
          setPlan(res)
          setStep('review')
        },
      },
    )
  }

  function confirm() {
    restore.mutate(
      { node_id: nodeId },
      {
        onSuccess: (res) => {
          setPlan(res)
          setStarted(true)
          setStep('progress')
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('newServer.title')}</DialogTitle>
          <DialogDescription>{t('newServer.description')}</DialogDescription>
        </DialogHeader>

        {step === 'choose' ? (
          <div className="space-y-3">
            <Field>
              <FieldLabel>{t('newServer.app')}</FieldLabel>
              {apps.length === 0 ? (
                <FieldHint>{t('newServer.noApps')}</FieldHint>
              ) : (
                <Select
                  value={app || null}
                  onValueChange={(v) => {
                    setApp(v ?? '')
                  }}
                >
                  <SelectTrigger aria-label={t('newServer.app')}>
                    <SelectValue placeholder={t('newServer.appPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {apps.map((a) => (
                      <SelectItem key={a} value={a}>
                        {a}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </Field>
            <Field>
              <FieldLabel>{t('newServer.node')}</FieldLabel>
              {nodeList.length === 0 ? (
                <FieldHint>{t('newServer.noNodes')}</FieldHint>
              ) : (
                <Select
                  value={nodeId || null}
                  onValueChange={(v) => {
                    setNodeId(v ?? '')
                  }}
                >
                  <SelectTrigger aria-label={t('newServer.node')}>
                    <SelectValue placeholder={t('newServer.nodePlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {nodeList.map((n) => (
                      <SelectItem
                        key={n.id}
                        value={n.id}
                        disabled={n.status === 'offline'}
                      >
                        {n.status === 'offline'
                          ? t('newServer.nodeOffline', { name: n.name })
                          : n.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </Field>
            {restore.isError ? (
              <p role="alert" className="text-sm text-destructive">
                {t('newServer.planFailed', { error: restore.error.message })}
              </p>
            ) : null}
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-sm">
              {items.length === 0
                ? t('newServer.reviewEmpty')
                : t('newServer.review', { node: nodeName })}
            </p>
            <ul className="space-y-2">
              {items.map((i) => (
                <li
                  key={`${i.kind}:${i.source}`}
                  className="flex items-start justify-between gap-3 rounded-md border p-2 text-sm"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{i.source}</span>
                      <Badge variant="outline">
                        {t(`newServer.kind.${i.kind}`)}
                      </Badge>
                    </div>
                    {i.new_name ? (
                      <p className="text-xs text-muted-foreground">
                        {t('newServer.asName', { name: i.new_name })}
                      </p>
                    ) : null}
                    {i.reason ? (
                      <p className="text-xs text-muted-foreground">
                        {i.reason}
                      </p>
                    ) : null}
                  </div>
                  <Badge
                    variant={
                      i.status === 'failed'
                        ? 'destructive'
                        : i.status === 'skipped'
                          ? 'muted'
                          : 'info'
                    }
                  >
                    {t(`newServer.status.${i.status}`)}
                  </Badge>
                </li>
              ))}
            </ul>
            {step === 'review' && restorable.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('newServer.nothingRestorable')}
              </p>
            ) : null}
            {started ? (
              <div className="space-y-1 text-sm">
                <p className="font-medium">{t('newServer.startedTitle')}</p>
                <p className="text-muted-foreground">
                  {t('newServer.startedBody', { node: nodeName })}
                </p>
              </div>
            ) : null}
            {step === 'review' && restore.isError ? (
              <p role="alert" className="text-sm text-destructive">
                {t('newServer.startFailed', { error: restore.error.message })}
              </p>
            ) : null}
          </div>
        )}

        <DialogFooter>
          {step === 'choose' ? (
            <>
              <Button
                variant="outline"
                onClick={() => {
                  close(false)
                }}
              >
                {t('newServer.cancel')}
              </Button>
              <Button
                disabled={!app || !nodeId || restore.isPending}
                onClick={review}
              >
                {t('newServer.next')}
              </Button>
            </>
          ) : step === 'review' ? (
            <>
              <Button
                variant="outline"
                onClick={() => {
                  restore.reset()
                  setStep('choose')
                }}
              >
                {t('newServer.back')}
              </Button>
              <Button
                disabled={restorable.length === 0 || restore.isPending}
                onClick={confirm}
              >
                {t('newServer.confirm')}
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="outline"
                render={<Link to="/apps/$name" params={{ name: app }} />}
              >
                {t('newServer.openApp')}
              </Button>
              <Button
                onClick={() => {
                  close(false)
                }}
              >
                {t('newServer.done')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
