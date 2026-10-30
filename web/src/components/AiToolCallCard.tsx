import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  SearchGlassIcon,
  ShieldAlertGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import type { AiToolCall } from '../types/aiAssistant'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

function summarizeArguments(
  args: Record<string, unknown>,
  emptyLabel: string,
): string {
  const entries = Object.entries(args)
  if (entries.length === 0) return emptyLabel
  return entries
    .map(([key, value]) => {
      const rendered = typeof value === 'string' ? value : JSON.stringify(value)
      const truncated =
        rendered.length > 80 ? `${rendered.slice(0, 80)}...` : rendered
      return `${key}: ${truncated}`
    })
    .join(', ')
}

function summarizeResult(result: unknown): string {
  if (typeof result === 'string') return result
  try {
    return JSON.stringify(result)
  } catch {
    return String(result)
  }
}

// Inline note for a tool call that never needed (or no longer needs) a
// confirm step: a read-only tool the assistant already auto-executed, or
// a mutating one the operator already approved/rejected. Deliberately
// lighter weight than the confirmation Card below, no action to take.
function AiToolCallActor({ toolCall }: Readonly<{ toolCall: AiToolCall }>) {
  const { t } = useTranslation('aiAssistant')
  if (!toolCall.acting_as) return null
  const abilities = toolCall.abilities ?? []
  return (
    <span className="pl-6 text-xs text-muted-foreground">
      {abilities.length > 0
        ? t('toolCall.actingAsWithAbility', {
            user: toolCall.acting_as,
            abilities: abilities.join(', '),
          })
        : t('toolCall.actingAs', { user: toolCall.acting_as })}
    </span>
  )
}

function AiToolCallNote({ toolCall }: Readonly<{ toolCall: AiToolCall }>) {
  const { t } = useTranslation('aiAssistant')
  const hasResult = toolCall.result !== undefined

  if (toolCall.resolution === 'rejected') {
    return (
      <div className="flex items-center gap-2 rounded-md border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
        <CircleXGlassIcon className="size-5 shrink-0 text-destructive" />
        <span>
          {t('toolCall.rejected')}{' '}
          <span className="font-medium text-foreground">
            {toolCall.tool_name}
          </span>
        </span>
      </div>
    )
  }

  const icon = toolCall.read_only ? (
    <SearchGlassIcon className="size-5 shrink-0 text-muted-foreground" />
  ) : (
    <CircleCheckGlassIcon className="size-5 shrink-0 text-green-600 dark:text-green-400" />
  )
  const verb = toolCall.read_only
    ? t('toolCall.checked')
    : t('toolCall.approved')

  return (
    <div className="flex flex-col gap-1 rounded-md border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
      <span className="flex items-center gap-2">
        {icon}
        <span>
          {verb}{' '}
          <span className="font-medium text-foreground">
            {toolCall.tool_name}
          </span>
        </span>
      </span>
      {hasResult ? (
        <span
          className={
            toolCall.is_error
              ? 'pl-6 text-destructive'
              : 'pl-6 text-xs text-muted-foreground'
          }
        >
          {summarizeResult(toolCall.result)}
        </span>
      ) : null}
      <AiToolCallActor toolCall={toolCall} />
    </div>
  )
}

// Confirmation control for a mutating tool call proposed mid-turn.
// Renders Approve/Reject only while the call is unresolved and read-only
// is false; every other state (read-only, already resolved) renders the
// lighter AiToolCallNote instead. This is the one UI surface a mutating
// action can ever execute through: no path here calls onApprove without
// the operator clicking the button.
export function AiToolCallCard({
  toolCall,
  onApprove,
  onReject,
  disabled = false,
}: Readonly<{
  toolCall: AiToolCall
  onApprove: () => void
  onReject: () => void
  disabled?: boolean
}>) {
  const { t } = useTranslation('aiAssistant')
  const needsConfirmation = !toolCall.read_only && !toolCall.resolution

  if (!needsConfirmation) {
    return <AiToolCallNote toolCall={toolCall} />
  }

  return (
    <Card className="border-amber-300 dark:border-amber-800">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <ShieldAlertGlassIcon className="size-5 text-amber-600 dark:text-amber-400" />
          {toolCall.tool_name}
          <Badge variant="warning">{t('toolCall.confirmationRequired')}</Badge>
        </CardTitle>
        <CardDescription>
          {summarizeArguments(toolCall.arguments, t('toolCall.noArguments'))}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            size="sm"
            disabled={disabled}
            onClick={onApprove}
          >
            {t('toolCall.approve')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={onReject}
          >
            {t('toolCall.reject')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
