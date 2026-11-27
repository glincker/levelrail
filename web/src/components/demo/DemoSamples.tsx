import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DEMO_CHAT_PROMPTS,
  DEMO_QUERY_COLUMNS,
  DEMO_QUERY_ROWS,
  DEMO_QUERY_SQL,
  DEMO_TERMINAL_LINES,
  type DemoChatPromptId,
} from '@/lib/demoSamples'
import { Button } from '@/components/ui/button'
import { DemoSampleFrame } from './DemoSampleFrame'

function promptText(
  t: (
    k:
      | 'sample.chat.prompts.crashloop'
      | 'sample.chat.prompts.slow'
      | 'sample.chat.prompts.rollback',
  ) => string,
  id: DemoChatPromptId,
): string {
  if (id === 'crashloop') return t('sample.chat.prompts.crashloop')
  if (id === 'slow') return t('sample.chat.prompts.slow')
  return t('sample.chat.prompts.rollback')
}

function replyText(
  t: (
    k:
      | 'sample.chat.replies.crashloop'
      | 'sample.chat.replies.slow'
      | 'sample.chat.replies.rollback',
  ) => string,
  id: DemoChatPromptId,
): string {
  if (id === 'crashloop') return t('sample.chat.replies.crashloop')
  if (id === 'slow') return t('sample.chat.replies.slow')
  return t('sample.chat.replies.rollback')
}

/** DemoSampleChat answers three suggested questions with canned replies, no model or network involved. */
export function DemoSampleChat() {
  const { t } = useTranslation('demo')
  const [picked, setPicked] = useState<DemoChatPromptId[]>([])
  return (
    <DemoSampleFrame hint={t('sample.chat.hint')}>
      <div
        className="min-h-0 flex-1 space-y-3 overflow-auto rounded-md border border-border p-3"
        aria-live="polite"
      >
        {picked.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('sample.chat.empty')}
          </p>
        ) : null}
        {picked.map((id) => (
          <div key={id} className="space-y-2 text-sm">
            <p className="ml-auto w-fit max-w-[85%] rounded-lg bg-muted px-3 py-2">
              {promptText(t, id)}
            </p>
            <p className="max-w-[85%] rounded-lg border border-border px-3 py-2">
              {replyText(t, id)}
            </p>
          </div>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        {DEMO_CHAT_PROMPTS.map((id) => (
          <Button
            key={id}
            type="button"
            size="sm"
            variant="outline"
            disabled={picked.includes(id)}
            onClick={() => {
              setPicked((cur) => [...cur, id])
            }}
          >
            {promptText(t, id)}
          </Button>
        ))}
      </div>
    </DemoSampleFrame>
  )
}

/** DemoSampleTerminal shows a fixed transcript so visitors see what a shell session looks like. */
export function DemoSampleTerminal() {
  const { t } = useTranslation('demo')
  return (
    <DemoSampleFrame hint={t('sample.terminal.hint')}>
      <pre
        className="min-h-0 flex-1 overflow-auto rounded-md border border-border bg-muted p-3 font-mono text-xs"
        role="group"
        aria-label={t('sample.terminal.label')}
      >
        {DEMO_TERMINAL_LINES.map((l, i) => (
          <div key={i}>
            {l.prompt ? (
              <span className="text-muted-foreground">$ </span>
            ) : null}
            {l.text}
          </div>
        ))}
      </pre>
    </DemoSampleFrame>
  )
}

/** DemoSampleQuery shows a query and its result table for the database console and explorer. */
export function DemoSampleQuery() {
  const { t } = useTranslation('demo')
  return (
    <DemoSampleFrame hint={t('sample.database.hint')}>
      <pre
        className="overflow-auto rounded-md border border-border bg-muted p-3 font-mono text-xs"
        role="group"
        aria-label={t('sample.database.queryLabel')}
      >
        {DEMO_QUERY_SQL}
      </pre>
      <div className="overflow-auto rounded-md border border-border">
        <table className="w-full text-left text-xs">
          <caption className="sr-only">
            {t('sample.database.resultLabel')}
          </caption>
          <thead className="bg-muted text-muted-foreground">
            <tr>
              {DEMO_QUERY_COLUMNS.map((c) => (
                <th key={c} scope="col" className="px-3 py-1.5 font-medium">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="font-mono">
            {DEMO_QUERY_ROWS.map((row) => (
              <tr key={row[0]} className="border-t border-border">
                {row.map((cell, i) => (
                  <td key={i} className="px-3 py-1.5">
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('sample.database.rowCount', { count: DEMO_QUERY_ROWS.length })}
      </p>
    </DemoSampleFrame>
  )
}
