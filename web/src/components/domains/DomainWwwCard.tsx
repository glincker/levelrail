import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CopyGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ApiError } from '../../lib/apiError'
import {
  domainWwwPlanQueryOptions,
  useApplyDomainWww,
  type WwwApplyResult,
  type WwwChoice,
  type WwwPlanRecord,
} from '../../queries/domainWww'

type ChoiceId = 'apex' | 'www' | 'both' | 'apexOnly'

const CHOICES: Record<ChoiceId, WwwChoice> = {
  apex: { mode: 'both', canonical: 'apex' },
  www: { mode: 'both', canonical: 'www' },
  both: { mode: 'both', canonical: 'apex' },
  apexOnly: { mode: 'apex_only', canonical: 'apex' },
}
const CHOICE_ORDER: ChoiceId[] = ['apex', 'www', 'both', 'apexOnly']

// Subdomains and automatic IP hostnames have no www twin to manage.
function wwwDoesNotApply(domain: string): boolean {
  const d = domain.toLowerCase()
  return (
    d.startsWith('www.') ||
    /^\d{1,3}(\.\d{1,3}){3}$/.test(d) ||
    d === 'localhost' ||
    d.endsWith('.localhost') ||
    d.endsWith('.sslip.io') ||
    d.endsWith('.nip.io')
  )
}

function CopyButton({ value }: { value: string }) {
  const { t } = useTranslation('domains')
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={copied ? t('www.copied') : t('www.copy', { value })}
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => setCopied(true))
      }}
    >
      {copied ? (
        <CircleCheckGlassIcon size="1.2em" className="text-tone-success" />
      ) : (
        <CopyGlassIcon size="1.2em" />
      )}
    </Button>
  )
}

function RecordRows({ records }: { records: WwwPlanRecord[] }) {
  const { t } = useTranslation('domains')
  return (
    <ul className="divide-y divide-border rounded-md border text-sm">
      {records.map((r) => {
        const current = r.replaces.find((x) => x.value !== r.value)
        return (
          <li
            key={`${r.type}-${r.name}`}
            className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2"
          >
            <span className="w-14 font-mono text-xs text-muted-foreground">
              {r.type}
            </span>
            <span className="w-16 font-mono">{r.name}</span>
            <span className="min-w-0 flex-1 break-all font-mono">
              {r.value}
              {current ? (
                <span className="block text-xs text-muted-foreground">
                  {t('www.col.now')}: {current.value}
                </span>
              ) : null}
            </span>
            <span className="text-xs text-muted-foreground">
              {r.state
                ? t(`www.state.${r.state}`)
                : t(`www.outcome.${r.outcome}`)}
            </span>
            <CopyButton value={r.value} />
          </li>
        )
      })}
    </ul>
  )
}

function ConnectRecommendation({
  domain,
  pointsHere,
  cnameValue,
  pending,
  checking,
  onConnect,
  onCheck,
  onOther,
}: {
  domain: string
  pointsHere: boolean
  cnameValue: string
  pending: boolean
  checking: boolean
  onConnect: () => void
  onCheck: () => void
  onOther: () => void
}) {
  const { t } = useTranslation('domains')
  return (
    <div className="space-y-2 rounded-md border border-primary/40 bg-muted/40 p-3">
      <p className="text-sm">
        {pointsHere
          ? t('www.connectPointsHere', { domain })
          : t('www.connectNeedsDns', { domain })}
      </p>
      {pointsHere ? null : (
        <p className="flex items-center gap-2 font-mono text-sm">
          <span>CNAME www</span>
          <span className="break-all">{cnameValue}</span>
          <CopyButton value={cnameValue} />
        </p>
      )}
      <div className="flex flex-wrap items-center gap-3">
        {pointsHere ? (
          <Button
            type="button"
            size="sm"
            disabled={pending}
            onClick={onConnect}
          >
            {pending ? t('www.connecting') : t('www.connectButton')}
          </Button>
        ) : (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={checking}
            onClick={onCheck}
          >
            {t('www.retry')}
          </Button>
        )}
        <Button
          type="button"
          variant="link"
          size="sm"
          className="h-auto p-0 text-muted-foreground"
          onClick={onOther}
        >
          {t('www.otherOptions')}
        </Button>
      </div>
    </div>
  )
}

function ApplyResult({ result }: { result: WwwApplyResult }) {
  const { t } = useTranslation('domains')
  return (
    <div className="space-y-2" role="status">
      <p className="flex items-center gap-2 text-sm font-medium">
        {result.applied ? (
          <CircleCheckGlassIcon size="1.2em" className="text-tone-success" />
        ) : (
          <CircleAlertGlassIcon size="1.2em" className="text-tone-warning" />
        )}
        {result.applied ? t('www.applied') : t('www.partial')}
      </p>
      <ul className="space-y-1 text-sm">
        {result.records.map((r) => (
          <li key={`${r.type}-${r.name}`} className="flex flex-wrap gap-2">
            <span className="font-mono">
              {r.type} {r.name}
            </span>
            <span
              className={
                r.result === 'failed'
                  ? 'text-destructive'
                  : 'text-muted-foreground'
              }
            >
              {t(`www.result.${r.result}`, { defaultValue: r.result })}
            </span>
            {r.message ? (
              <span className="text-muted-foreground">{r.message}</span>
            ) : null}
          </li>
        ))}
      </ul>
      {result.redirect_applied ? (
        <p className="text-sm text-muted-foreground">
          {t('www.redirectApplied')}
        </p>
      ) : null}
    </div>
  )
}

export function DomainWwwCard({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const groupId = useId()
  const [choiceId, setChoiceId] = useState<ChoiceId>('apex')
  const [showOptions, setShowOptions] = useState(false)
  const hidden = wwwDoesNotApply(domain)
  const choice = CHOICES[choiceId]
  const plan = useQuery({
    ...domainWwwPlanQueryOptions(appName, domain, choice),
    enabled: !hidden,
  })
  const apply = useApplyDomainWww(appName, domain)

  // The server rejects subdomains with a 400, so the card steps aside.
  if (hidden || (plan.error instanceof ApiError && plan.error.status === 400)) {
    return null
  }

  const data = plan.data
  const hasProvider = Boolean(data?.provider)
  const hasConflict = data?.records.some((r) => r.outcome === 'conflict')
  const showRedirect = choiceId !== 'both' && data?.redirect
  const recommend = data?.www_attached === false && !showOptions
  const connected = data?.www_attached === true && showOptions === false
  const cnameValue =
    data?.records.find((r) => r.name === 'www')?.value ?? domain

  return (
    <section
      aria-labelledby={`${groupId}-title`}
      className="space-y-3 rounded-lg border bg-card p-3 text-card-foreground"
    >
      <div>
        <h3 id={`${groupId}-title`} className="text-sm font-medium">
          {t('www.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('www.help', { domain })}
        </p>
      </div>

      {recommend && data ? (
        <ConnectRecommendation
          domain={domain}
          pointsHere={data.www_points_here === true}
          cnameValue={cnameValue}
          pending={apply.isPending}
          checking={plan.isFetching}
          onConnect={() =>
            apply.mutate({ ...choice, replace: false, attach: true })
          }
          onCheck={() => void plan.refetch()}
          onOther={() => setShowOptions(true)}
        />
      ) : connected && data?.www_redirects ? (
        <p className="flex items-center gap-2 text-sm text-tone-success">
          <CircleCheckGlassIcon size="1.2em" />
          {t('www.connectedRedirect', { domain })}
        </p>
      ) : null}
      {recommend && apply.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('www.applyError', { message: apply.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      {recommend && apply.data ? <ApplyResult result={apply.data} /> : null}

      <fieldset className={recommend ? 'hidden' : 'space-y-1'}>
        <legend className="sr-only">{t('www.choiceLabel')}</legend>
        {CHOICE_ORDER.map((id) => (
          <label key={id} className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name={`${groupId}-choice`}
              checked={choiceId === id}
              onChange={() => {
                setChoiceId(id)
                apply.reset()
              }}
              className="size-4 accent-primary"
            />
            {t(`www.choice.${id}`, { domain })}
          </label>
        ))}
      </fieldset>

      {plan.isPending ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t('www.loading')}
        </p>
      ) : plan.isError ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-2">
            {t('www.error', { message: plan.error.message })}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void plan.refetch()}
            >
              {t('www.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : data && recommend ? null : data ? (
        <div className="space-y-2">
          <RecordRows records={data.records} />
          {showRedirect ? (
            <p className="text-xs text-muted-foreground">
              {t('www.redirect', {
                from: data.redirect?.from,
                to: data.redirect?.to,
              })}
            </p>
          ) : null}
          {!hasProvider ? (
            <p className="text-sm text-muted-foreground">
              {t('www.noProvider')}
            </p>
          ) : null}
          {hasConflict ? (
            <Alert>
              <AlertDescription>{t('www.replaceWarning')}</AlertDescription>
            </Alert>
          ) : null}
          {apply.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('www.applyError', { message: apply.error.message })}
              </AlertDescription>
            </Alert>
          ) : null}
          {apply.data ? <ApplyResult result={apply.data} /> : null}
          <div className="flex flex-wrap gap-2">
            {hasProvider ? (
              <Button
                type="button"
                size="sm"
                disabled={apply.isPending}
                onClick={() =>
                  apply.mutate({
                    ...choice,
                    replace: Boolean(hasConflict),
                    ...(choiceId === 'both' ? { redirect: false } : {}),
                  })
                }
              >
                {apply.isPending
                  ? t('www.applying')
                  : hasConflict
                    ? t('www.applyReplace')
                    : t('www.apply')}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={plan.isFetching}
              onClick={() => void plan.refetch()}
            >
              {t('www.retry')}
            </Button>
          </div>
        </div>
      ) : null}
    </section>
  )
}
