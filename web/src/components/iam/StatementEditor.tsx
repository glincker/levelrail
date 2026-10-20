import { useTranslation } from 'react-i18next'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import { issuesFor } from '../../lib/iamDraft'
import type { DraftStatement, Effect } from '../../lib/iamDraft'
import type {
  AbilityInfo,
  FieldIssue,
  IamResources,
} from '../../queries/iamBuilder'
import { ActionPicker } from './ActionPicker'
import { ResourcePicker } from './ResourcePicker'

function FieldIssues({ issues }: { issues: FieldIssue[] }) {
  if (issues.length === 0) return null
  return (
    <ul className="space-y-0.5" role="alert">
      {issues.map((i) => (
        <li
          key={`${i.path}:${i.message}`}
          className={cn(
            'text-xs',
            i.severity === 'error' ? 'text-destructive' : TONE.warning.text,
          )}
        >
          <code className="mr-1.5 text-muted-foreground">{i.path}</code>
          {i.message}
        </li>
      ))}
    </ul>
  )
}

/** StatementEditor is one Allow or Deny rule: effect, abilities, resources, with server field errors shown under the field they belong to. */
export function StatementEditor({
  index,
  statement,
  onChange,
  onRemove,
  canRemove,
  abilities,
  resources,
  issues,
}: {
  index: number
  statement: DraftStatement
  onChange: (next: DraftStatement) => void
  onRemove: () => void
  canRemove: boolean
  abilities: AbilityInfo[]
  resources: IamResources
  issues: FieldIssue[]
}) {
  const { t } = useTranslation('iam')
  const n = index + 1
  const effectIssues = issuesFor(issues, index, 'Effect')
  const actionIssues = issuesFor(issues, index, 'Action')
  const resourceIssues = issuesFor(issues, index, 'Resource')
  const effects: Effect[] = ['Allow', 'Deny']

  return (
    <section
      aria-label={t('builder.statement', { n })}
      className={cn(
        'space-y-3 rounded-xl border bg-card p-4',
        statement.effect === 'Deny' ? TONE.danger.border : 'border-border',
      )}
    >
      <header className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-foreground">
          {t('builder.statement', { n })}
        </h3>
        {canRemove ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onRemove}
            aria-label={t('builder.removeStatement', { n })}
          >
            <TrashIcon className="text-destructive" />
          </Button>
        ) : null}
      </header>

      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">
          {t('builder.effect.label')}
        </p>
        <div
          role="group"
          aria-label={t('builder.effect.label')}
          className="flex gap-2"
        >
          {effects.map((e) => (
            <Button
              key={e}
              type="button"
              size="sm"
              variant={statement.effect === e ? 'default' : 'outline'}
              aria-pressed={statement.effect === e}
              onClick={() => onChange({ ...statement, effect: e })}
            >
              {t(`builder.effect.${e === 'Allow' ? 'allow' : 'deny'}`)}
            </Button>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          {statement.effect === 'Allow'
            ? t('builder.effect.allowHint')
            : t('builder.effect.denyHint')}
        </p>
        <FieldIssues issues={effectIssues} />
      </div>

      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">
          {t('builder.actions.label')}
        </p>
        <ActionPicker
          abilities={abilities}
          value={statement.actions}
          onChange={(actions) => onChange({ ...statement, actions })}
          invalid={actionIssues.length > 0}
        />
        <FieldIssues issues={actionIssues} />
      </div>

      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">
          {t('builder.resources.label')}
        </p>
        <ResourcePicker
          idPrefix={`stmt-${index}`}
          resources={resources}
          value={statement.resources}
          onChange={(next) => onChange({ ...statement, resources: next })}
          invalid={resourceIssues.length > 0}
        />
        <FieldIssues issues={resourceIssues} />
      </div>
    </section>
  )
}
