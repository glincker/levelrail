import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { formatBytes, formatDate } from '../../lib/format'
import { useNodeListOptional } from '../../queries/nodes'
import { useRestoreVolumeTo } from '../../queries/backupProtection'
import { useVolumeBackupHistory } from '../../queries/volumeBackupHistory'

const SAME_NODE = '__same__'
type Destination = 'same' | 'other'

/** RestoreWizardDialog restores a volume backup into a new volume, never over an existing one. */
export function RestoreWizardDialog({
  app,
  volume,
  open,
  onOpenChange,
}: {
  app: string
  volume: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('backups')
  const history = useVolumeBackupHistory(app, volume)
  const nodes = useNodeListOptional()
  const restore = useRestoreVolumeTo(app, volume)

  const [step, setStep] = useState(0)
  const [backupId, setBackupId] = useState('')
  const [destination, setDestination] = useState<Destination>('same')
  const [targetApp, setTargetApp] = useState('')
  const [nodeId, setNodeId] = useState(SAME_NODE)
  const [newName, setNewName] = useState('')

  const succeeded = (history.data ?? []).filter((b) => b.status === 'succeeded')
  const chosen = backupId || succeeded[0]?.id || ''
  const nodeList = nodes.data ?? []
  const nodeLabel =
    nodeId === SAME_NODE
      ? t('restoreWizard.sameNode')
      : (nodeList.find((n) => n.id === nodeId)?.name ?? nodeId)
  const trimmedApp = targetApp.trim()
  const destinationValid = destination === 'same' || trimmedApp.length > 0

  function close(next: boolean) {
    onOpenChange(next)
    if (!next) {
      setStep(0)
      setBackupId('')
      setDestination('same')
      setTargetApp('')
      setNodeId(SAME_NODE)
      setNewName('')
      restore.reset()
    }
  }

  function start() {
    restore.mutate(
      {
        backup_id: chosen,
        new_volume_name: newName.trim() || undefined,
        target_app: destination === 'other' ? trimmedApp : undefined,
        node_id: nodeId === SAME_NODE ? undefined : nodeId,
      },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('restoreWizard.started', { name: res.new_volume_name }),
            description: t('restoreWizard.startedDescription'),
            type: 'success',
          })
          close(false)
        },
        onError: (err) => {
          toast.add({
            title: t('restoreWizard.failed'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('restoreWizard.title', { app, volume })}</DialogTitle>
          <DialogDescription>
            {t('restoreWizard.description')}
          </DialogDescription>
        </DialogHeader>

        {step === 0 ? (
          succeeded.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('restoreWizard.noBackups')}
            </p>
          ) : (
            <Field>
              <FieldLabel htmlFor="restore-backup">
                {t('restoreWizard.backup')}
              </FieldLabel>
              <Select
                value={chosen || null}
                onValueChange={(v) => {
                  setBackupId(v ?? '')
                }}
              >
                <SelectTrigger id="restore-backup" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {succeeded.map((b) => (
                    <SelectItem key={b.id} value={b.id}>
                      {formatDate(b.started_at, '-')} (
                      {formatBytes(b.size_bytes)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )
        ) : null}

        {step === 1 ? (
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor="restore-destination">
                {t('restoreWizard.destination')}
              </FieldLabel>
              <Select
                value={destination}
                onValueChange={(v) => {
                  setDestination(v === 'other' ? 'other' : 'same')
                }}
              >
                <SelectTrigger id="restore-destination" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="same">
                    {t('restoreWizard.sameApp')}
                  </SelectItem>
                  <SelectItem value="other">
                    {t('restoreWizard.otherApp')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
            {destination === 'other' ? (
              <Field>
                <FieldLabel htmlFor="restore-target-app">
                  {t('restoreWizard.targetApp')}
                </FieldLabel>
                <Input
                  id="restore-target-app"
                  autoComplete="off"
                  spellCheck={false}
                  value={targetApp}
                  onChange={(e) => {
                    setTargetApp(e.target.value)
                  }}
                />
                <FieldHint>{t('restoreWizard.targetAppHint')}</FieldHint>
              </Field>
            ) : null}
            <Field>
              <FieldLabel htmlFor="restore-node">
                {t('restoreWizard.node')}
              </FieldLabel>
              <Select
                value={nodeId}
                onValueChange={(v) => {
                  setNodeId(v ?? SAME_NODE)
                }}
              >
                <SelectTrigger id="restore-node" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={SAME_NODE}>
                    {t('restoreWizard.sameNode')}
                  </SelectItem>
                  {nodeList.map((n) => (
                    <SelectItem key={n.id} value={n.id}>
                      {n.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {destination === 'same' ? (
              <Field>
                <FieldLabel htmlFor="restore-new-name">
                  {t('restoreWizard.newName')}
                </FieldLabel>
                <Input
                  id="restore-new-name"
                  autoComplete="off"
                  spellCheck={false}
                  value={newName}
                  onChange={(e) => {
                    setNewName(e.target.value)
                  }}
                />
                <FieldHint>{t('restoreWizard.newNameHint')}</FieldHint>
              </Field>
            ) : null}
          </div>
        ) : null}

        {step === 2 ? (
          <p className="text-sm text-foreground">
            {t('restoreWizard.summary', {
              backup: chosen,
              name:
                destination === 'other'
                  ? `app-${trimmedApp}-${volume}`
                  : newName.trim() || t('restoreWizard.generatedName'),
              node: nodeLabel,
            })}
          </p>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              if (step === 0) close(false)
              else setStep(step - 1)
            }}
          >
            {step === 0 ? t('restoreWizard.cancel') : t('restoreWizard.back')}
          </Button>
          {step < 2 ? (
            <Button
              type="button"
              disabled={
                (step === 0 && !chosen) || (step === 1 && !destinationValid)
              }
              onClick={() => {
                setStep(step + 1)
              }}
            >
              {t('restoreWizard.next')}
            </Button>
          ) : (
            <Button type="button" disabled={restore.isPending} onClick={start}>
              {t('restoreWizard.start')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
