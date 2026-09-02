import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TerminalIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { useExecApp, type ExecAppResult } from '../queries/exec'
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

// DEFAULT_COMMAND pre-fills the input so a first-time, non-technical
// operator never faces a blank required field: "Command is required"
// is a validation message meant for someone who cleared the field on
// purpose, not the very first thing anyone sees.
const DEFAULT_COMMAND = 'ls -la /app'

// SUGGESTED_COMMANDS are one-click fillers for the handful of things an
// operator actually reaches for here (what files are in the app, what
// environment variables are set, is anything still running), so
// "type a shell command from scratch" is the fallback, not the only
// path. All read-only, safe to run against anything.
const SUGGESTED_COMMANDS: { label: string; command: string }[] = [
  { label: 'List files', command: 'ls -la /app' },
  { label: 'Environment variables', command: 'env' },
  { label: 'Running processes', command: 'ps aux' },
  { label: 'Disk usage', command: 'df -h' },
]

// splitCommandLine turns a single typed line into a command plus argv
// entries by splitting on whitespace: no quoting, no shell operators
// (pipes, redirects, globs, env expansion), matching the backend's own
// contract exactly (internal/api/exec.go's execRequest doc comment):
// POST /apps/{name}/exec never shell-interprets what it's given. A user
// who needs any of that has to ask for a shell explicitly, e.g. typing
// `sh -c "ls | wc -l"`, the same escape hatch the backend's own doc
// comment names, at which point this function only needs to split off
// two tokens ("sh", "-c") and hand the whole quoted remainder through
// as a single arg. json-ish naive splitting is not attempted here on
// purpose: this is a quick one-off command runner, not a shell
// emulator.
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

// ExecPanel is Overview's one-off command runner: a command input, a
// Run button, and a read-only output area, deliberately not a real
// terminal (no PTY, no keystroke-by-keystroke interaction, no session
// kept alive between runs). See internal/api/exec.go's own package doc
// comment for exactly why: the backend endpoint this calls is a
// synchronous run-and-wait, not a stream, so there is nothing here for
// a fancier UI to attach to yet. Rendered as its own tab
// (routes/apps/$name/exec.tsx), not a dialog off Overview, the same
// "large enough a concern to earn its own URL" reasoning
// routes/apps/$name/deploys's own doc comment gives for deploy history.
export function ExecPanel({ name }: { name: string }) {
  const execApp = useExecApp(name)
  const { register, handleSubmit, formState, setValue, setFocus } =
    useForm<ExecFormValues>({
      resolver: zodResolver(execSchema),
      defaultValues: { commandLine: DEFAULT_COMMAND },
    })

  const onSubmit = handleSubmit((values) => {
    const { command, args } = splitCommandLine(values.commandLine)
    execApp.mutate({ command, args })
  })

  const result: ExecAppResult | undefined = execApp.data

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TerminalIcon className="size-4" />
          Run a one-off command
        </CardTitle>
        <CardDescription>
          Runs inside this app&apos;s currently running container and waits for
          it to finish (up to 30 seconds). Not an interactive shell: no pipes,
          redirects, or globbing, type{' '}
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
            <FieldLabel htmlFor="exec-command">Command</FieldLabel>
            <Input
              id="exec-command"
              {...register('commandLine')}
              className="font-mono"
              placeholder="ls -la /app"
              autoComplete="off"
              spellCheck={false}
              disabled={execApp.isPending}
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
            disabled={execApp.isPending}
            className="sm:self-end"
          >
            <PlayIcon className="size-3.5" data-icon="inline-start" />
            {execApp.isPending ? 'Running...' : 'Run'}
          </Button>
        </form>

        {execApp.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{execApp.error.message}</AlertDescription>
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
              <FieldLabel htmlFor="exec-stdout">stdout</FieldLabel>
              <Textarea
                id="exec-stdout"
                readOnly
                className="min-h-32 font-mono text-xs"
                value={result.stdout || '(empty)'}
              />
            </Field>
            {result.stderr ? (
              <Field>
                <FieldLabel htmlFor="exec-stderr">stderr</FieldLabel>
                <Textarea
                  id="exec-stderr"
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
