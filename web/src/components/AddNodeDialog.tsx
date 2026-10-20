import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftIcon,
  PlusIcon,
  HardDrivesIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { useCreateNodeJoinToken } from '../queries/nodes'
import { useBrand } from '../hooks/useBrand'
import type { NodeJoinTokenResponse } from '../types/nodeDetail'
import { CommandBlock } from './nodes/CommandBlock'

const NODE_NAME_PATTERN = /^[a-z][a-z0-9-]*$/

// Add-node dialog for the "I already have a server" path. The cloud
// wizard (AddNodeWizard.tsx) reuses ManualEnrollFields as its own step.
export function AddNodeDialog() {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusIcon />
        {t('join.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <ManualEnrollFields
          key={open ? 'open' : 'closed'}
          onDone={() => setOpen(false)}
        />
      </DialogContent>
    </Dialog>
  )
}

// Two phases: a small form (address, optional name, mesh), then the
// single-use token with the server-built join commands.
export function ManualEnrollFields({
  onDone,
  onBack,
}: {
  onDone: () => void
  onBack?: () => void
}) {
  const { t } = useTranslation('nodes')
  const brand = useBrand()
  const [created, setCreated] = useState<NodeJoinTokenResponse | null>(null)
  const [nodeName, setNodeName] = useState('')
  const [mesh, setMesh] = useState(false)
  // The dashboard's own hostname is usually reachable from the new node;
  // 9443 is the control plane's fixed agent gRPC port.
  const [controlPlaneAddr, setControlPlaneAddr] = useState(
    () => `${window.location.hostname}:9443`,
  )
  const createJoinToken = useCreateNodeJoinToken()

  const trimmedName = nodeName.trim()
  const nameInvalid = trimmedName !== '' && !NODE_NAME_PATTERN.test(trimmedName)

  function generate() {
    const addr = controlPlaneAddr.trim()
    createJoinToken.mutate(
      {
        control_plane_addr: addr || undefined,
        node_name: trimmedName || undefined,
        mesh: mesh || undefined,
      },
      { onSuccess: setCreated },
    )
  }

  if (created) {
    const commands = created.join_commands
    const fallbackCommand = [
      `APP_CONTROL_PLANE_ADDR=${controlPlaneAddr.trim()}`,
      `APP_JOIN_TOKEN=${created.token}`,
      created.ca_fingerprint
        ? `APP_CA_FINGERPRINT=${created.ca_fingerprint}`
        : null,
      `./${brand.BinaryName}-agent`,
    ]
      .filter(Boolean)
      .join(' ')
    const expires = new Date(created.expires_at).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
    })

    return (
      <>
        <DialogHeader>
          <DialogTitle>{t('join.createdTitle')}</DialogTitle>
          <DialogDescription>{t('join.createdDescription')}</DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
          <WarningIcon className="mt-0.5 size-4 shrink-0" />
          <p className="text-sm">{t('join.singleUse', { time: expires })}</p>
        </div>

        {commands ? (
          <>
            <CommandBlock
              id="node-join-docker"
              label={t('join.dockerLabel')}
              hint={t('join.dockerHint')}
              value={commands.docker}
            />
            <CommandBlock
              id="node-join-binary"
              label={t('join.binaryLabel')}
              hint={t('join.binaryHint')}
              value={commands.binary}
            />
          </>
        ) : (
          <CommandBlock
            id="node-join-fallback"
            label={t('join.fallbackLabel')}
            hint={t('join.fallbackHint')}
            value={fallbackCommand}
          />
        )}

        <CommandBlock
          id="node-join-token"
          label={t('join.tokenLabel')}
          value={created.token}
          hint={t('join.afterRun')}
        />

        <DialogFooter>
          <Button type="button" onClick={onDone}>
            {t('join.done')}
          </Button>
        </DialogFooter>
      </>
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {onBack ? (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={onBack}
              aria-label={t('join.back')}
            >
              <ArrowLeftIcon />
            </Button>
          ) : null}
          <HardDrivesIcon className="size-4 text-muted-foreground" />
          {t('join.title')}
        </DialogTitle>
        <DialogDescription>{t('join.description')}</DialogDescription>
      </DialogHeader>

      <Field>
        <FieldLabel htmlFor="node-control-plane-addr">
          {t('join.addrLabel')}
        </FieldLabel>
        <Input
          id="node-control-plane-addr"
          value={controlPlaneAddr}
          onChange={(e) => setControlPlaneAddr(e.target.value)}
          className="font-mono text-xs"
        />
        <FieldDescription>{t('join.addrHint')}</FieldDescription>
      </Field>

      <Field>
        <FieldLabel htmlFor="node-join-name">{t('join.nameLabel')}</FieldLabel>
        <Input
          id="node-join-name"
          value={nodeName}
          placeholder={t('join.namePlaceholder')}
          aria-invalid={nameInvalid}
          onChange={(e) => setNodeName(e.target.value)}
          className="font-mono text-xs"
        />
        <FieldDescription>
          {nameInvalid ? t('join.nameInvalid') : t('join.nameHint')}
        </FieldDescription>
      </Field>

      <div className="space-y-1">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={mesh}
            onCheckedChange={(checked) => setMesh(checked === true)}
          />
          {t('join.meshLabel')}
        </label>
        <p className="text-xs text-muted-foreground">{t('join.meshHint')}</p>
      </div>

      {createJoinToken.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{createJoinToken.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button
          type="button"
          disabled={createJoinToken.isPending || nameInvalid}
          onClick={generate}
        >
          {createJoinToken.isPending
            ? t('join.generating')
            : t('join.generate')}
        </Button>
      </DialogFooter>
    </>
  )
}
