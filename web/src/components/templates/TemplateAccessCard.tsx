import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  ArrowSquareOutIcon,
  KeyIcon,
  ScrollIcon,
} from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { buttonVariants } from '@/components/ui/button'
import { StepUpDialog } from '../env/StepUpDialog'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { useRevealedValues } from '../../hooks/useRevealedValues'
import {
  EnvApiError,
  revealValue,
  useEnvSettings,
} from '../../queries/envProtection'
import { useAppUrls } from '../../queries/appUrls'
import { useServiceTemplate } from '../../queries/serviceTemplates'
import type { AppDetail } from '../../types/appDetail'
import { directUrl } from '../../lib/firstApp'
import { TemplateCredentialRow } from './TemplateCredentialRow'
import { TemplateDescription } from './TemplateDescription'

export interface TemplateAccessCardProps {
  /** The service whose address and generated secrets are shown. */
  app: AppDetail
  templateId?: string
  onNavigate?: () => void
}

/** Address, generated credentials (revealed on demand), setup notes and next steps for a freshly deployed app. */
export function TemplateAccessCard({
  app,
  templateId,
  onNavigate,
}: TemplateAccessCardProps) {
  const { t } = useTranslation('templates')
  const urls = useAppUrls(app.name)
  const settings = useEnvSettings(app.name)
  const template = useServiceTemplate(templateId ?? '')
  const revealed = useRevealedValues()
  const showError = useCallback(
    (error: unknown) => {
      const denied =
        error instanceof EnvApiError &&
        (error.status === 403 || error.code === 'reveal_disabled')
      toast.add({
        title: denied
          ? t('result.revealDenied')
          : error instanceof Error
            ? error.message
            : String(error),
        type: 'error',
      })
    },
    [t],
  )
  const gate = useStepUpGate(showError)

  const policy = settings.data
  const canReveal =
    policy !== undefined &&
    policy.policy_valid &&
    policy.reveal_policy !== 'disabled' &&
    !(policy.secrets_write_only ?? app.secrets_write_only ?? false)

  const address =
    (app.domains ?? [])[0] !== undefined
      ? `https://${(app.domains ?? [])[0]}`
      : directUrl(urls.data)
  const keys = app.secret_env ?? []

  const reveal = (key: string) => {
    void gate.guard(
      { action: 'reveal', app: app.name },
      async () => {
        const res = await revealValue(app.name, key, true)
        revealed.reveal(key, res.value, res.hide_after_seconds)
      },
      key,
    )
  }
  const copy = (value: string) => {
    void navigator.clipboard.writeText(value).then(() => {
      toast.add({ title: t('result.copied'), type: 'success' })
    })
  }

  return (
    <div className="flex flex-col gap-3 text-sm">
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="font-medium">{t('result.address')}</span>
        {address ? (
          <a
            href={address}
            target="_blank"
            rel="noreferrer noopener"
            className="min-w-0 truncate font-mono text-xs text-primary underline-offset-2 hover:underline"
          >
            {address}
          </a>
        ) : (
          <span className="text-xs text-muted-foreground">
            {t('result.addressPending')}
          </span>
        )}
      </div>

      <section aria-labelledby="tpl-creds" className="flex flex-col gap-1.5">
        <h3 id="tpl-creds" className="flex items-center gap-1 font-medium">
          <KeyIcon className="size-3.5" aria-hidden="true" />
          {t('result.credentials')}
        </h3>
        {keys.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('result.noCredentials')}
          </p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">
              {t('result.credentialsHint')}
            </p>
            <ul className="flex flex-col gap-1.5">
              {keys.map((key) => (
                <TemplateCredentialRow
                  key={key}
                  secretKey={key}
                  value={revealed.values[key]}
                  canReveal={canReveal}
                  onReveal={() => {
                    reveal(key)
                  }}
                  onHide={() => {
                    revealed.hide(key)
                  }}
                  onCopy={copy}
                />
              ))}
            </ul>
            {!canReveal && !settings.isPending ? (
              <p className="text-xs text-muted-foreground">
                {t('result.revealDenied')}
              </p>
            ) : null}
          </>
        )}
      </section>

      <TemplateDescription
        description={template.data?.description}
        slogan={template.data?.slogan}
      />

      <div className="flex flex-wrap items-center gap-2">
        <Link
          to="/apps/$name"
          params={{ name: app.name }}
          onClick={onNavigate}
          className={buttonVariants({ size: 'sm' })}
        >
          <ArrowSquareOutIcon aria-hidden="true" />
          {t('result.openApp')}
        </Link>
        <Link
          to="/apps/$name/logs"
          params={{ name: app.name }}
          onClick={onNavigate}
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          <ScrollIcon aria-hidden="true" />
          {t('result.viewLogs')}
        </Link>
        <Link
          to="/apps/$name/environment"
          params={{ name: app.name }}
          onClick={onNavigate}
          className={buttonVariants({ variant: 'ghost', size: 'sm' })}
        >
          {t('result.openEnvironment')}
        </Link>
      </div>

      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        subject={gate.pending?.subject}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </div>
  )
}
