import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import type { PolicyResource } from '../../queries/iamPolicies'
import type { IamPrincipal } from '../../queries/iamBuilder'
import { VirtualRows } from './VirtualRows'

/** PolicyList is the scannable policy index: rule mix, how many principals each applies to, and when it last changed. */
export function PolicyList({
  policies,
  principals,
  onOpen,
  onCreate,
}: {
  policies: PolicyResource[]
  principals: IamPrincipal[]
  onOpen: (p: PolicyResource) => void
  onCreate: () => void
}) {
  const { t } = useTranslation('iam')
  if (policies.length === 0) {
    return (
      <EmptyState
        icon={<ShieldCheckIcon className="size-5" />}
        title={t('list.empty.title')}
        description={t('list.empty.description')}
        action={
          <Button size="sm" onClick={onCreate}>
            {t('page.createPolicy')}
          </Button>
        }
      />
    )
  }
  const appliesCount = (id: string) =>
    principals.filter((p) => p.policy_ids.includes(id)).length

  return (
    <VirtualRows
      items={policies}
      label={t('page.tabs.policies')}
      rowKey={(p) => p.id}
      maxHeightClass="max-h-[36rem]"
      renderRow={(p) => {
        const allow = p.document.Statement.filter(
          (s) => s.Effect === 'Allow',
        ).length
        const deny = p.document.Statement.length - allow
        const n = appliesCount(p.id)
        return (
          <div className="flex h-14 items-center gap-3 px-3">
            <button
              type="button"
              onClick={() => onOpen(p)}
              className="min-w-0 flex-1 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
            >
              <span className="block truncate text-sm font-medium text-foreground">
                {p.name}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {p.description || t('list.rules', { count: allow + deny })}
              </span>
            </button>
            <span className="hidden shrink-0 text-xs text-muted-foreground sm:block">
              {t('list.allow', { count: allow })},{' '}
              {t('list.deny', { count: deny })}
            </span>
            <span className="hidden w-28 shrink-0 text-xs text-muted-foreground md:block">
              {n === 0
                ? t('list.notAttached')
                : t('list.appliesToCount', { count: n })}
            </span>
            <span className="hidden w-32 shrink-0 text-xs text-muted-foreground lg:block">
              {new Date(p.updated_at).toLocaleDateString()}
            </span>
            <Button size="sm" variant="outline" onClick={() => onOpen(p)}>
              {t('list.open')}
            </Button>
          </div>
        )
      }}
    />
  )
}
