import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { useGitHubAppManifestPreview } from '../queries/githubApp'
import {
  isValidOrgLogin,
  registerStartParams,
  type GitHubAppOwnerKind,
} from '../lib/githubAppOwner'
import { FormDialogFooter, ResettableDialog } from './ConnectionCard'

// GitHubAppOwnerFields picks who owns the App on GitHub and whether other
// accounts may install it. A private App can only be installed on its owner.
export function GitHubAppOwnerFields({
  ownerKind,
  onOwnerKindChange,
  orgLogin,
  onOrgLoginChange,
  allowOthers,
  onAllowOthersChange,
}: Readonly<{
  ownerKind: GitHubAppOwnerKind
  onOwnerKindChange: (kind: GitHubAppOwnerKind) => void
  orgLogin: string
  onOrgLoginChange: (login: string) => void
  allowOthers: boolean
  onAllowOthersChange: (allow: boolean) => void
}>) {
  const { t } = useTranslation('settings')
  const orgInvalid =
    ownerKind === 'organization' &&
    orgLogin.trim() !== '' &&
    !isValidOrgLogin(orgLogin)

  return (
    <div className="space-y-4">
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-foreground">
          {t('githubApp.owner.legend')}
        </legend>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name="gh-app-owner"
            className="accent-primary"
            checked={ownerKind === 'personal'}
            onChange={() => onOwnerKindChange('personal')}
          />
          {t('githubApp.owner.personal')}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name="gh-app-owner"
            className="accent-primary"
            checked={ownerKind === 'organization'}
            onChange={() => onOwnerKindChange('organization')}
          />
          {t('githubApp.owner.organization')}
        </label>
        {ownerKind === 'organization' ? (
          <Field>
            <FieldLabel htmlFor="gh-preview-owner-login">
              {t('githubApp.owner.orgLogin')}
            </FieldLabel>
            <Input
              id="gh-preview-owner-login"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              aria-invalid={orgInvalid}
              value={orgLogin}
              onChange={(e) => onOrgLoginChange(e.target.value)}
              placeholder={t('githubApp.owner.orgPlaceholder')}
            />
            <FieldDescription>
              {orgInvalid
                ? t('githubApp.owner.orgInvalid')
                : t('githubApp.owner.orgHelp')}
            </FieldDescription>
          </Field>
        ) : null}
      </fieldset>
      <Field>
        <div className="flex items-start gap-2">
          <Checkbox
            id="gh-preview-public"
            checked={allowOthers}
            onCheckedChange={(checked) => onAllowOthersChange(checked === true)}
          />
          <FieldLabel htmlFor="gh-preview-public">
            {t('githubApp.owner.allowOthers')}
          </FieldLabel>
        </div>
        <FieldDescription>
          {allowOthers
            ? t('githubApp.owner.publicHelp')
            : t(
                ownerKind === 'organization'
                  ? 'githubApp.owner.privateOrgHelp'
                  : 'githubApp.owner.privatePersonalHelp',
              )}
        </FieldDescription>
      </Field>
    </div>
  )
}

// GitHubAppManifestDialog shows what register/start is about to send
// GitHub before the browser navigates away, and lets the operator pick the
// owner (personal or org) and visibility. Fetches the read-only preview
// only while open; confirming does a full-page navigation.
export function GitHubAppManifestDialog({
  open,
  onOpenChange,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
}>) {
  const { t } = useTranslation('settings')
  const [instanceURL, setInstanceURL] = useState('')
  const trimmedInstanceURL = instanceURL.trim()
  const {
    data: preview,
    isLoading,
    isError,
    error,
  } = useGitHubAppManifestPreview(trimmedInstanceURL, open)
  const [name, setName] = useState('')
  const [ownerKind, setOwnerKind] = useState<GitHubAppOwnerKind>('personal')
  const [orgLogin, setOrgLogin] = useState('')
  const [allowOthers, setAllowOthers] = useState(false)
  // Empty means untouched: fall back to the fetched default name.
  const displayName = name === '' ? (preview?.app_name ?? '') : name
  const orgMissing =
    ownerKind === 'organization' &&
    (orgLogin.trim() === '' || !isValidOrgLogin(orgLogin))

  function resetForm() {
    setName('')
    setInstanceURL('')
    setOwnerKind('personal')
    setOrgLogin('')
    setAllowOthers(false)
  }

  function handleConfirm() {
    const params = registerStartParams({
      name,
      instanceURL,
      ownerKind,
      orgLogin,
      allowOthers,
    })
    window.location.href = `/api/v1/github-app/register/start?${params.toString()}`
  }

  function renderPreviewBody() {
    if (isLoading) {
      return (
        <div className="space-y-4" aria-hidden="true">
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-24" />
            <Skeleton className="h-9 w-full" />
          </div>
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-16" />
            <Skeleton className="h-9 w-full" />
          </div>
          <div className="space-y-1.5">
            <Skeleton className="h-3.5 w-full" />
            <Skeleton className="h-3.5 w-5/6" />
            <Skeleton className="h-3.5 w-2/3" />
          </div>
        </div>
      )
    }
    if (isError) {
      return (
        <p className="text-sm text-destructive">
          {error instanceof Error
            ? error.message
            : t('githubApp.preview.loadError')}
        </p>
      )
    }
    if (!preview) {
      return null
    }
    return (
      <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
        <Field>
          <FieldLabel htmlFor="gh-preview-instance-url">
            {t('githubApp.preview.instance')}
          </FieldLabel>
          <Input
            id="gh-preview-instance-url"
            className="font-mono"
            autoComplete="off"
            spellCheck={false}
            value={instanceURL}
            onChange={(e) => {
              setInstanceURL(e.target.value)
            }}
            placeholder="https://github.com"
          />
          <FieldDescription>
            {t('githubApp.preview.instanceHelp')}
          </FieldDescription>
        </Field>
        <GitHubAppOwnerFields
          ownerKind={ownerKind}
          onOwnerKindChange={setOwnerKind}
          orgLogin={orgLogin}
          onOrgLoginChange={setOrgLogin}
          allowOthers={allowOthers}
          onAllowOthersChange={setAllowOthers}
        />
        <Field>
          <FieldLabel htmlFor="gh-preview-name">
            {t('githubApp.preview.appName')}
          </FieldLabel>
          <Input
            id="gh-preview-name"
            value={displayName}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
          <FieldDescription>
            {t('githubApp.preview.appNameHelp')}
          </FieldDescription>
        </Field>
        <div className="space-y-1.5 text-sm">
          <UrlRow
            label={t('githubApp.preview.homepage')}
            value={preview.homepage_url}
          />
          <UrlRow
            label={t('githubApp.preview.callback')}
            value={preview.callback_url}
          />
          <UrlRow
            label={t('githubApp.preview.setup')}
            value={preview.setup_url}
          />
          <UrlRow
            label={t('githubApp.preview.webhook')}
            value={preview.webhook_url}
            note={
              preview.webhook_active
                ? undefined
                : t('githubApp.preview.webhookInactive')
            }
          />
        </div>
        <div className="space-y-1.5">
          <p className="text-sm font-medium text-foreground">
            {t('githubApp.preview.permissions')}
          </p>
          <ul className="space-y-1 text-sm text-muted-foreground">
            {Object.entries(preview.permissions).map(([key, level]) => (
              <li key={key} className="font-mono text-xs">
                {key}: {level}
              </li>
            ))}
          </ul>
        </div>
      </div>
    )
  }

  return (
    <ResettableDialog
      open={open}
      onOpenChange={onOpenChange}
      onReset={resetForm}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('githubApp.preview.title')}</DialogTitle>
          <DialogDescription>
            {t('githubApp.preview.description')}
          </DialogDescription>
        </DialogHeader>
        {renderPreviewBody()}
        <FormDialogFooter
          onCancel={() => onOpenChange(false)}
          submitDisabled={!preview || orgMissing}
          pending={false}
          onSubmit={handleConfirm}
          submitLabel={t('githubApp.preview.continue')}
          pendingLabel={t('githubApp.preview.continue')}
        />
      </DialogContent>
    </ResettableDialog>
  )
}

function UrlRow({
  label,
  value,
  note,
}: {
  label: string
  value: string
  note?: string
}) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right font-mono text-xs">
        {value}
        {note ? (
          <span className="ml-1.5 text-amber-700 dark:text-amber-400">
            ({note})
          </span>
        ) : null}
      </span>
    </div>
  )
}
