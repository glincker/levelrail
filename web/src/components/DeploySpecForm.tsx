import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useFieldArray, useForm, useWatch } from 'react-hook-form'
import type { Control, UseFormRegister } from 'react-hook-form'
import { z } from 'zod'
import { PlusIcon, RocketIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { useDeploySpec } from '../queries/appGroup'
import type { DeploySpecResult } from '../types/appGroup'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'

const buildTypeValues = ['dockerfile', 'railpack', 'static', 'image'] as const

const serviceSchema = z
  .object({
    key: z.string().trim().min(1, 'Service key is required'),
    buildType: z.enum(buildTypeValues),
    buildPath: z.string().trim(),
    buildImage: z.string().trim(),
    port: z.string().trim(),
    domains: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.buildType === 'image' && !data.buildImage) {
      ctx.addIssue({
        code: 'custom',
        message: 'Image reference is required for build type "image"',
        path: ['buildImage'],
      })
    }
  })

const deploySpecSchema = z
  .object({
    repoUrl: z.string().trim().min(1, 'Repository URL is required'),
    ref: z.string().trim().min(1, 'Branch, tag, or commit is required'),
    imageRepoBase: z.string().trim(),
    services: z.array(serviceSchema).min(1, 'At least one service is required'),
  })
  .superRefine((data, ctx) => {
    const seen = new Set<string>()
    data.services.forEach((s, index) => {
      const key = s.key.trim()
      if (key && seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Duplicate service key',
          path: ['services', index, 'key'],
        })
      }
      seen.add(key)
    })
  })

type DeploySpecFormValues = z.infer<typeof deploySpecSchema>

const emptyService = {
  key: '',
  buildType: 'dockerfile' as const,
  buildPath: '',
  buildImage: '',
  port: '',
  domains: '',
}

// Manual trigger for POST /api/v1/apps/{name}/deploy-spec
// (internal/api/apps_multi.go's handleDeploySpec): every listed service
// builds from the same repo_url/ref checkout. Per-service env/health/
// resources are deliberately not collected here, only build config, port,
// and domains, the common case.
export function DeploySpecForm({ appName }: { appName: string }) {
  const deploySpec = useDeploySpec(appName)
  const { register, control, handleSubmit, formState, reset } =
    useForm<DeploySpecFormValues>({
      resolver: zodResolver(deploySpecSchema),
      defaultValues: {
        repoUrl: '',
        ref: '',
        imageRepoBase: '',
        services: [emptyService],
      },
    })
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'services',
  })

  const onSubmit = handleSubmit((values) => {
    deploySpec.mutate(
      {
        repoUrl: values.repoUrl.trim(),
        ref: values.ref.trim(),
        imageRepoBase: values.imageRepoBase.trim() || undefined,
        services: values.services.map((s) => ({
          key: s.key.trim(),
          buildType: s.buildType,
          buildPath: s.buildPath.trim() || undefined,
          buildImage: s.buildImage.trim() || undefined,
          port: s.port.trim() ? Number(s.port.trim()) : undefined,
          domains: s.domains.trim()
            ? s.domains
                .split(',')
                .map((d) => d.trim())
                .filter(Boolean)
            : undefined,
        })),
      },
      {
        onSuccess: () => {
          reset({
            repoUrl: values.repoUrl,
            ref: values.ref,
            imageRepoBase: values.imageRepoBase,
            services: [emptyService],
          })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RocketIcon className="size-4" />
          Deploy multiple services
        </CardTitle>
        <CardDescription>
          Fan an app.yaml-style services: map out into independent builds, all
          under this app.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <div className="flex flex-col gap-3 sm:flex-row">
            <Field className="flex-[2]">
              <FieldLabel htmlFor="deploy-spec-repo-url">
                Repository URL
              </FieldLabel>
              <Input
                id="deploy-spec-repo-url"
                {...register('repoUrl')}
                className="font-mono"
                placeholder="https://github.com/you/app.git"
                autoComplete="off"
                spellCheck={false}
                disabled={deploySpec.isPending}
              />
              <FieldError
                errors={
                  formState.errors.repoUrl
                    ? [formState.errors.repoUrl]
                    : undefined
                }
              />
            </Field>
            <Field className="flex-1">
              <FieldLabel htmlFor="deploy-spec-ref">
                Branch, tag, or commit
              </FieldLabel>
              <Input
                id="deploy-spec-ref"
                {...register('ref')}
                className="font-mono"
                placeholder="main"
                autoComplete="off"
                spellCheck={false}
                disabled={deploySpec.isPending}
              />
              <FieldError
                errors={
                  formState.errors.ref ? [formState.errors.ref] : undefined
                }
              />
            </Field>
            <Field className="flex-1">
              <FieldLabel htmlFor="deploy-spec-image-repo-base">
                Image name prefix (optional)
              </FieldLabel>
              <Input
                id="deploy-spec-image-repo-base"
                {...register('imageRepoBase')}
                className="font-mono"
                placeholder={appName}
                autoComplete="off"
                spellCheck={false}
                disabled={deploySpec.isPending}
              />
            </Field>
          </div>

          <FieldGroup className="gap-3">
            {fields.map((field, index) => (
              <ServiceRow
                key={field.id}
                index={index}
                register={register}
                control={control}
                errors={formState.errors.services?.[index]}
                onRemove={fields.length > 1 ? () => remove(index) : undefined}
                disabled={deploySpec.isPending}
              />
            ))}
          </FieldGroup>
          {formState.errors.services?.root ? (
            <p className="text-sm text-destructive">
              {formState.errors.services.root.message}
            </p>
          ) : null}

          <div className="flex items-center justify-between">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => append(emptyService)}
              disabled={deploySpec.isPending}
            >
              <PlusIcon className="size-3.5" data-icon="inline-start" />
              Add service
            </Button>
            <Button type="submit" disabled={deploySpec.isPending}>
              {deploySpec.isPending ? 'Deploying...' : 'Deploy services'}
            </Button>
          </div>
        </form>

        {deploySpec.isError ? (
          <Alert variant="destructive" className="mt-3">
            <AlertDescription>{deploySpec.error.message}</AlertDescription>
          </Alert>
        ) : null}
        {deploySpec.isSuccess ? (
          <DeploySpecResultSummary result={deploySpec.data} />
        ) : null}
      </CardContent>
    </Card>
  )
}

type ServiceFormError = NonNullable<
  ReturnType<
    typeof useForm<DeploySpecFormValues>
  >['formState']['errors']['services']
>[number]

function ServiceRow({
  index,
  register,
  control,
  errors,
  onRemove,
  disabled,
}: {
  index: number
  register: UseFormRegister<DeploySpecFormValues>
  control: Control<DeploySpecFormValues>
  errors?: ServiceFormError
  onRemove?: () => void
  disabled: boolean
}) {
  const buildType = useWatch({ control, name: `services.${index}.buildType` })

  return (
    <div className="rounded-md border border-border p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
        <Field className="flex-1">
          <FieldLabel htmlFor={`deploy-spec-key-${index}`}>
            Service key
          </FieldLabel>
          <Input
            id={`deploy-spec-key-${index}`}
            {...register(`services.${index}.key`)}
            className="font-mono"
            placeholder="web"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
          />
          <FieldError errors={errors?.key ? [errors.key] : undefined} />
        </Field>
        <Field className="flex-1">
          <FieldLabel htmlFor={`deploy-spec-build-type-${index}`}>
            Build type
          </FieldLabel>
          <Controller
            control={control}
            name={`services.${index}.buildType`}
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange}>
                <SelectTrigger id={`deploy-spec-build-type-${index}`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {buildTypeValues.map((v) => (
                    <SelectItem key={v} value={v}>
                      {v}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </Field>
        <Field className="flex-1">
          <FieldLabel htmlFor={`deploy-spec-port-${index}`}>
            Port (optional)
          </FieldLabel>
          <Input
            id={`deploy-spec-port-${index}`}
            {...register(`services.${index}.port`)}
            type="number"
            placeholder="3000"
            disabled={disabled}
          />
        </Field>
        {onRemove ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="mt-6 shrink-0 text-muted-foreground"
            onClick={onRemove}
            disabled={disabled}
            aria-label="Remove service"
          >
            <TrashIcon className="size-4" />
          </Button>
        ) : null}
      </div>
      <div className="mt-3 flex flex-col gap-3 sm:flex-row">
        {buildType === 'image' ? (
          <Field className="flex-1">
            <FieldLabel htmlFor={`deploy-spec-build-image-${index}`}>
              Image reference
            </FieldLabel>
            <Input
              id={`deploy-spec-build-image-${index}`}
              {...register(`services.${index}.buildImage`)}
              className="font-mono"
              placeholder="ghcr.io/org/image:tag"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
            />
            <FieldError
              errors={errors?.buildImage ? [errors.buildImage] : undefined}
            />
          </Field>
        ) : (
          <Field className="flex-1">
            <FieldLabel htmlFor={`deploy-spec-build-path-${index}`}>
              {buildType === 'dockerfile' ? 'Dockerfile path' : 'Build path'}{' '}
              (optional)
            </FieldLabel>
            <Input
              id={`deploy-spec-build-path-${index}`}
              {...register(`services.${index}.buildPath`)}
              className="font-mono"
              placeholder="./Dockerfile"
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
            />
          </Field>
        )}
        <Field className="flex-1">
          <FieldLabel htmlFor={`deploy-spec-domains-${index}`}>
            Domains (optional, comma-separated)
          </FieldLabel>
          <Input
            id={`deploy-spec-domains-${index}`}
            {...register(`services.${index}.domains`)}
            placeholder="app.example.com, www.app.example.com"
            autoComplete="off"
            spellCheck={false}
            disabled={disabled}
          />
        </Field>
      </div>
    </div>
  )
}

function DeploySpecResultSummary({ result }: { result: DeploySpecResult }) {
  return (
    <div className="mt-3 space-y-2 rounded-md border border-border p-3">
      <p className="text-sm font-medium text-foreground">
        Deploy triggered for app_id {result.app_id}
      </p>
      <ul className="space-y-1">
        {result.services.map((s) => (
          <li key={s.service_key} className="flex items-center gap-2 text-sm">
            <Badge variant={s.error ? 'destructive' : 'success'}>
              {s.error ? 'Failed' : 'Deployed'}
            </Badge>
            <span className="font-mono text-xs">{s.service_key}</span>
            {s.error ? (
              <span className="text-xs text-destructive">{s.error}</span>
            ) : (
              <span className="truncate text-xs text-muted-foreground">
                {s.image}
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
