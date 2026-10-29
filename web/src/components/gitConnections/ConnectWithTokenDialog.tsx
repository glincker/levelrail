import { useContext, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
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
import { SecretInput } from '@/components/ui/secret-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useDraft } from '../../hooks/useDraft'
import { BrandContext } from '../../lib/brandContext'
import {
  TOKEN_GUIDES,
  TOKEN_HOSTS,
  type TokenHost,
} from '../../lib/gitTokenGuides'
import {
  useCreateGitConnection,
  useTestGitConnection,
} from '../../queries/gitConnections'
import { DraftBanner } from '../DraftBanner'
import { GitTokenGuide } from './GitTokenGuide'

interface TokenDraft {
  picked: TokenHost
  urls: Partial<Record<TokenHost, string>>
  label: string
}

// Connects GitLab, Bitbucket Cloud, Gitea or Forgejo with a pasted token: no OAuth
// application, no public domain. The token is checked before it is saved.
export default function ConnectWithTokenDialog({
  provider,
  open,
  onOpenChange,
}: Readonly<{
  provider?: TokenHost
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('settings')
  const brand = useContext(BrandContext)
  const create = useCreateGitConnection()
  const probe = useTestGitConnection()
  const draft = useDraft<TokenDraft>('git-token-connect', {
    picked: provider ?? 'gitlab',
    urls: {},
    label: '',
  })
  const { picked, urls, label } = draft.value
  const host = provider ?? picked
  const guide = TOKEN_GUIDES[host]
  const [token, setToken] = useState('')
  const serverUrl = urls[host] ?? guide.defaultUrl
  const canSubmit =
    token.trim() !== '' && (!guide.selfHosted || serverUrl.trim() !== '')

  function reset() {
    setToken('')
    create.reset()
    probe.reset()
  }

  const body = {
    provider: host,
    instance_url: guide.selfHosted ? serverUrl.trim() : undefined,
    token: token.trim(),
  }

  function submit() {
    create.mutate(
      { ...body, label: label.trim() || undefined },
      {
        onSuccess: () => {
          toast.add({
            title: t('gitConnections.token.connected'),
            description: t('gitConnections.token.connectedBody'),
            type: 'success',
          })
          draft.clearOnSuccess()
          draft.discard()
          reset()
          onOpenChange(false)
        },
      },
    )
  }

  const result = probe.data
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent className="flex max-h-[90vh] flex-col sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('gitConnections.token.title')}</DialogTitle>
          <DialogDescription>
            {t('gitConnections.token.description')}
          </DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="min-w-0 flex-1 space-y-4 overflow-y-auto"
          onSubmit={(e) => {
            e.preventDefault()
            if (canSubmit) submit()
          }}
        >
          <DraftBanner draft={draft} />
          {provider ? null : (
            <Field>
              <FieldLabel htmlFor="token-host">
                {t('gitConnections.token.host')}
              </FieldLabel>
              <Select
                value={host}
                onValueChange={(v) => {
                  draft.update({ picked: v ?? 'gitlab' })
                  probe.reset()
                }}
              >
                <SelectTrigger id="token-host" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TOKEN_HOSTS.map((h) => (
                    <SelectItem key={h} value={h}>
                      {t(`gitConnectionsHub.hosts.${h}.name`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
          {guide.selfHosted ? (
            <Field>
              <FieldLabel htmlFor="token-server">
                {t('gitConnections.add.instanceUrl')}
              </FieldLabel>
              <Input
                id="token-server"
                className="font-mono"
                autoComplete="off"
                spellCheck={false}
                value={serverUrl}
                onChange={(e) =>
                  draft.update({ urls: { ...urls, [host]: e.target.value } })
                }
              />
              <FieldDescription>
                {t('gitConnections.token.serverHelp')}
              </FieldDescription>
            </Field>
          ) : null}
          <GitTokenGuide
            host={host}
            serverUrl={serverUrl}
            tokenName={brand?.Name ?? 'api'}
          />
          <Field>
            <FieldLabel htmlFor="token-value">
              {t(
                guide.tokenFormat === 'emailToken'
                  ? 'gitConnections.token.fieldEmailToken'
                  : 'gitConnections.token.field',
              )}
            </FieldLabel>
            <SecretInput
              id="token-value"
              autoComplete="off"
              spellCheck={false}
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="token-label">
              {t('gitConnections.add.label')}
            </FieldLabel>
            <Input
              id="token-label"
              maxLength={80}
              value={label}
              onChange={(e) => draft.update({ label: e.target.value })}
            />
          </Field>
          <div className="space-y-2">
            <Button
              type="button"
              variant="outline"
              disabled={!canSubmit || probe.isPending}
              onClick={() => probe.mutate(body)}
            >
              <PlugGlassIcon className="size-4.5" aria-hidden="true" />
              {probe.isPending
                ? t('gitConnections.add.testing')
                : t('gitConnections.token.check')}
            </Button>
            {result?.ok ? (
              <ul
                role="status"
                className="space-y-0.5 text-sm text-muted-foreground"
              >
                <li>
                  {t('gitConnections.add.testOkAccount', {
                    account: result.account,
                  })}
                </li>
                <li>
                  {result.token?.scopes_known
                    ? t('gitConnections.token.scopesFound', {
                        scopes: result.token.scopes?.join(', '),
                      })
                    : t('gitConnections.token.scopesUnknown')}
                </li>
                <li>
                  {result.token?.repo_access
                    ? t('gitConnections.token.repos', {
                        count: result.token.repo_count,
                        plus: result.token.repo_count_capped ? '+' : '',
                      })
                    : t('gitConnections.token.noRepos')}
                </li>
              </ul>
            ) : null}
            {result && !result.ok && result.kind !== 'ok' ? (
              <div role="alert" className="space-y-1 text-sm">
                <p className="text-destructive">
                  {t(`gitConnections.add.testKind.${result.kind}.message`)}
                </p>
                <p className="text-muted-foreground">
                  {t('gitConnections.add.fixLabel')}:{' '}
                  {t(`gitConnections.add.testKind.${result.kind}.fix`)}
                </p>
              </div>
            ) : null}
            {probe.error ? (
              <p role="alert" className="text-sm text-destructive">
                {probe.error.message}
              </p>
            ) : null}
          </div>
          {create.error ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
        </form>
        <DialogFooter className="flex-wrap">
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
            <PlusGlassIcon className="size-4.5" aria-hidden="true" />
            {create.isPending
              ? t('gitConnections.add.verifying')
              : t('gitConnections.add.connect')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
