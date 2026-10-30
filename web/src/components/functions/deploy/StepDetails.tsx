import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Domain } from '../../../queries/domains'
import { FieldError, FieldHelp } from './FieldMessage'
import {
  IDLE_CHOICES,
  MIN_IDLE_MINUTES,
  routableDomains,
  type FieldErrors,
  type FlowValues,
  type UrlPreview,
} from './logic'
import type { NameState } from './useNameTaken'

const NO_DOMAIN = '__none__'

function NameField({
  values,
  errors,
  nameState,
  onName,
}: Readonly<{
  values: FlowValues
  errors: FieldErrors
  nameState: NameState
  onName: (v: string) => void
}>) {
  const { t } = useTranslation('functions')
  return (
    <div className="space-y-1.5">
      <Label htmlFor="fn-name">{t('deploy.details.name')}</Label>
      <Input
        id="fn-name"
        value={values.name}
        autoComplete="off"
        aria-invalid={errors.name ? true : undefined}
        aria-describedby="fn-name-help fn-name-error"
        onChange={(e) => {
          onName(e.target.value)
        }}
      />
      <FieldError id="fn-name-error" messageKey={errors.name} />
      {!errors.name && nameState === 'checking' ? (
        <FieldHelp id="fn-name-help">
          {t('deploy.details.nameChecking')}
        </FieldHelp>
      ) : null}
      {!errors.name && nameState === 'free' ? (
        <FieldHelp id="fn-name-help">
          {t('deploy.details.nameFree', { name: values.name.trim() })}
        </FieldHelp>
      ) : null}
      {!errors.name && nameState === 'idle' ? (
        <FieldHelp id="fn-name-help">{t('deploy.details.nameHint')}</FieldHelp>
      ) : null}
    </div>
  )
}

function IdleField({
  values,
  errors,
  update,
}: Readonly<{
  values: FlowValues
  errors: FieldErrors
  update: (p: Partial<FlowValues>) => void
}>) {
  const { t } = useTranslation('functions')
  return (
    <div className="space-y-1.5">
      <Label htmlFor="fn-idle">{t('deploy.details.idle')}</Label>
      <div className="flex gap-2">
        <Select
          value={values.idleChoice}
          onValueChange={(v) => {
            update({ idleChoice: v ?? '10' })
          }}
        >
          <SelectTrigger id="fn-idle" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {IDLE_CHOICES.map((c) => (
              <SelectItem key={c} value={c}>
                {t(`deploy.details.idleOption.${c}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {values.idleChoice === 'custom' ? (
          <Input
            aria-label={t('deploy.details.customIdle')}
            type="number"
            min={MIN_IDLE_MINUTES}
            className="w-28"
            value={values.customIdle}
            aria-invalid={errors.customIdle ? true : undefined}
            onChange={(e) => {
              update({ customIdle: e.target.value })
            }}
          />
        ) : null}
      </div>
      <FieldHelp>{t('deploy.details.idleHint')}</FieldHelp>
      <FieldError id="fn-idle-error" messageKey={errors.customIdle} />
    </div>
  )
}

function RouteFields({
  values,
  errors,
  domains,
  preview,
  update,
}: Readonly<{
  values: FlowValues
  errors: FieldErrors
  domains: Domain[]
  preview: UrlPreview
  update: (p: Partial<FlowValues>) => void
}>) {
  const { t } = useTranslation('functions')
  const options = routableDomains(domains)
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">
        {t('deploy.details.route')}
      </legend>
      <FieldHelp>{t('deploy.details.routeHint')}</FieldHelp>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="fn-route-domain">
            {t('deploy.details.routeDomain')}
          </Label>
          <Select
            value={values.routeDomain || NO_DOMAIN}
            onValueChange={(v) => {
              update({ routeDomain: !v || v === NO_DOMAIN ? '' : v })
            }}
          >
            <SelectTrigger id="fn-route-domain" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_DOMAIN}>
                {t('deploy.details.routeNone')}
              </SelectItem>
              {options.map((d) => (
                <SelectItem key={d.domain} value={d.domain}>
                  {d.domain}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldError
            id="fn-route-domain-error"
            messageKey={errors.routeDomain}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="fn-route-path">{t('deploy.details.routePath')}</Label>
          <Input
            id="fn-route-path"
            placeholder="/hello"
            value={values.routePath}
            aria-invalid={errors.routePath ? true : undefined}
            onChange={(e) => {
              update({ routePath: e.target.value })
            }}
          />
          <FieldError id="fn-route-path-error" messageKey={errors.routePath} />
        </div>
      </div>
      <dl className="space-y-1 text-xs">
        <div className="flex gap-2">
          <dt className="text-muted-foreground">
            {t('deploy.details.previewFunction')}
          </dt>
          <dd className="min-w-0 font-mono break-all">
            {preview.functionUrl ?? t('deploy.details.previewPending')}
          </dd>
        </div>
        {preview.routeUrl ? (
          <div className="flex gap-2">
            <dt className="text-muted-foreground">
              {t('deploy.details.previewRoute')}
            </dt>
            <dd className="min-w-0 font-mono break-all">{preview.routeUrl}</dd>
          </div>
        ) : null}
      </dl>
    </fieldset>
  )
}

export function StepDetails({
  values,
  errors,
  domains,
  preview,
  nameState,
  update,
  onName,
}: Readonly<{
  values: FlowValues
  errors: FieldErrors
  domains: Domain[]
  preview: UrlPreview
  nameState: NameState
  update: (p: Partial<FlowValues>) => void
  onName: (v: string) => void
}>) {
  const { t } = useTranslation('functions')
  return (
    <div className="space-y-4">
      <NameField
        values={values}
        errors={errors}
        nameState={nameState}
        onName={onName}
      />
      <IdleField values={values} errors={errors} update={update} />
      <RouteFields
        values={values}
        errors={errors}
        domains={domains}
        preview={preview}
        update={update}
      />
      <Collapsible>
        <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium">
          <ChevronDownGlassIcon aria-hidden="true" className="size-5" />
          {t('deploy.details.advanced')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <div className="space-y-1.5 pt-3">
            <Label htmlFor="fn-port">{t('deploy.details.port')}</Label>
            <Input
              id="fn-port"
              type="number"
              min={1}
              max={65535}
              value={values.port}
              aria-invalid={errors.port ? true : undefined}
              onChange={(e) => {
                update({ port: e.target.value })
              }}
            />
            <FieldHelp>{t('deploy.details.portHint')}</FieldHelp>
          </div>
        </CollapsiblePanel>
      </Collapsible>
      <FieldError id="fn-port-error" messageKey={errors.port} />
    </div>
  )
}
