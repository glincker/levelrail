import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  PlugsConnectedIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { BrandTile } from '../BrandLogoBadge'
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
import { toast } from '@/components/ui/toast'
import {
  useCreateGitConnection,
  useTestGitConnection,
} from '../../queries/gitConnections'
import type {
  GitConnectionProvider,
  GitProviderConnection,
} from '../../types/gitConnections'

const NEW_APP = 'new'

const DEFAULT_INSTANCE: Record<GitConnectionProvider, string> = {
  gitlab: 'https://gitlab.com',
  gitea: '',
  forgejo: '',
  bitbucket: '',
  bitbucket_server: '',
}

// Bitbucket Server has no mark of its own in thesvg, so it shows Bitbucket's.
const BRAND_MARK: Record<GitConnectionProvider, string> = {
  gitlab: 'gitlab',
  gitea: 'gitea',
  forgejo: 'forgejo',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
}

// Registers an OAuth application (or, for Bitbucket Server, a personal access
// token) as one more connection. It never replaces an existing one.
export function AddGitConnectionDialog({
  provider,
  existing,
  open,
  onOpenChange,
}: Readonly<{
  provider: GitConnectionProvider
  existing: GitProviderConnection[]
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('settings')
  const create = useCreateGitConnection()
  const probe = useTestGitConnection()
  const [source, setSource] = useState(NEW_APP)
  const [instanceUrl, setInstanceUrl] = useState(DEFAULT_INSTANCE[provider])
  const [clientId, setClientId] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [token, setToken] = useState('')
  const [label, setLabel] = useState('')
  const [copied, setCopied] = useState(false)

  const tokenAuth = provider === 'bitbucket_server'
  const reusing = !tokenAuth && source !== NEW_APP
  const needsInstance = provider !== 'bitbucket'
  const canSubmit = tokenAuth
    ? token.trim() !== '' && instanceUrl.trim() !== ''
    : reusing ||
      (clientId.trim() !== '' &&
        clientSecret.trim() !== '' &&
        (!needsInstance || instanceUrl.trim() !== ''))

  function reset() {
    setSource(NEW_APP)
    setInstanceUrl(DEFAULT_INSTANCE[provider])
    setClientId('')
    setClientSecret('')
    setToken('')
    setLabel('')
    create.reset()
    probe.reset()
  }

  function submit() {
    create.mutate(
      {
        provider,
        label: label.trim() || undefined,
        ...(tokenAuth
          ? { instance_url: instanceUrl.trim(), token: token.trim() }
          : reusing
            ? { reuse_connection_id: source }
            : {
                instance_url: needsInstance ? instanceUrl.trim() : undefined,
                client_id: clientId.trim(),
                client_secret: clientSecret,
              }),
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('gitConnections.toast.added'),
            description: t('gitConnections.toast.addedBody'),
            type: 'success',
          })
          reset()
          onOpenChange(false)
          if (created.authorize_path) {
            window.location.assign(created.authorize_path)
          }
        },
      },
    )
  }

  const callback = `${window.location.origin}/api/v1/${provider}-app/callback`

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BrandTile
              name={BRAND_MARK[provider]}
              label={t(`gitConnections.add.title.${provider}`)}
              size="sm"
            />
            {t(`gitConnections.add.title.${provider}`)}
          </DialogTitle>
          <DialogDescription>
            {tokenAuth
              ? t('gitConnections.add.tokenDescription')
              : t('gitConnections.add.description', { callback })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          {tokenAuth ? null : (
            <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/30 p-2">
              <span className="text-xs text-muted-foreground">
                {t('gitConnections.add.callbackLabel')}
              </span>
              <code className="min-w-0 flex-1 break-all text-xs">
                {callback}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  void navigator.clipboard.writeText(callback).then(() => {
                    setCopied(true)
                  })
                }}
              >
                {copied ? (
                  <CheckIcon className="size-4" aria-hidden="true" />
                ) : (
                  <CopyIcon className="size-4" aria-hidden="true" />
                )}
                {copied
                  ? t('gitConnections.add.callbackCopied')
                  : t('gitConnections.add.callbackCopy')}
              </Button>
            </div>
          )}
          {existing.length > 0 && !tokenAuth ? (
            <Field>
              <FieldLabel htmlFor="git-conn-source">
                {t('gitConnections.add.application')}
              </FieldLabel>
              <Select
                value={source}
                onValueChange={(v) => setSource(v ?? NEW_APP)}
              >
                <SelectTrigger id="git-conn-source" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NEW_APP}>
                    {t('gitConnections.add.newApplication')}
                  </SelectItem>
                  {existing.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {t('gitConnections.add.reuse', { label: c.label })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>
                {t('gitConnections.add.applicationHelp')}
              </FieldDescription>
            </Field>
          ) : null}
          {tokenAuth ? (
            <Field>
              <FieldLabel htmlFor="git-conn-token">
                {t('gitConnections.add.token')}
              </FieldLabel>
              <Input
                id="git-conn-token"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={token}
                onChange={(e) => setToken(e.target.value)}
              />
              <FieldDescription>
                {t('gitConnections.add.tokenHelp')}
              </FieldDescription>
            </Field>
          ) : null}
          {reusing ? null : (
            <>
              {needsInstance ? (
                <Field>
                  <FieldLabel htmlFor="git-conn-instance">
                    {t('gitConnections.add.instanceUrl')}
                  </FieldLabel>
                  <Input
                    id="git-conn-instance"
                    className="font-mono"
                    autoComplete="off"
                    spellCheck={false}
                    value={instanceUrl}
                    onChange={(e) => setInstanceUrl(e.target.value)}
                  />
                  <FieldDescription>
                    {tokenAuth
                      ? t('gitConnections.add.instanceHelpServer')
                      : t('gitConnections.add.instanceHelp')}
                  </FieldDescription>
                </Field>
              ) : null}
              {tokenAuth ? null : (
                <>
                  <Field>
                    <FieldLabel htmlFor="git-conn-client-id">
                      {t(`gitConnections.add.clientId.${provider}`)}
                    </FieldLabel>
                    <Input
                      id="git-conn-client-id"
                      autoComplete="off"
                      spellCheck={false}
                      value={clientId}
                      onChange={(e) => setClientId(e.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="git-conn-client-secret">
                      {t('gitConnections.add.clientSecret')}
                    </FieldLabel>
                    <Input
                      id="git-conn-client-secret"
                      type="password"
                      autoComplete="off"
                      spellCheck={false}
                      value={clientSecret}
                      onChange={(e) => setClientSecret(e.target.value)}
                    />
                  </Field>
                </>
              )}
            </>
          )}
          <Field>
            <FieldLabel htmlFor="git-conn-label">
              {t('gitConnections.add.label')}
            </FieldLabel>
            <Input
              id="git-conn-label"
              maxLength={80}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
            <FieldDescription>
              {t('gitConnections.add.labelHelp')}
            </FieldDescription>
          </Field>
          {needsInstance && !reusing ? (
            <div className="space-y-2">
              <Button
                type="button"
                variant="outline"
                disabled={instanceUrl.trim() === '' || probe.isPending}
                onClick={() =>
                  probe.mutate({
                    provider,
                    instance_url: instanceUrl.trim(),
                    token: tokenAuth ? token.trim() || undefined : undefined,
                  })
                }
              >
                <PlugsConnectedIcon className="size-4" aria-hidden="true" />
                {probe.isPending
                  ? t('gitConnections.add.testing')
                  : t('gitConnections.add.test')}
              </Button>
              {probe.data?.ok ? (
                <p role="status" className="text-sm text-muted-foreground">
                  {probe.data.account
                    ? t('gitConnections.add.testOkAccount', {
                        account: probe.data.account,
                      })
                    : t('gitConnections.add.testOk')}
                </p>
              ) : null}
              {probe.data && !probe.data.ok && probe.data.kind !== 'ok' ? (
                <div role="alert" className="space-y-1 text-sm">
                  <p className="text-destructive">
                    {t(
                      `gitConnections.add.testKind.${probe.data.kind}.message`,
                    )}
                  </p>
                  <p className="text-muted-foreground">
                    {t('gitConnections.add.fixLabel')}:{' '}
                    {t(`gitConnections.add.testKind.${probe.data.kind}.fix`)}
                  </p>
                </div>
              ) : null}
              {probe.error ? (
                <p role="alert" className="text-sm text-destructive">
                  {probe.error.message}
                </p>
              ) : null}
            </div>
          ) : null}
          {create.error ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('gitConnections.add.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!canSubmit || create.isPending}
            onClick={submit}
          >
            <PlusIcon className="size-4" aria-hidden="true" />
            {create.isPending
              ? t(
                  tokenAuth
                    ? 'gitConnections.add.verifying'
                    : 'gitConnections.add.saving',
                )
              : t(
                  tokenAuth
                    ? 'gitConnections.add.connect'
                    : 'gitConnections.add.submit',
                )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
