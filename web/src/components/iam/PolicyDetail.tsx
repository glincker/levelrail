import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  LinkBreakIcon,
  PencilSimpleIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import { DeletePolicyDialog } from '../DeletePolicyDialog'
import type { PolicyDocument, PolicyResource } from '../../queries/iamPolicies'
import {
  iamAnalysisQueryOptions,
  iamPrincipalsQueryOptions,
  policyVersionsQueryOptions,
} from '../../queries/iamBuilder'
import type { StatementChange } from '../../queries/iamBuilder'

function StatementCard({
  s,
  change,
}: {
  s: PolicyDocument['Statement'][number]
  change?: StatementChange['change']
}) {
  const { t } = useTranslation('iam')
  const deny = s.Effect === 'Deny'
  const tone = deny ? TONE.danger : TONE.success
  return (
    <li
      className={cn(
        'rounded-lg border px-3 py-2 text-sm',
        change === 'removed' ? 'opacity-70' : '',
        tone.border,
      )}
    >
      <p className="flex flex-wrap items-center gap-2">
        <span
          className={cn(
            'rounded-full px-2 py-0.5 text-xs',
            tone.soft,
            tone.text,
          )}
        >
          {deny ? t('builder.effect.deny') : t('builder.effect.allow')}
        </span>
        {change ? (
          <Badge variant={change === 'added' ? 'success' : 'warning'}>
            {t(change === 'added' ? 'versions.added' : 'versions.removed')}
          </Badge>
        ) : null}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        <code>{s.Action.join(', ')}</code> on{' '}
        <code>{s.Resource.join(', ')}</code>
      </p>
    </li>
  )
}

function Versions({
  policyId,
  actorName,
}: {
  policyId: string
  actorName: (actor: string) => string
}) {
  const { t } = useTranslation('iam')
  const q = useQuery(policyVersionsQueryOptions(policyId))
  if (q.isPending) return <Skeleton className="h-16 w-full" />
  if (q.isError || q.data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('versions.empty')}</p>
    )
  }
  const oldest = q.data[q.data.length - 1]?.version
  return (
    <ol className="space-y-3">
      {q.data.map((v) => (
        <li key={v.version} className="space-y-1.5">
          <p className="text-xs text-muted-foreground">
            <span className="font-medium text-foreground">
              {t('versions.version', { n: v.version })}
            </span>{' '}
            {new Date(v.created_at).toLocaleString()}{' '}
            {t('versions.by', {
              actor: actorName(v.actor) || t('versions.unknownActor'),
            })}
          </p>
          {v.version === oldest ? (
            <p className="text-xs text-muted-foreground">
              {t('versions.initial')}
            </p>
          ) : v.changes.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('versions.noChange')}
            </p>
          ) : (
            <ul className="space-y-1.5">
              {v.changes.map((c, i) => (
                <StatementCard
                  key={`${c.change}:${i}`}
                  s={c.statement}
                  change={c.change}
                />
              ))}
            </ul>
          )}
        </li>
      ))}
    </ol>
  )
}

/** PolicyDetail is the policy-centric view: its rules in plain words, who it applies to, its findings, and its version history with rule diffs. */
export function PolicyDetail({
  policy,
  onClose,
  onEdit,
  onAttach,
  onDetach,
}: {
  policy: PolicyResource | undefined
  onClose: () => void
  onEdit: (p: PolicyResource) => void
  onAttach: (p: PolicyResource) => void
  onDetach: (p: PolicyResource, key: string) => void
}) {
  const { t } = useTranslation('iam')
  const principals = useQuery({
    ...iamPrincipalsQueryOptions(),
    enabled: Boolean(policy),
  })
  const analysis = useQuery({
    ...iamAnalysisQueryOptions(),
    enabled: Boolean(policy),
  })
  const appliesTo = (principals.data ?? []).filter(
    (p) => policy && p.policy_ids.includes(policy.id),
  )
  const findings = (analysis.data?.findings ?? []).filter(
    (f) => policy && f.policy_id === policy.id,
  )

  return (
    <Sheet open={Boolean(policy)} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        {policy ? (
          <>
            <SheetHeader>
              <SheetTitle>{policy.name}</SheetTitle>
              <SheetDescription>
                {policy.description || t('detail.title')}
              </SheetDescription>
            </SheetHeader>
            <div className="space-y-5 px-4 pb-6">
              <div className="flex flex-wrap gap-2">
                <Button size="sm" onClick={() => onEdit(policy)}>
                  <PencilSimpleIcon />
                  {t('list.edit')}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => onAttach(policy)}
                >
                  <PlusIcon />
                  {t('list.attach')}
                </Button>
                <DeletePolicyDialog policy={policy} />
              </div>

              <section className="space-y-2">
                <h3 className="text-sm font-medium text-foreground">
                  {t('detail.rules')}
                </h3>
                <ul className="space-y-1.5">
                  {policy.document.Statement.map((s, i) => (
                    <StatementCard key={i} s={s} />
                  ))}
                </ul>
              </section>

              <section className="space-y-2">
                <h3 className="text-sm font-medium text-foreground">
                  {t('detail.appliesTo')}
                </h3>
                {principals.isPending ? (
                  <Skeleton className="h-10 w-full" />
                ) : appliesTo.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('detail.nobody')}
                  </p>
                ) : (
                  <ul className="divide-y divide-border rounded-lg border border-border">
                    {appliesTo.map((p) => (
                      <li
                        key={`${p.principal_type}:${p.principal_id}`}
                        className="flex items-center justify-between gap-2 px-3 py-2"
                      >
                        <span className="min-w-0 truncate text-sm">
                          {p.name}{' '}
                          <span className="text-xs text-muted-foreground">
                            {t(`access.${p.principal_type}`)}
                          </span>
                        </span>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            onDetach(
                              policy,
                              `${p.principal_type}:${p.principal_id}`,
                            )
                          }
                        >
                          <LinkBreakIcon />
                          {t('detail.detach')}
                        </Button>
                      </li>
                    ))}
                  </ul>
                )}
              </section>

              <section className="space-y-2">
                <h3 className="text-sm font-medium text-foreground">
                  {t('detail.findings')}
                </h3>
                {findings.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('detail.noFindings')}
                  </p>
                ) : (
                  <ul className="space-y-1.5">
                    {findings.map((f, i) => (
                      <li
                        key={`${f.kind}:${i}`}
                        className="rounded-lg border border-border px-3 py-2 text-sm"
                      >
                        <span className="text-foreground">{f.message}</span>{' '}
                        <span className="text-muted-foreground">{f.fix}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </section>

              <section className="space-y-2">
                <h3 className="text-sm font-medium text-foreground">
                  {t('versions.title')}
                </h3>
                <Versions
                  policyId={policy.id}
                  actorName={(actor) =>
                    (principals.data ?? []).find(
                      (p) => `${p.principal_type}:${p.principal_id}` === actor,
                    )?.name ?? actor
                  }
                />
              </section>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
