import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CopyGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type {
  ChecklistAction,
  ChecklistStep,
  StepStatus,
} from '../../queries/goLiveChecklist'
import { STATUS_TONE } from './liveState'

export function StatusIcon({ status }: { status: StepStatus }) {
  const cls = `mt-0.5 size-5 shrink-0 ${STATUS_TONE[status]}`
  switch (status) {
    case 'ok':
      return (
        <CircleCheckGlassIcon size="1.2em" className={cls} aria-hidden="true" />
      )
    case 'fail':
      return (
        <CircleAlertGlassIcon size="1.2em" className={cls} aria-hidden="true" />
      )
    case 'warn':
      return (
        <TriangleAlertGlassIcon
          size="1.2em"
          className={cls}
          aria-hidden="true"
        />
      )
    case 'pending':
      return (
        <LoaderCircleGlassIcon
          size="1.2em"
          className={cls}
          aria-hidden="true"
        />
      )
    default:
      return (
        <CircleMinusGlassIcon size="1.2em" className={cls} aria-hidden="true" />
      )
  }
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation('golive')
  const { copied, copy } = useCopyToClipboard()
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={copied ? t('copied') : t('copyValue', { label })}
      onClick={() => {
        copy(value)
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.2em" aria-hidden="true" />
      ) : (
        <CopyGlassIcon size="1.2em" aria-hidden="true" />
      )}
    </Button>
  )
}

function PrimaryButton({
  action,
  pending,
  onAction,
}: {
  action: ChecklistAction
  pending: boolean
  onAction: (a: ChecklistAction) => void
}) {
  const { t } = useTranslation('golive')
  const router = useRouter()
  const { copied, copy } = useCopyToClipboard()
  const variant = action.kind === 'recheck' ? 'outline' : 'default'
  if (action.kind === 'link' && action.href) {
    const href = action.href
    const external = /^https?:\/\//.test(href)
    return (
      <Button
        size="sm"
        variant={variant}
        nativeButton={false}
        render={
          <a
            href={href}
            {...(external
              ? { target: '_blank', rel: 'noreferrer' }
              : {
                  onClick: (e: { preventDefault: () => void }) => {
                    e.preventDefault()
                    void router.navigate({ href })
                  },
                })}
          />
        }
      >
        {action.label}
      </Button>
    )
  }
  if (action.kind === 'copy') {
    return (
      <Button
        size="sm"
        variant={variant}
        onClick={() => {
          copy(action.value ?? '')
        }}
      >
        {copied ? t('copied') : action.label}
      </Button>
    )
  }
  return (
    <span className="flex flex-col items-start gap-1">
      <Button
        size="sm"
        variant={variant}
        disabled={pending || Boolean(action.blocked)}
        onClick={() => {
          onAction(action)
        }}
      >
        {action.label}
      </Button>
      {action.blocked ? (
        <span className="text-xs text-muted-foreground">
          {t('blocked', { reason: action.blocked })}
        </span>
      ) : null}
    </span>
  )
}

function Provider({ step }: { step: ChecklistStep }) {
  const { t } = useTranslation('golive')
  const p = step.provider
  if (!p) return null
  return (
    <Collapsible className="rounded-md border border-border p-2">
      <CollapsibleTrigger className="text-sm font-medium">
        {t('provider.title', { name: p.name })}
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-2 text-sm">
        <ol className="list-decimal space-y-1 pl-5">
          {p.steps.map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ol>
        <p className="font-medium">{t('provider.ports')}</p>
        <ul className="list-disc pl-5">
          {p.ports.map((r) => (
            <li key={`${r.port}/${r.protocol}`}>
              {t('provider.portRule', {
                port: r.port,
                protocol: r.protocol,
                source: r.source,
              })}
            </li>
          ))}
        </ul>
        {p.note ? <p className="text-muted-foreground">{p.note}</p> : null}
        <a
          href={p.docs_url}
          target="_blank"
          rel="noreferrer"
          className="text-primary underline underline-offset-2"
        >
          {t('provider.docs', { name: p.name })}
        </a>
      </CollapsiblePanel>
    </Collapsible>
  )
}

export function StepRow({
  step,
  index,
  pending,
  onAction,
}: {
  step: ChecklistStep
  index: number
  pending: boolean
  onAction: (a: ChecklistAction, step: ChecklistStep) => void
}) {
  const { t } = useTranslation('golive')
  return (
    <li
      className="flex gap-3 rounded-lg border border-border p-4"
      data-step={step.id}
      data-status={step.status}
    >
      <StatusIcon status={step.status} />
      <div className="min-w-0 flex-1 space-y-2">
        <h3 className="text-sm font-semibold">
          {index + 1}. {step.title}
          <span className="sr-only">
            {' ('}
            {t(`status.${step.status}`)}
            {')'}
          </span>
        </h3>
        <p className="text-sm text-muted-foreground">{step.summary}</p>
        {step.evidence.length > 0 ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
            {step.evidence
              .filter((e) => e.value)
              .map((e) => (
                <div key={`${e.label}:${e.value}`} className="contents">
                  <dt className="text-muted-foreground">{e.label}</dt>
                  <dd className="flex min-w-0 items-center gap-1 break-all">
                    {e.value}
                    {e.copy ? (
                      <CopyButton value={e.value} label={e.label} />
                    ) : null}
                  </dd>
                </div>
              ))}
          </dl>
        ) : null}
        {step.scope ? (
          <p className="text-xs text-muted-foreground">
            {t(`scope.${step.scope}`, { defaultValue: step.scope })}
          </p>
        ) : null}
        {step.next_check_at ? (
          <p className="text-xs text-muted-foreground">
            {t('nextCheck', {
              time: new Date(step.next_check_at).toLocaleTimeString(),
            })}
          </p>
        ) : null}
        <Provider step={step} />
        {step.status !== 'ok' && step.primary ? (
          <PrimaryButton
            action={step.primary}
            pending={pending}
            onAction={(a) => {
              onAction(a, step)
            }}
          />
        ) : null}
      </div>
    </li>
  )
}
