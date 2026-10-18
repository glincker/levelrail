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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useInspectPlugin,
  useInstallPlugin,
  usePluginKeys,
  useUpgradePlugin,
  type InstallPluginInput,
  type Plugin,
  type PluginPreview,
} from '../../queries/plugins'
import { PluginReview } from './PluginReview'

const AUTO_KEY = 'auto'

export function InstallPluginDialog({
  open,
  upgradeOf,
  onClose,
  onInstalled,
}: Readonly<{
  open: boolean
  upgradeOf?: string
  onClose: () => void
  onInstalled: (plugin: Plugin) => void
}>) {
  const { t } = useTranslation('plugins')
  const inspect = useInspectPlugin()
  const install = useInstallPlugin()
  const upgrade = useUpgradePlugin()
  const { data: keys = [] } = usePluginKeys()
  const [source, setSource] = useState<'manifest' | 'url'>('manifest')
  const [manifest, setManifest] = useState('')
  const [url, setUrl] = useState('')
  const [signature, setSignature] = useState('')
  const [keyId, setKeyId] = useState(AUTO_KEY)
  const [preview, setPreview] = useState<PluginPreview | null>(null)

  const input = (): InstallPluginInput => ({
    manifest: source === 'manifest' ? manifest : undefined,
    manifest_url: source === 'url' ? url.trim() : undefined,
    signature: signature.trim() || undefined,
    key_id: keyId === AUTO_KEY ? undefined : keyId,
  })
  const ready =
    source === 'manifest' ? manifest.trim() !== '' : url.trim() !== ''
  const applying = install.isPending || upgrade.isPending
  const error = inspect.error ?? install.error ?? upgrade.error

  const reset = () => {
    setPreview(null)
    setManifest('')
    setUrl('')
    setSignature('')
    inspect.reset()
    install.reset()
    upgrade.reset()
  }

  const review = () =>
    inspect.mutate(input(), { onSuccess: (p) => setPreview(p) })

  const approve = () => {
    if (!preview) return
    const body = { ...input(), approve_abilities: preview.manifest.abilities }
    const done = (plugin: Plugin) => {
      toast.add({
        type: 'success',
        title: t('installDialog.installedApproved', { name: plugin.name }),
      })
      reset()
      onInstalled(plugin)
    }
    if (preview.action === 'upgrade') {
      upgrade.mutate(
        { name: preview.manifest.name, input: body },
        { onSuccess: done },
      )
    } else {
      install.mutate(body, { onSuccess: done })
    }
  }

  const wrongPlugin =
    preview !== null &&
    upgradeOf !== undefined &&
    preview.manifest.name !== upgradeOf

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {upgradeOf
              ? t('installDialog.upgradeTitle', { name: upgradeOf })
              : t('installDialog.title')}
          </DialogTitle>
          <DialogDescription>
            {preview
              ? t('installDialog.reviewDescription')
              : t('installDialog.description')}
          </DialogDescription>
        </DialogHeader>
        {preview ? (
          <PluginReview preview={preview} />
        ) : (
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor="plugin-source">
                {t('installDialog.source')}
              </FieldLabel>
              <Select
                value={source}
                onValueChange={(v) =>
                  setSource(v === 'url' ? 'url' : 'manifest')
                }
              >
                <SelectTrigger id="plugin-source" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="manifest">
                    {t('installDialog.sourceManifest')}
                  </SelectItem>
                  <SelectItem value="url">
                    {t('installDialog.sourceUrl')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
            {source === 'manifest' ? (
              <Field>
                <FieldLabel htmlFor="plugin-manifest">
                  {t('installDialog.manifest')}
                </FieldLabel>
                <Textarea
                  id="plugin-manifest"
                  value={manifest}
                  onChange={(e) => setManifest(e.target.value)}
                  spellCheck={false}
                  className="font-mono text-xs"
                  rows={8}
                />
                <FieldDescription>
                  {t('installDialog.manifestHint')}
                </FieldDescription>
              </Field>
            ) : (
              <Field>
                <FieldLabel htmlFor="plugin-url">
                  {t('installDialog.url')}
                </FieldLabel>
                <Input
                  id="plugin-url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  spellCheck={false}
                  placeholder="https://"
                  className="font-mono text-xs"
                />
                <FieldDescription>
                  {t('installDialog.urlHint')}
                </FieldDescription>
              </Field>
            )}
            <Field>
              <FieldLabel htmlFor="plugin-signature">
                {t('installDialog.signature')}
              </FieldLabel>
              <Input
                id="plugin-signature"
                value={signature}
                onChange={(e) => setSignature(e.target.value)}
                spellCheck={false}
                className="font-mono text-xs"
              />
              <FieldDescription>
                {source === 'url'
                  ? t('installDialog.signatureHintUrl')
                  : t('installDialog.signatureHint')}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="plugin-key">
                {t('installDialog.keyId')}
              </FieldLabel>
              <Select
                value={keyId}
                onValueChange={(v) => setKeyId(v ?? AUTO_KEY)}
              >
                <SelectTrigger id="plugin-key" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={AUTO_KEY}>
                    {t('installDialog.keyAuto')}
                  </SelectItem>
                  {keys.map((k) => (
                    <SelectItem key={k.key_id} value={k.key_id}>
                      {k.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {keys.length === 0 ? (
                <FieldDescription>{t('installDialog.noKeys')}</FieldDescription>
              ) : null}
            </Field>
          </div>
        )}
        {wrongPlugin ? (
          <p role="alert" className="text-sm text-destructive">
            {t('installDialog.wrongPlugin', { name: upgradeOf })}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={preview ? () => setPreview(null) : onClose}
          >
            {preview ? t('installDialog.back') : t('installDialog.cancel')}
          </Button>
          {preview ? (
            <Button
              type="button"
              disabled={
                !preview.verification.installable || wrongPlugin || applying
              }
              onClick={approve}
            >
              {applying
                ? t('installDialog.pending')
                : t('installDialog.approveInstall')}
            </Button>
          ) : (
            <Button
              type="button"
              disabled={!ready || inspect.isPending}
              onClick={review}
            >
              {inspect.isPending
                ? t('installDialog.reviewing')
                : t('installDialog.review')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
