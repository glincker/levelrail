import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlayGlassIcon, TerminalGlassIcon } from '@thesvg/ui-react'
import { useOneShotRun } from '../queries/oneShotJob'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'

// Whitespace split with simple quote grouping, no shell semantics: the
// server runs the argv directly, as the CLI's apps run does.
function splitArgv(line: string): string[] {
  const parts = line.match(/"[^"]*"|'[^']*'|\S+/g) ?? []
  return parts.map((p) =>
    (p.startsWith('"') && p.endsWith('"') && p.length >= 2) ||
    (p.startsWith("'") && p.endsWith("'") && p.length >= 2)
      ? p.slice(1, -1)
      : p,
  )
}

export function RunCommandCard({
  name,
  services,
}: {
  name: string
  services?: string[]
}) {
  const { t } = useTranslation('appRun')
  const [line, setLine] = useState('')
  const [service, setService] = useState(services?.[0] ?? '')
  const { run, job, startError, startForbidden, pollError, running } =
    useOneShotRun()

  const multi = (services?.length ?? 0) >= 2
  const target = multi && service ? `${name}-${service}` : name
  const argv = splitArgv(line)

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (argv.length === 0 || running) return
    run(target, argv)
  }

  const error = startForbidden ? t('accessRule') : (startError ?? pollError)
  const done = job && job.state !== 'Running' && job.state !== ''

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TerminalGlassIcon className="size-5" />
          {t('title')}
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <form
          onSubmit={onSubmit}
          className="flex flex-col items-start gap-3 sm:flex-row"
        >
          {multi ? (
            <Field className="sm:w-40">
              <FieldLabel htmlFor="run-service">{t('service')}</FieldLabel>
              <NativeSelect
                id="run-service"
                value={service}
                onChange={(e) => setService(e.target.value)}
                disabled={running}
              >
                {services?.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </NativeSelect>
            </Field>
          ) : null}
          <Field className="flex-1">
            <FieldLabel htmlFor="run-command">{t('command')}</FieldLabel>
            <Input
              id="run-command"
              value={line}
              onChange={(e) => setLine(e.target.value)}
              className="font-mono"
              placeholder={t('placeholder')}
              autoComplete="off"
              spellCheck={false}
              disabled={running}
              aria-describedby="run-command-hint"
            />
            <p id="run-command-hint" className="text-xs text-muted-foreground">
              {t('hint')}
            </p>
          </Field>
          <Button
            type="submit"
            disabled={argv.length === 0 || running}
            className="sm:self-start sm:mt-6"
          >
            <PlayGlassIcon className="size-5" data-icon="inline-start" />
            {running ? t('running') : t('run')}
          </Button>
        </form>

        <div aria-live="polite">
          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}

          {job ? (
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-muted-foreground">
                  {t('state')}
                </span>
                <Badge
                  variant={job.state === 'Succeeded' ? 'success' : 'outline'}
                >
                  {job.state || 'Running'}
                </Badge>
                {done && job.exit_code !== undefined ? (
                  <>
                    <span className="text-sm font-medium text-muted-foreground">
                      {t('exitCode')}
                    </span>
                    <Badge
                      variant={job.exit_code === 0 ? 'success' : 'destructive'}
                    >
                      {job.exit_code}
                    </Badge>
                  </>
                ) : null}
              </div>
              {job.message ? (
                <p className="text-xs text-muted-foreground">{job.message}</p>
              ) : null}
              {done ? (
                <>
                  <p className="text-sm font-medium text-muted-foreground">
                    {t('output')}
                  </p>
                  <pre className="max-h-80 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs break-all whitespace-pre-wrap">
                    {job.output || t('empty')}
                  </pre>
                  {job.output_truncated ? (
                    <p className="text-xs text-muted-foreground">
                      {t('truncated')}
                    </p>
                  ) : null}
                </>
              ) : null}
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
