import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { providerEngines, type HostedForm } from '@/lib/hostedConnect'
import type { ExternalProvider } from '../../types/externalDatabase'
import { TLS_LABEL, TLS_MODES, slug } from '../connectDatabaseForm'
import { ProviderMark } from './ProviderMark'

export type ParseState = 'idle' | 'parsing' | 'parsed' | 'failed'

interface Props {
  provider: ExternalProvider | null
  providerLabel: string
  detected: string | null
  form: HostedForm
  url: string
  parseState: ParseState
  parseError: string
  patch: (p: Partial<HostedForm>) => void
  onEngine: (engine: HostedForm['engine']) => void
  onHost: (host: string) => void
  onUrl: (url: string) => void
  onUrlCommit: (url: string) => void
  onNameEdit: () => void
}

// Step 2: paste a connection string or fill the prefilled fields.
export function HostedDetailsStep({
  provider,
  providerLabel,
  detected,
  form,
  url,
  parseState,
  parseError,
  patch,
  onEngine,
  onHost,
  onUrl,
  onUrlCommit,
  onNameEdit,
}: Readonly<Props>) {
  const { t } = useTranslation('databases')
  const engines = providerEngines(provider)
  const verifyMode = form.tls === 'verify-ca' || form.tls === 'verify-full'
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <ProviderMark
          brand={provider?.brand ?? ''}
          label={providerLabel}
          className="size-5"
        />
        {providerLabel}
        {detected ? (
          <span className="text-xs font-normal text-muted-foreground">
            {t('external.hosted.detected', { provider: detected })}
          </span>
        ) : null}
      </div>

      <div className="space-y-1">
        <Label htmlFor="hosted-url">{t('external.hosted.paste')}</Label>
        <Input
          id="hosted-url"
          type="password"
          autoComplete="off"
          spellCheck={false}
          className="font-mono"
          value={url}
          placeholder="postgres://user@host/db"
          aria-describedby="hosted-url-hint"
          onChange={(e) => {
            onUrl(e.target.value)
          }}
          onPaste={(e) => {
            onUrlCommit(e.clipboardData.getData('text'))
          }}
          onBlur={(e) => {
            onUrlCommit(e.target.value)
          }}
        />
        <p
          id="hosted-url-hint"
          role={parseState === 'failed' ? 'alert' : undefined}
          className={
            parseState === 'failed'
              ? 'text-xs text-destructive'
              : 'text-xs text-muted-foreground'
          }
        >
          {parseState === 'failed'
            ? t('external.hosted.parseError', { reason: parseError })
            : parseState === 'parsed'
              ? t('external.hosted.pasteParsed')
              : t('external.hosted.pasteHint')}
        </p>
      </div>

      <p className="text-xs font-medium text-muted-foreground">
        {t('external.hosted.orFields')}
      </p>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_6rem]">
        <div className="space-y-1">
          <Label htmlFor="hosted-name">
            {t('external.connect.fields.name')}
          </Label>
          <Input
            id="hosted-name"
            value={form.name}
            onChange={(e) => {
              onNameEdit()
              patch({ name: slug(e.target.value) })
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="hosted-host">
            {t('external.connect.fields.host')}
          </Label>
          <Input
            id="hosted-host"
            value={form.host}
            placeholder={provider?.host_hint}
            onChange={(e) => {
              onHost(e.target.value)
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="hosted-port">
            {t('external.connect.fields.port')}
          </Label>
          <Input
            id="hosted-port"
            inputMode="numeric"
            value={form.port}
            onChange={(e) => {
              patch({ port: e.target.value.replace(/\D/g, '') })
            }}
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-4">
        <div className="space-y-1">
          <Label htmlFor="hosted-engine">
            {t('external.connect.fields.engine')}
          </Label>
          <NativeSelect
            id="hosted-engine"
            className="w-full"
            value={form.engine}
            disabled={engines.length === 1}
            onChange={(e) => {
              onEngine(e.target.value as HostedForm['engine'])
            }}
          >
            {engines.map((e) => (
              <option key={e} value={e}>
                {e}
              </option>
            ))}
          </NativeSelect>
        </div>
        <div className="space-y-1">
          <Label htmlFor="hosted-user">
            {t('external.connect.fields.user')}
          </Label>
          <Input
            id="hosted-user"
            autoComplete="off"
            value={form.username}
            onChange={(e) => {
              patch({ username: e.target.value })
            }}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="hosted-password">
            {t('external.connect.fields.password')}
          </Label>
          <Input
            id="hosted-password"
            type="password"
            autoComplete="new-password"
            value={form.password}
            onChange={(e) => {
              patch({ password: e.target.value })
            }}
          />
        </div>
        {form.engine === 'redis' ? null : (
          <div className="space-y-1">
            <Label htmlFor="hosted-database">
              {t('external.connect.fields.database')}
            </Label>
            <Input
              id="hosted-database"
              value={form.database}
              onChange={(e) => {
                patch({ database: e.target.value })
              }}
            />
          </div>
        )}
      </div>
      <p className="-mt-1 text-xs text-muted-foreground">
        {t('external.connect.fields.passwordHint')}
      </p>

      <div className="space-y-1">
        <Label htmlFor="hosted-tls">{t('external.connect.fields.tls')}</Label>
        <NativeSelect
          id="hosted-tls"
          className="w-full sm:w-72"
          value={form.tls}
          aria-describedby="hosted-tls-note"
          onChange={(e) => {
            patch({ tls: e.target.value as HostedForm['tls'] })
          }}
        >
          {TLS_MODES.map((m) => (
            <option key={m} value={m}>
              {t(TLS_LABEL[m])}
            </option>
          ))}
        </NativeSelect>
        <p id="hosted-tls-note" className="text-xs text-muted-foreground">
          {t(`external.hosted.tlsExplain.${form.tls}`)}{' '}
          {verifyMode && !provider?.ca_url
            ? t('external.hosted.verifyNote')
            : null}
        </p>
      </div>

      {provider && provider.id !== 'generic' ? (
        <p className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
          <a
            className="text-state-accent underline underline-offset-2"
            href={provider.docs_url}
            target="_blank"
            rel="noopener noreferrer"
          >
            {t('external.hosted.docs', { provider: provider.label })}
          </a>
          {provider.ca_url ? (
            <a
              className="text-state-accent underline underline-offset-2"
              href={provider.ca_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('external.hosted.caLink', { provider: provider.label })}
            </a>
          ) : null}
        </p>
      ) : null}
    </div>
  )
}
