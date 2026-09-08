import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TerminalIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { useExecDatabase, type ExecDatabaseResult } from '../queries/exec'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

const execSchema = z.object({
  commandLine: z.string().trim().min(1, 'Command is required'),
})

type ExecFormValues = z.infer<typeof execSchema>

// DEFAULT_COMMAND mirrors ExecPanel's own reasoning: pre-fill so a
// first-time operator never faces a blank required field. "env" works
// against every engine this platform manages, unlike an engine-specific
// client command, so it is a safer universal default than picking one
// engine's own CLI.
const DEFAULT_COMMAND = 'env'

// SUGGESTED_COMMANDS covers the handful of engine clients a database
// container actually ships, so "type a command from scratch" is the
// fallback, not the only path. Each entry only makes sense against its
// own engine, but all are harmless to click against the wrong one (the
// binary simply won't be found, reported as a nonzero exit code, not a
// destructive action).
const SUGGESTED_COMMANDS: { label: string; command: string }[] = [
  { label: 'Environment variables', command: 'env' },
  { label: 'Redis ping', command: 'redis-cli ping' },
  { label: 'Postgres list databases', command: "psql -U postgres -c \\l" },
  { label: 'MySQL/MariaDB status', command: 'mysqladmin status' },
]

// splitCommandLine: see ExecPanel's own doc comment, identical
// whitespace/quote-only splitting, no shell operators.
function splitCommandLine(line: string): { command: string; args: string[] } {
  const parts = line.match(/"[^"]*"|'[^']*'|\S+/g) ?? []
  const unquoted = parts.map((p) =>
    (p.startsWith('"') && p.endsWith('"')) ||
    (p.startsWith("'") && p.endsWith("'"))
      ? p.slice(1, -1)
      : p,
  )
  const [command, ...args] = unquoted
  return { command: command ?? '', args }
}

// ExecDatabasePanel is ExecPanel's database counterpart: a command
// input, a Run button, and a read-only output area against a managed
// database's own currently running container, deliberately not a real
// terminal (see internal/api/database_exec.go's own doc comment on
// handleExecDatabase for why). Rendered as its own tab
// (routes/databases/$name/exec.tsx), matching ExecPanel's own
// "large enough a concern to earn its own URL" placement.
export function ExecDatabasePanel({ name }: { name: string }) {
  const execDatabase = useExecDatabase(name)
  const { register, handleSubmit, formState, setValue, setFocus } =
    useForm<ExecFormValues>({
      resolver: zodResolver(execSchema),
      defaultValues: { commandLine: DEFAULT_COMMAND },
    })

  const onSubmit = handleSubmit((values) => {
    const { command, args } = splitCommandLine(values.commandLine)
    execDatabase.mutate({ command, args })
  })

  const result: ExecDatabaseResult | undefined = execDatabase.data

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TerminalIcon className="size-4" />
          Run a one-off command
        </CardTitle>
        <CardDescription>
          Runs inside this database&apos;s currently running container and
          waits for it to finish (up to 30 seconds). Not an interactive
          shell: no pipes, redirects, or globbing, type{' '}
          <code className="font-mono">sh -c &quot;...&quot;</code> if you need
          any of those.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-medium text-muted-foreground">
            Quick picks:
          </span>
          {SUGGESTED_COMMANDS.map((suggestion) => (
            <Button
              key={suggestion.command}
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setValue('commandLine', suggestion.command, {
                  shouldValidate: true,
                })
                setFocus('commandLine')
              }}
            >
              {suggestion.label}
            </Button>
          ))}
        </div>

        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="flex flex-col items-start gap-3 sm:flex-row"
        >
          <Field className="flex-1">
            <FieldLabel htmlFor="db-exec-command">Command</FieldLabel>
            <Input
              id="db-exec-command"
              {...register('commandLine')}
              className="font-mono"
              placeholder="redis-cli ping"
              autoComplete="off"
              spellCheck={false}
              disabled={execDatabase.isPending}
            />
            <FieldError
              errors={
                formState.errors.commandLine
                  ? [formState.errors.commandLine]
                  : undefined
              }
            />
          </Field>
          <Button
            type="submit"
            disabled={execDatabase.isPending}
            className="sm:self-end"
          >
            <PlayIcon className="size-3.5" data-icon="inline-start" />
            {execDatabase.isPending ? 'Running...' : 'Run'}
          </Button>
        </form>

        {execDatabase.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{execDatabase.error.message}</AlertDescription>
          </Alert>
        ) : null}

        {result ? (
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-muted-foreground">
                Exit code
              </span>
              <Badge
                variant={result.exit_code === 0 ? 'success' : 'destructive'}
              >
                {result.exit_code}
              </Badge>
              {result.truncated ? (
                <Badge variant="warning">output truncated</Badge>
              ) : null}
            </div>
            <Field>
              <FieldLabel htmlFor="db-exec-stdout">stdout</FieldLabel>
              <Textarea
                id="db-exec-stdout"
                readOnly
                className="min-h-32 font-mono text-xs"
                value={result.stdout || '(empty)'}
              />
            </Field>
            {result.stderr ? (
              <Field>
                <FieldLabel htmlFor="db-exec-stderr">stderr</FieldLabel>
                <Textarea
                  id="db-exec-stderr"
                  readOnly
                  className="min-h-20 font-mono text-xs"
                  value={result.stderr}
                />
              </Field>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
