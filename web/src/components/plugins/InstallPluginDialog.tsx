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
  useInstallPlugin,
  usePluginKeys,
  type Plugin,
} from '../../queries/plugins'

export function InstallPluginDialog({
  open,
  onClose,
  onInstalled,
}: Readonly<{
  open: boolean
  onClose: () => void
  onInstalled: (plugin: Plugin) => void
}>) {
  const { t } = useTranslation('plugins')
  const install = useInstallPlugin()
  const { data: keys = [] } = usePluginKeys()
  const [manifest, setManifest] = useState('')
  const [signature, setSignature] = useState('')
  const [keyId, setKeyId] = useState('')
  const [digest, setDigest] = useState('')
  const [config, setConfig] = useState('')

  const ready =
    manifest.trim() !== '' && signature.trim() !== '' && keyId !== ''

  const submit = () =>
    install.mutate(
      {
        manifest,
        signature: signature.trim(),
        key_id: keyId,
        expected_digest: digest.trim() || undefined,
        config_json: config.trim() || undefined,
      },
      {
        onSuccess: (plugin) => {
          toast.add({
            type: 'success',
            title: t('installDialog.installed', { name: plugin.name }),
          })
          setManifest('')
          setSignature('')
          setDigest('')
          setConfig('')
          install.reset()
          onInstalled(plugin)
        },
      },
    )

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('installDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('installDialog.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
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
          </Field>
          <Field>
            <FieldLabel htmlFor="plugin-key">
              {t('installDialog.keyId')}
            </FieldLabel>
            <Select value={keyId} onValueChange={(v) => setKeyId(v ?? '')}>
              <SelectTrigger id="plugin-key" className="w-full">
                <SelectValue placeholder={t('installDialog.keyPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
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
          <Field>
            <FieldLabel htmlFor="plugin-digest">
              {t('installDialog.digest')}
            </FieldLabel>
            <Input
              id="plugin-digest"
              value={digest}
              onChange={(e) => setDigest(e.target.value)}
              spellCheck={false}
              className="font-mono text-xs"
            />
            <FieldDescription>{t('installDialog.digestHint')}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="plugin-config">
              {t('installDialog.config')}
            </FieldLabel>
            <Textarea
              id="plugin-config"
              value={config}
              onChange={(e) => setConfig(e.target.value)}
              spellCheck={false}
              className="font-mono text-xs"
              rows={3}
            />
            <FieldDescription>{t('installDialog.configHint')}</FieldDescription>
          </Field>
          {install.error ? (
            <p role="alert" className="text-sm text-destructive">
              {install.error.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('installDialog.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!ready || install.isPending}
            onClick={submit}
          >
            {install.isPending
              ? t('installDialog.pending')
              : t('installDialog.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
