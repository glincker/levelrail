import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

// All three of internal/spec's StrategyRolling/StrategyRecreate/
// StrategyBlueGreen values are real, reconciler-backed choices
// (internal/reconcile/application's controller runs all three).
const SELECTABLE_STRATEGIES = ['recreate', 'blue-green', 'rolling'] as const

const deployStrategySchema = z.object({
  strategy: z.enum(['recreate', 'blue-green', 'rolling']),
  replicas: z.coerce
    .number({ error: 'Replicas is required' })
    .int('Replicas must be a whole number')
    .positive('Replicas must be at least 1'),
})

type DeployStrategyFormInput = z.input<typeof deployStrategySchema>
type DeployStrategyFormOutput = z.output<typeof deployStrategySchema>

function toFieldValues(app: AppDetail): DeployStrategyFormInput {
  return {
    strategy: app.strategy,
    replicas: app.replicas,
  }
}

const STRATEGY_LABELS: Record<string, string> = {
  recreate: 'Recreate',
  'blue-green': 'Blue-green',
  rolling: 'Rolling',
}

// Same full-replace-PUT pattern as DomainEditor/EnvEditor/
// ResourceLimitsEditor/HealthCheckEditor, bound to AppDetail.strategy/
// AppDetail.replicas. `values` + `resetOptions.keepDirtyValues`
// preserved for the same reason: this editor sits on the same AppDetail
// prop as the others and must not let a background refetch stomp
// unsaved edits in a sibling editor.
export function DeployStrategyEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState } = useForm<
    DeployStrategyFormInput,
    unknown,
    DeployStrategyFormOutput
  >({
    resolver: zodResolver(deployStrategySchema),
    values: toFieldValues(app),
    resetOptions: { keepDirtyValues: true },
  })
  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      { ...app, strategy: values.strategy, replicas: values.replicas },
      {
        onSuccess: () => {
          toast.add({ title: 'Deploy strategy saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowsClockwiseIcon className="size-4" />
          Deploy strategy
        </CardTitle>
        <CardDescription>
          How new deploys roll out, and how many replicas stay running.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="deploy-strategy">Strategy</FieldLabel>
              <Controller
                control={control}
                name="strategy"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="deploy-strategy" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {SELECTABLE_STRATEGIES.map((value) => (
                        <SelectItem key={value} value={value}>
                          {STRATEGY_LABELS[value]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <FieldError errors={[formState.errors.strategy]} />
            </Field>

            <Field>
              <FieldLabel htmlFor="deploy-replicas">Replicas</FieldLabel>
              <Input
                id="deploy-replicas"
                type="number"
                step="1"
                min="1"
                {...register('replicas')}
              />
              <FieldError errors={[formState.errors.replicas]} />
            </Field>
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save deploy strategy'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
