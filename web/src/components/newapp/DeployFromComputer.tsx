import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { KeyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useBrand } from '../../hooks/useBrand'
import { cliCommand } from '../../lib/cliCommand'
import { useCreateToken } from '../../queries/tokens'
import { CliCommandBox } from './CliCommandBox'

const TOKEN_DAYS = 30
const ENV_API_TOKEN = 'APP_API_TOKEN'

/**
 * Deploy a local directory with the CLI. One click mints a short-lived
 * token and shows the ready commands once; the plaintext lives only in state.
 */
export function DeployFromComputer() {
  const { t } = useTranslation('newApp')
  const brand = useBrand()
  const create = useCreateToken()
  const [token, setToken] = useState<string | null>(null)
  const deploy = cliCommand(
    brand.BinaryName,
    `deploy --api-url ${window.location.origin}`,
  )

  function mint() {
    const day = new Date().toISOString().slice(0, 10)
    create.mutate(
      {
        name: t('upload.tokenName', { date: day }),
        abilities: ['read', 'write', 'deploy'],
        expires_in_days: TOKEN_DAYS,
      },
      {
        onSuccess: (res) => {
          setToken(res.token)
        },
      },
    )
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium text-foreground">
        {t('upload.title')}
      </h3>
      <p className="text-sm text-muted-foreground">{t('upload.description')}</p>
      {token ? (
        <div className="space-y-2">
          <p className="flex items-center gap-1.5 text-xs text-amber-700 dark:text-amber-300">
            <WarningIcon className="size-3.5 shrink-0" aria-hidden="true" />
            {t('upload.tokenOnce', { days: TOKEN_DAYS })}
          </p>
          <CliCommandBox command={`export ${ENV_API_TOKEN}=${token}`} />
          <CliCommandBox command={deploy} />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setToken(null)
              create.reset()
            }}
          >
            {t('upload.hideToken')}
          </Button>
        </div>
      ) : (
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              size="sm"
              disabled={create.isPending}
              onClick={mint}
            >
              <KeyIcon aria-hidden="true" />
              {create.isPending
                ? t('upload.creating')
                : t('upload.createToken')}
            </Button>
            <span className="text-xs text-muted-foreground">
              {t('upload.tokenHint', { days: TOKEN_DAYS })}
            </span>
          </div>
          {create.isError ? (
            <p role="alert" className="text-sm text-tone-danger">
              {t('upload.createFailed', { reason: create.error.message })}{' '}
              <Link
                to="/settings/tokens"
                className="font-medium underline underline-offset-2"
              >
                {t('upload.openTokens')}
              </Link>
            </p>
          ) : null}
          <CliCommandBox command={deploy} />
        </div>
      )}
      <p className="text-xs text-muted-foreground">{t('upload.note')}</p>
    </div>
  )
}
