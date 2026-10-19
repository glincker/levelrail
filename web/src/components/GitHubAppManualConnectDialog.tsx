import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useConnectGitHubAppManually } from '../queries/githubApp'
import {
  FormDialogFooter,
  mutationToastCallbacks,
  ResettableDialog,
} from './ConnectionCard'

// ManualConnectDialog: an operator creates the App themselves at
// github.com/settings/apps/new and pastes the resulting credentials
// in directly. installation_id/account_login are optional: an App
// saved without them still connects (status.installed stays false).
export function ManualConnectDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('settings')
  const connect = useConnectGitHubAppManually()
  const [instanceURL, setInstanceURL] = useState('')
  const [appID, setAppID] = useState('')
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [installationID, setInstallationID] = useState('')
  const [accountLogin, setAccountLogin] = useState('')

  function resetForm() {
    setInstanceURL('')
    setAppID('')
    setClientID('')
    setClientSecret('')
    setWebhookSecret('')
    setPrivateKey('')
    setInstallationID('')
    setAccountLogin('')
  }

  const appIDNum = Number(appID)
  const canSubmit =
    appID.trim() !== '' &&
    Number.isFinite(appIDNum) &&
    appIDNum > 0 &&
    clientID.trim() !== '' &&
    clientSecret.trim() !== '' &&
    webhookSecret.trim() !== '' &&
    privateKey.trim() !== ''

  function handleSubmit() {
    if (!canSubmit) {
      return
    }
    const installationIDNum = installationID.trim()
      ? Number(installationID)
      : undefined
    connect.mutate(
      {
        app_id: appIDNum,
        client_id: clientID.trim(),
        instance_url: instanceURL.trim() || undefined,
        client_secret: clientSecret,
        webhook_secret: webhookSecret,
        private_key: privateKey,
        installation_id:
          installationIDNum !== undefined && Number.isFinite(installationIDNum)
            ? installationIDNum
            : undefined,
        account_login: accountLogin.trim() || undefined,
      },
      mutationToastCallbacks(
        t('githubManualDialog.connected'),
        t('githubManualDialog.connectFailed'),
        () => {
          resetForm()
          onOpenChange(false)
        },
      ),
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
          <DialogTitle>{t('githubManualDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('githubManualDialog.descPrefix')}{' '}
            <a
              href="https://github.com/settings/apps/new"
              target="_blank"
              rel="noreferrer"
              className="underline"
            >
              github.com/settings/apps/new
            </a>{' '}
            {t('githubManualDialog.descSuffix')}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto pr-1">
          <Field>
            <FieldLabel htmlFor="gh-instance-url">
              {t('githubManualDialog.instanceLabel')}
            </FieldLabel>
            <Input
              id="gh-instance-url"
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
              {t('githubManualDialog.instanceHint')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-app-id">
              {t('githubManualDialog.appId')}
            </FieldLabel>
            <Input
              id="gh-app-id"
              type="number"
              inputMode="numeric"
              value={appID}
              onChange={(e) => {
                setAppID(e.target.value)
              }}
              placeholder="123456"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-client-id">
              {t('githubManualDialog.clientId')}
            </FieldLabel>
            <Input
              id="gh-client-id"
              value={clientID}
              onChange={(e) => {
                setClientID(e.target.value)
              }}
              placeholder="Iv1.abc123def456"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-client-secret">
              {t('githubManualDialog.clientSecret')}
            </FieldLabel>
            <Input
              id="gh-client-secret"
              type="password"
              value={clientSecret}
              onChange={(e) => {
                setClientSecret(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-webhook-secret">
              {t('githubManualDialog.webhookSecret')}
            </FieldLabel>
            <Input
              id="gh-webhook-secret"
              type="password"
              value={webhookSecret}
              onChange={(e) => {
                setWebhookSecret(e.target.value)
              }}
            />
            <FieldDescription>
              {t('githubManualDialog.webhookHint')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="gh-private-key">
              {t('githubManualDialog.privateKey')}
            </FieldLabel>
            <Textarea
              id="gh-private-key"
              value={privateKey}
              onChange={(e) => {
                setPrivateKey(e.target.value)
              }}
              placeholder="-----BEGIN RSA PRIVATE KEY-----"
              rows={6}
              className="font-mono text-xs"
            />
            <FieldDescription>
              {t('githubManualDialog.privateKeyHint')}
            </FieldDescription>
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel htmlFor="gh-installation-id">
                {t('githubManualDialog.installationId')}
              </FieldLabel>
              <Input
                id="gh-installation-id"
                type="number"
                inputMode="numeric"
                value={installationID}
                onChange={(e) => {
                  setInstallationID(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="gh-account-login">
                {t('githubManualDialog.accountLogin')}
              </FieldLabel>
              <Input
                id="gh-account-login"
                value={accountLogin}
                onChange={(e) => {
                  setAccountLogin(e.target.value)
                }}
                placeholder="octocat"
              />
            </Field>
          </div>
        </div>
        <FormDialogFooter
          onCancel={() => {
            resetForm()
            onOpenChange(false)
          }}
          submitDisabled={!canSubmit || connect.isPending}
          pending={connect.isPending}
          onSubmit={handleSubmit}
          submitLabel={t('githubManualDialog.connect')}
          pendingLabel={t('githubManualDialog.connecting')}
        />
      </DialogContent>
    </ResettableDialog>
  )
}
