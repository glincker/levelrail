import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ecrLoginCommand, messageOf } from '../../lib/cloudAwsServices'
import {
  useAwsAcm,
  useAwsAudit,
  useAwsEcr,
  useAwsSecrets,
  useCreateEcrRepo,
  useEcrLoginToken,
  usePutAwsSecret,
  useRequestAcmCert,
} from '../../queries/cloudAwsServices'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import { CopyButton, StateLine } from './AwsServiceShared'

export function EcrBody({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const repos = useAwsEcr(id, true)
  const create = useCreateEcrRepo(id)
  const login = useEcrLoginToken(id)
  const [name, setName] = useState('')
  return (
    <div className="space-y-2">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(name.trim(), {
            onSuccess: () => {
              setName('')
            },
          })
        }}
      >
        <Input
          className="max-w-xs"
          value={name}
          placeholder={t('services.ecr.namePlaceholder')}
          aria-label={t('services.ecr.name')}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        <Button
          size="sm"
          type="submit"
          disabled={name.trim() === '' || create.isPending}
        >
          {t('services.ecr.create')}
        </Button>
        <CopyButton
          label={t('services.ecr.copyLogin')}
          getText={async () => ecrLoginCommand(await login.mutateAsync())}
        />
      </form>
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(create.error)}
        </p>
      ) : null}
      {login.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(login.error)}
        </p>
      ) : null}
      <StateLine
        pending={repos.isPending}
        error={repos.error}
        empty={repos.data?.length === 0 ? t('services.ecr.empty') : undefined}
      />
      {repos.data && repos.data.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('services.ecr.name')}</TableHead>
              <TableHead>{t('services.ecr.uri')}</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {repos.data.map((r) => (
              <TableRow key={r.arn}>
                <TableCell>{r.name}</TableCell>
                <TableCell className="font-mono text-xs">{r.uri}</TableCell>
                <TableCell>
                  <CopyButton label={t('services.copy')} text={r.uri} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
    </div>
  )
}

export function AcmBody({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const certs = useAwsAcm(id, true)
  const request = useRequestAcmCert(id)
  const [domain, setDomain] = useState('')
  return (
    <div className="space-y-2">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          request.mutate(domain.trim(), {
            onSuccess: () => {
              setDomain('')
            },
          })
        }}
      >
        <Input
          className="max-w-xs"
          value={domain}
          placeholder={t('services.acm.domainPlaceholder')}
          aria-label={t('services.acm.domain')}
          onChange={(e) => {
            setDomain(e.target.value)
          }}
        />
        <Button
          size="sm"
          type="submit"
          disabled={domain.trim() === '' || request.isPending}
        >
          {t('services.acm.request')}
        </Button>
      </form>
      {request.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(request.error)}
        </p>
      ) : null}
      <StateLine
        pending={certs.isPending}
        error={certs.error}
        empty={certs.data?.length === 0 ? t('services.acm.empty') : undefined}
      />
      {(certs.data ?? []).map((c) => (
        <div key={c.arn} className="space-y-1 rounded-md bg-muted/40 p-2">
          <p className="text-sm">
            <span className="font-medium">{c.domain}</span>{' '}
            <span className="text-xs text-muted-foreground">{c.status}</span>
          </p>
          {(c.records ?? []).map((r) => (
            <div
              key={`${r.name}-${r.value}`}
              className="flex flex-wrap items-center gap-2 text-xs"
            >
              <span className="font-mono">{r.type}</span>
              <span className="max-w-64 truncate font-mono" title={r.name}>
                {r.name}
              </span>
              <CopyButton label={t('services.acm.copyName')} text={r.name} />
              <CopyButton label={t('services.acm.copyValue')} text={r.value} />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

export function SecretsBody({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const list = useAwsSecrets(id, true)
  const put = usePutAwsSecret(id)
  const [typedName, setTypedName] = useState<string | null>(null)
  const [value, setValue] = useState('')
  const [overwrite, setOverwrite] = useState(false)
  // Until the operator types, the input shows the server's name prefix so names land in scope.
  const prefix = list.data?.name_prefix ? `/${list.data.name_prefix}/` : ''
  const name = typedName ?? prefix
  const rows = [
    ...(list.data?.ssm_parameters ?? []),
    ...(list.data?.secrets_manager ?? []),
  ]
  return (
    <div className="space-y-2">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          put.mutate(
            { name: name.trim(), value, overwrite },
            {
              onSuccess: () => {
                setValue('')
              },
            },
          )
        }}
      >
        <Input
          className="max-w-xs"
          value={name}
          placeholder={t('services.secrets.namePlaceholder')}
          aria-label={t('services.secrets.name')}
          onChange={(e) => {
            setTypedName(e.target.value)
          }}
        />
        <Input
          className="max-w-xs"
          type="password"
          autoComplete="off"
          value={value}
          aria-label={t('services.secrets.value')}
          placeholder={t('services.secrets.value')}
          onChange={(e) => {
            setValue(e.target.value)
          }}
        />
        <label className="flex items-center gap-1.5 text-xs">
          <Checkbox
            checked={overwrite}
            onCheckedChange={(c) => {
              setOverwrite(c === true)
            }}
          />
          {t('services.secrets.overwrite')}
        </label>
        <Button
          size="sm"
          type="submit"
          disabled={!name.startsWith('/') || value === '' || put.isPending}
        >
          {t('services.secrets.put')}
        </Button>
      </form>
      {put.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(put.error)}
        </p>
      ) : null}
      <StateLine
        pending={list.isPending}
        error={list.error}
        empty={
          rows.length === 0 && list.data
            ? t('services.secrets.empty')
            : undefined
        }
      />
      {rows.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('services.secrets.name')}</TableHead>
              <TableHead>{t('services.secrets.value')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={`${r.arn ?? ''}${r.name}`}>
                <TableCell className="font-mono text-xs">{r.name}</TableCell>
                <TableCell className="text-muted-foreground">
                  {t('services.secrets.redacted')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
    </div>
  )
}

export function AuditBody({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const [draft, setDraft] = useState('')
  const [resource, setResource] = useState('')
  const events = useAwsAudit(id, resource, true)
  return (
    <div className="space-y-2">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setResource(draft.trim())
        }}
      >
        <Input
          className="max-w-xs"
          value={draft}
          aria-label={t('services.audit.resource')}
          placeholder={t('services.audit.resource')}
          onChange={(e) => {
            setDraft(e.target.value)
          }}
        />
        <Button size="sm" type="submit" disabled={draft.trim() === ''}>
          {t('services.audit.search')}
        </Button>
      </form>
      {resource === '' ? (
        <p className="text-sm text-muted-foreground">
          {t('services.audit.hint')}
        </p>
      ) : (
        <StateLine
          pending={events.isPending}
          error={events.error}
          empty={
            events.data?.length === 0 ? t('services.audit.empty') : undefined
          }
        />
      )}
      {events.data && events.data.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('services.audit.time')}</TableHead>
              <TableHead>{t('services.audit.event')}</TableHead>
              <TableHead>{t('services.audit.user')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.data.map((e) => (
              <TableRow key={e.id}>
                <TableCell className="text-xs">
                  {formatAbsoluteTime(e.time)}
                </TableCell>
                <TableCell>
                  {e.name}{' '}
                  <span className="text-xs text-muted-foreground">
                    {e.source}
                  </span>
                </TableCell>
                <TableCell className="text-xs">{e.username}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
    </div>
  )
}
