import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type {
  ExternalEngine,
  ExternalProbeResult,
} from '../types/externalDatabase'
import { ExternalHealthBadge } from './ExternalHealthBadge'
import { ChoiceButton } from './ChoiceButton'
import {
  ENGINES,
  TLS_LABEL,
  TLS_MODES,
  slug,
  type FormState,
} from './connectDatabaseForm'

interface Props {
  form: FormState
  adopting: boolean
  probe: ExternalProbeResult | null
  testing: boolean
  testError: string | null
  saveError: string | null
  patch: (p: Partial<FormState>) => void
  setEngine: (e: ExternalEngine) => void
  setDatabase: (d: string) => void
  runTest: () => void
}

// Details step of ConnectDatabaseDialog: the address, credentials, TLS and
// the inline connection test.
export function ConnectDatabaseDetails({
  form,
  adopting,
  probe,
  testing,
  testError,
  saveError,
  patch,
  setEngine,
  setDatabase,
  runTest,
}: Props) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-3">
      {adopting ? (
        <p className="flex items-start gap-2 rounded-md bg-muted/60 p-2 text-xs text-muted-foreground">
          <ShieldCheckIcon
            className="mt-0.5 size-4 shrink-0"
            aria-hidden="true"
          />
          {t('external.connect.adoptSafe')}
        </p>
      ) : null}

      <div className="space-y-1">
        <Label htmlFor="ext-name">{t('external.connect.fields.name')}</Label>
        <Input
          id="ext-name"
          value={form.name}
          onChange={(e) => {
            patch({ name: slug(e.target.value) })
          }}
          placeholder="legacy-postgres"
        />
        <p className="text-xs text-muted-foreground">
          {t('external.connect.fields.nameHint', {
            example: `${form.name || 'legacy-postgres'}.url`,
          })}
        </p>
      </div>

      {!adopting ? (
        <div className="space-y-1">
          <Label>{t('external.connect.fields.engine')}</Label>
          <div className="flex flex-wrap gap-1.5">
            {ENGINES.map((e) => (
              <ChoiceButton
                key={e}
                selected={form.engine === e}
                onClick={() => {
                  setEngine(e)
                }}
              >
                {e}
              </ChoiceButton>
            ))}
          </div>
        </div>
      ) : null}

      <div className="grid grid-cols-[1fr_6rem] gap-2">
        <div className="space-y-1">
          <Label htmlFor="ext-host">{t('external.connect.fields.host')}</Label>
          <Input
            id="ext-host"
            value={form.host}
            onChange={(e) => {
              patch({ host: e.target.value })
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="ext-port">{t('external.connect.fields.port')}</Label>
          <Input
            id="ext-port"
            inputMode="numeric"
            value={form.port}
            onChange={(e) => {
              patch({ port: e.target.value.replace(/\D/g, '') })
            }}
          />
        </div>
      </div>
      <p className="-mt-2 text-xs text-muted-foreground">
        {t('external.connect.fields.hostHint')}
      </p>

      <div className="space-y-1">
        <Label htmlFor="ext-network">
          {t('external.connect.fields.network')}
        </Label>
        <Input
          id="ext-network"
          value={form.network}
          onChange={(e) => {
            patch({ network: e.target.value })
          }}
        />
        <p className="text-xs text-muted-foreground">
          {t('external.connect.fields.networkHint')}
        </p>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <div className="space-y-1">
          <Label htmlFor="ext-user">{t('external.connect.fields.user')}</Label>
          <Input
            id="ext-user"
            autoComplete="off"
            value={form.username}
            onChange={(e) => {
              patch({ username: e.target.value })
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="ext-password">
            {t('external.connect.fields.password')}
          </Label>
          <Input
            id="ext-password"
            type="password"
            autoComplete="new-password"
            value={form.password}
            onChange={(e) => {
              patch({ password: e.target.value })
            }}
          />
        </div>
      </div>
      <p className="-mt-2 text-xs text-muted-foreground">
        {t('external.connect.fields.passwordHint')}
      </p>

      {form.engine !== 'redis' ? (
        <div className="space-y-1">
          <Label htmlFor="ext-database">
            {t('external.connect.fields.database')}
          </Label>
          <Input
            id="ext-database"
            value={form.database}
            onChange={(e) => {
              patch({ database: e.target.value })
            }}
          />
        </div>
      ) : null}

      <div className="space-y-1">
        <Label>{t('external.connect.fields.tls')}</Label>
        <div className="flex gap-1.5">
          {TLS_MODES.map((m) => (
            <ChoiceButton
              key={m}
              selected={form.tls === m}
              onClick={() => {
                patch({ tls: m })
              }}
            >
              {t(TLS_LABEL[m])}
            </ChoiceButton>
          ))}
        </div>
      </div>

      <div className="space-y-2 rounded-lg border border-border p-3">
        <div className="flex items-center justify-between gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={form.host === '' || testing}
            onClick={runTest}
          >
            {testing
              ? t('external.connect.test.running')
              : t('external.connect.test.button')}
          </Button>
          {probe ? <ExternalHealthBadge status={probe.status} /> : null}
        </div>
        {testError !== null ? (
          <p className="text-sm text-destructive" role="alert">
            {testError}
          </p>
        ) : null}
        {probe ? (
          <div className="space-y-1 text-sm">
            <p className="flex items-start gap-1.5 text-muted-foreground">
              {probe.status === 'reachable' ? (
                <CheckCircleIcon
                  className="mt-0.5 size-4 shrink-0 text-success"
                  aria-hidden="true"
                />
              ) : (
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0"
                  aria-hidden="true"
                />
              )}
              {t(`external.health.next.${probe.status}`)}
            </p>
            {probe.reason ? (
              <p className="font-mono text-xs break-words text-muted-foreground">
                {probe.reason}
              </p>
            ) : null}
            {probe.version ? (
              <p className="text-xs text-muted-foreground">
                {t('external.connect.test.server', {
                  version: probe.version,
                })}
              </p>
            ) : null}
            {probe.databases && probe.databases.length > 0 ? (
              <div className="space-y-1">
                <p className="text-xs text-muted-foreground">
                  {t('external.connect.test.discovered')}
                </p>
                <div className="flex flex-wrap gap-1.5">
                  {probe.databases.map((d) => (
                    <ChoiceButton
                      key={d}
                      selected={form.database === d}
                      onClick={() => {
                        setDatabase(d)
                      }}
                    >
                      {d}
                    </ChoiceButton>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>

      {saveError ? (
        <p className="text-sm text-destructive" role="alert">
          {saveError}
        </p>
      ) : null}
    </div>
  )
}
