import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  KeyGlassIcon,
  KeyRotationGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { RoleGate } from '@/components/ui/RoleGate'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import {
  useDeleteGitSshKey,
  useGitSshKey,
  useSetGitSshKey,
} from '../queries/gitSshKey'

type Mode = 'paste' | 'tofu'
type Confirm = 'rotate' | 'remove' | null

export function GitSshKeySection({ appName }: { appName: string }) {
  const { t } = useTranslation('gitSource')
  const key = useGitSshKey(appName)
  const set = useSetGitSshKey(appName)
  const del = useDeleteGitSshKey(appName)
  const [mode, setMode] = useState<Mode>('paste')
  const [hostKey, setHostKey] = useState('')
  const [confirm, setConfirm] = useState<Confirm>(null)
  const [copied, setCopied] = useState(false)

  const data = key.data ?? null
  const canSubmit = mode === 'tofu' || hostKey.trim() !== ''

  function generate() {
    const req =
      mode === 'tofu'
        ? { trust_on_first_use: true as const }
        : { host_key: hostKey.trim() }
    set.mutate(req, {
      onSuccess: () => {
        setConfirm(null)
        setHostKey('')
        toast.add({ title: t('sshKey.saved'), type: 'success' })
      },
      onError: () => setConfirm(null),
    })
  }

  function copyKey() {
    if (!data) return
    void navigator.clipboard.writeText(data.public_key).then(() => {
      setCopied(true)
    })
  }

  return (
    <section
      className="space-y-2 rounded-lg border border-input p-3"
      aria-labelledby="git-ssh-key-title"
      data-testid="git-ssh-key"
    >
      <div className="flex flex-wrap items-center gap-2">
        <KeyGlassIcon className="size-5" aria-hidden="true" />
        <h3 id="git-ssh-key-title" className="text-sm font-medium">
          {t('sshKey.title')}
        </h3>
        <span className="text-xs text-muted-foreground">
          {key.isLoading
            ? t('sshKey.loading')
            : data
              ? t('sshKey.fingerprint', { fingerprint: data.fingerprint })
              : t('sshKey.none')}
        </span>
        {data ? (
          <div className="ml-auto flex gap-2">
            <RoleGate ability="write:sensitive">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setConfirm('rotate')}
              >
                <KeyRotationGlassIcon className="size-5" aria-hidden="true" />
                {t('sshKey.rotate')}
              </Button>
            </RoleGate>
            <RoleGate ability="write:sensitive">
              <Button
                type="button"
                size="sm"
                variant="destructive"
                onClick={() => setConfirm('remove')}
              >
                <TrashGlassIcon className="size-5" aria-hidden="true" />
                {t('sshKey.remove')}
              </Button>
            </RoleGate>
          </div>
        ) : null}
      </div>

      {key.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {key.error.message}
        </p>
      ) : null}

      {data ? (
        <div className="space-y-1">
          <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
            <code
              aria-label={t('sshKey.publicKey')}
              className="min-w-0 flex-1 overflow-x-auto text-xs break-all"
            >
              {data.public_key}
            </code>
            <Button type="button" size="sm" variant="outline" onClick={copyKey}>
              {copied ? (
                <CheckGlassIcon size="1.4em" />
              ) : (
                <CopyGlassIcon size="1.4em" />
              )}
              {copied ? t('copy.copied') : t('copy.copy')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t('sshKey.hint')}</p>
          {data.host_key_pinned && data.host_fingerprint ? (
            <p className="text-xs text-muted-foreground">
              {t('sshKey.hostPinned', { fingerprint: data.host_fingerprint })}
            </p>
          ) : null}
        </div>
      ) : null}

      {!data && !key.isLoading && !key.isError ? (
        <GenerateForm
          mode={mode}
          hostKey={hostKey}
          onMode={setMode}
          onHostKey={setHostKey}
          canSubmit={canSubmit}
          pending={set.isPending}
          onSubmit={generate}
        />
      ) : null}

      {set.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {set.error.message}
        </p>
      ) : null}
      {del.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {del.error.message}
        </p>
      ) : null}

      <Dialog
        open={confirm !== null}
        onOpenChange={(open) => {
          if (!open) setConfirm(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {confirm === 'rotate'
                ? t('sshKey.rotateTitle')
                : t('sshKey.removeTitle')}
            </DialogTitle>
            <DialogDescription>
              {confirm === 'rotate'
                ? t('sshKey.rotateBody')
                : t('sshKey.removeBody')}
            </DialogDescription>
          </DialogHeader>
          {confirm === 'rotate' ? (
            <GenerateForm
              mode={mode}
              hostKey={hostKey}
              onMode={setMode}
              onHostKey={setHostKey}
              canSubmit={canSubmit}
              pending={set.isPending}
              onSubmit={generate}
              submitLabel={t('sshKey.rotateConfirm')}
              hideSubmit
            />
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setConfirm(null)}
            >
              {t('sshKey.cancel')}
            </Button>
            {confirm === 'rotate' ? (
              <Button
                type="button"
                disabled={!canSubmit || set.isPending}
                onClick={generate}
              >
                {t('sshKey.rotateConfirm')}
              </Button>
            ) : (
              <Button
                type="button"
                variant="destructive"
                disabled={del.isPending}
                onClick={() =>
                  del.mutate(undefined, {
                    onSuccess: () => {
                      setConfirm(null)
                      toast.add({ title: t('sshKey.removed'), type: 'success' })
                    },
                    onError: () => setConfirm(null),
                  })
                }
              >
                {t('sshKey.removeConfirm')}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}

function GenerateForm({
  mode,
  hostKey,
  onMode,
  onHostKey,
  canSubmit,
  pending,
  onSubmit,
  submitLabel,
  hideSubmit,
}: {
  mode: Mode
  hostKey: string
  onMode: (m: Mode) => void
  onHostKey: (v: string) => void
  canSubmit: boolean
  pending: boolean
  onSubmit: () => void
  submitLabel?: string
  hideSubmit?: boolean
}) {
  const { t } = useTranslation('gitSource')
  const name = `ssh-host-mode-${hideSubmit ? 'rotate' : 'new'}`
  return (
    <div className="space-y-2">
      <fieldset className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
        <legend className="sr-only">{t('sshKey.hostKeyChoice')}</legend>
        {(['paste', 'tofu'] as const).map((m) => (
          <label key={m} className="flex items-center gap-1.5">
            <input
              type="radio"
              name={name}
              checked={mode === m}
              onChange={() => onMode(m)}
            />
            {t(`sshKey.mode.${m}`)}
          </label>
        ))}
      </fieldset>
      {mode === 'paste' ? (
        <Textarea
          aria-label={t('sshKey.hostKeyLabel')}
          placeholder={t('sshKey.hostKeyPlaceholder')}
          value={hostKey}
          onChange={(e) => onHostKey(e.target.value)}
          className="min-h-16 font-mono text-xs"
        />
      ) : (
        <p role="note" className="text-xs font-medium text-foreground">
          {t('sshKey.tofuWarning')}
        </p>
      )}
      {hideSubmit ? null : (
        <RoleGate ability="write:sensitive">
          <Button
            type="button"
            size="sm"
            disabled={!canSubmit || pending}
            onClick={onSubmit}
          >
            <KeyGlassIcon className="size-5" aria-hidden="true" />
            {submitLabel ?? t('sshKey.generate')}
          </Button>
        </RoleGate>
      )}
    </div>
  )
}
