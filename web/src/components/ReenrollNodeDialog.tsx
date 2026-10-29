import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  reenrollCommand,
  useCreateNodeReenrollToken,
} from '../queries/nodeCert'
import type { NodeReenrollTokenResponse } from '../types/nodeCert'
import type { NodeResource } from '../types/nodeDetail'

// Generates a re-enrollment token for an existing node and shows the
// command once, the same shape as AddNodeDialog.
export function ReenrollNodeDialog({
  node,
  defaultOpen = false,
}: {
  node: NodeResource
  defaultOpen?: boolean
}) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(defaultOpen)
  const [created, setCreated] = useState<NodeReenrollTokenResponse | null>(null)
  const [copied, setCopied] = useState(false)
  const [editedAddr, setEditedAddr] = useState<string | null>(null)
  const controlPlaneAddr =
    editedAddr ??
    created?.control_plane_address ??
    `${window.location.hostname}:9443`
  const createToken = useCreateNodeReenrollToken()
  const command = created ? reenrollCommand(created, controlPlaneAddr) : ''

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setEditedAddr(null)
      setCopied(false)
      createToken.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
        {t('reenroll.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('reenroll.title', { name: node.name })}</DialogTitle>
          <DialogDescription>{t('reenroll.description')}</DialogDescription>
        </DialogHeader>

        {created ? (
          <>
            <div className="flex items-start gap-2 rounded-lg border border-tone-warning-border bg-tone-warning-soft p-2.5 text-tone-warning">
              <TriangleAlertGlassIcon className="mt-0.5 size-5 shrink-0 text-tone-warning" />
              <p className="text-sm">
                {t('reenroll.shownOnce', {
                  time: new Date(created.expires_at).toLocaleTimeString(),
                })}
              </p>
            </div>
            <Field>
              <FieldLabel htmlFor="reenroll-control-plane-addr">
                {t('reenroll.addrLabel')}
              </FieldLabel>
              <Input
                id="reenroll-control-plane-addr"
                value={controlPlaneAddr}
                onChange={(e) => {
                  setEditedAddr(e.target.value)
                  setCopied(false)
                }}
                className="font-mono text-xs"
              />
              <FieldDescription>{t('reenroll.addrHint')}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="reenroll-command">
                {t('reenroll.runOnNode')}
              </FieldLabel>
              <div className="flex items-start gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code
                  id="reenroll-command"
                  className="min-w-0 flex-1 overflow-x-auto text-xs break-all whitespace-pre-wrap"
                >
                  {command}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(command).then(() => {
                      setCopied(true)
                    })
                  }}
                >
                  {copied ? (
                    <CheckGlassIcon size="1.4em" />
                  ) : (
                    <CopyGlassIcon size="1.4em" />
                  )}
                  {copied ? t('reenroll.copied') : t('reenroll.copy')}
                </Button>
              </div>
              <FieldDescription>{t('reenroll.afterHint')}</FieldDescription>
            </Field>
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {t('reenroll.done')}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            {createToken.isError ? (
              <Alert variant="destructive">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.4em"
                />
                <AlertDescription>{createToken.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                disabled={createToken.isPending}
                onClick={() => {
                  createToken.mutate(node.id, { onSuccess: setCreated })
                }}
              >
                {createToken.isPending
                  ? t('reenroll.generating')
                  : t('reenroll.generate')}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
