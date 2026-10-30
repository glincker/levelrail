import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ServerGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useNodeListOptional } from '../queries/nodes'
import { useCreateNodeStorageDestination } from '../queries/storage'

const BYTES_PER_GB = 1_000_000_000

export function CreateNodeStorageDestinationDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [nodeId, setNodeId] = useState('')
  const [directory, setDirectory] = useState('')
  const [quotaGb, setQuotaGb] = useState('0')
  const [verify, setVerify] = useState(true)
  const nodes = useNodeListOptional().data ?? []
  const create = useCreateNodeStorageDestination()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setName('')
      setNodeId('')
      setDirectory('')
      setQuotaGb('0')
      setVerify(true)
      create.reset()
    }
  }

  const quota = Number(quotaGb)
  const invalid =
    name.trim() === '' || nodeId === '' || !Number.isFinite(quota) || quota < 0

  function submit() {
    create.mutate(
      {
        kind: 'agent',
        name: name.trim(),
        node_id: nodeId,
        ...(directory.trim() ? { directory: directory.trim() } : {}),
        ...(quota > 0 ? { quota_bytes: Math.round(quota * BYTES_PER_GB) } : {}),
        verify,
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: t('storageDestinations.node.connected', {
              name: created.name,
            }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        <ServerGlassIcon className="size-5" aria-hidden="true" />
        {t('storageDestinations.node.add')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('storageDestinations.node.title')}</DialogTitle>
          <DialogDescription>
            {t('storageDestinations.node.description')}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!invalid) submit()
          }}
        >
          <Field>
            <FieldLabel htmlFor="node-storage-name">
              {t('storageDestinations.node.name')}
            </FieldLabel>
            <Input
              id="node-storage-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="node-storage-node">
              {t('storageDestinations.node.node')}
            </FieldLabel>
            <Select
              value={nodeId}
              onValueChange={(value) => {
                setNodeId(value ?? '')
              }}
            >
              <SelectTrigger id="node-storage-node" className="w-full">
                <SelectValue
                  placeholder={t('storageDestinations.node.nodePlaceholder')}
                />
              </SelectTrigger>
              <SelectContent>
                {nodes.map((n) => (
                  <SelectItem key={n.id} value={n.id}>
                    {n.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {nodes.length === 0 ? (
              <FieldDescription>
                {t('storageDestinations.node.noNodes')}
              </FieldDescription>
            ) : null}
          </Field>
          <Field>
            <FieldLabel htmlFor="node-storage-dir">
              {t('storageDestinations.node.directory')}
            </FieldLabel>
            <Input
              id="node-storage-dir"
              className="font-mono"
              placeholder="backups"
              value={directory}
              onChange={(e) => {
                setDirectory(e.target.value)
              }}
            />
            <FieldDescription>
              {t('storageDestinations.node.directoryHint')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="node-storage-quota">
              {t('storageDestinations.node.quota')}
            </FieldLabel>
            <Input
              id="node-storage-quota"
              type="number"
              min={0}
              value={quotaGb}
              onChange={(e) => {
                setQuotaGb(e.target.value)
              }}
            />
            <FieldDescription>
              {t('storageDestinations.node.quotaHint')}
            </FieldDescription>
          </Field>
          <div className="flex items-center gap-2 text-sm">
            <Checkbox
              id="node-storage-verify"
              checked={verify}
              onCheckedChange={(v) => {
                setVerify(v === true)
              }}
            />
            <label htmlFor="node-storage-verify">
              {t('storageDestinations.node.verify')}
            </label>
          </div>
          {create.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="submit" disabled={invalid || create.isPending}>
              {create.isPending
                ? t('storageDestinations.node.connecting')
                : t('storageDestinations.node.connect')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
