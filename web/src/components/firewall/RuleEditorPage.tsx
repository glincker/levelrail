import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { RuleEditor } from './RuleEditor'
import type { RuleSeed } from './useFirewallActions'

/** RuleEditorPage is the full page around RuleEditor: back link to all rules, title and the builder. */
export function RuleEditorPage({
  ruleId,
  seed,
  scope,
  onDone,
}: {
  ruleId?: string
  seed?: RuleSeed
  scope: string
  onDone: () => void
}) {
  const { t } = useTranslation('firewall')
  return (
    <PageTemplate
      icon={<ShieldCheckGlassIcon className="text-tone-success" size="1.2em" />}
      title={ruleId ? t('editor.editTitle') : t('editor.newTitle')}
      description={t('editor.description')}
      breadcrumb={
        <Link
          to="/firewall/rules"
          search={{ scope: scope || undefined }}
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftGlassIcon aria-hidden="true" className="size-4" />
          {t('editor.back')}
        </Link>
      }
    >
      <RuleEditor
        ruleId={ruleId}
        seed={seed}
        initialScope={scope}
        onDone={onDone}
      />
    </PageTemplate>
  )
}
