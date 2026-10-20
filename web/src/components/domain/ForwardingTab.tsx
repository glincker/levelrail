import { useTranslation } from 'react-i18next'
import { useAppListOptional } from '../../queries/apps'
import { ForwarderEditor } from './ForwarderEditor'
import { PreviewControls } from './PreviewControls'
import { SaveBar, Section } from './PolicyShared'
import { usePolicyEditor } from './usePolicyEditor'
import type {
  DomainPolicies,
  ForwardersSpec,
} from '../../queries/domainPolicyTypes'

const EMPTY: ForwardersSpec = { rules: [] }

export function ForwardingTab({
  app,
  domain,
  policies,
}: {
  app: string
  domain: string
  policies: DomainPolicies
}) {
  const { t } = useTranslation('domainPolicies')
  const saved = policies.policy.forwarders ?? EMPTY
  const ed = usePolicyEditor(app, domain, 'forwarders', saved)
  const { data: apps } = useAppListOptional()
  const appNames = (apps ?? []).map((a) => a.name)

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="space-y-4">
        <Section
          title={t('forwarders.title')}
          description={t('forwarders.help', { app })}
        >
          <ForwarderEditor
            rules={ed.draft.rules}
            onChange={(rules) => ed.setDraft({ rules })}
            errors={ed.errors}
            apps={appNames}
            max={policies.limits.max_forwarders}
          />
        </Section>
        <SaveBar
          saved={saved}
          draft={ed.draft}
          dirty={ed.dirty}
          saving={ed.save.isPending}
          error={ed.save.error?.message}
          onSave={ed.onSave}
          onDiscard={ed.discard}
        />
      </div>
      <PreviewControls
        id="forwarders-preview"
        method={ed.method}
        path={ed.path}
        onMethod={ed.setMethod}
        onPath={ed.setPath}
        data={ed.preview.data}
        loading={ed.preview.isFetching}
      />
    </div>
  )
}
