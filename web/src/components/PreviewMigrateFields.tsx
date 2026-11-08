import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetPreviewPolicy } from '../queries/previewPolicy'
import type { PreviewPolicy } from '../types/previewEnvironment'

const TAG_PLACEHOLDERS = ['{number}', '{sha}', '{short_sha}', '{branch}']
const EXAMPLE_PR = 42
const EXAMPLE_SHA = '9f3c2a7d41b85e60aa17c3d2f0e9b4c8d5a6f701'
const EXAMPLE_BRANCH = 'feature/login'

function exampleImageTag(template: string): string {
  return template
    .replaceAll('{number}', String(EXAMPLE_PR))
    .replaceAll('{short_sha}', EXAMPLE_SHA.slice(0, 7))
    .replaceAll('{sha}', EXAMPLE_SHA)
    .replaceAll('{branch}', EXAMPLE_BRANCH.replaceAll('/', '-'))
}

// Migrate command, its timeout and the image tag template of the preview policy.
export function PreviewMigrateFields({
  appName,
  policy,
  isImageApp,
}: {
  appName: string
  policy: PreviewPolicy
  isImageApp: boolean
}) {
  const { t } = useTranslation('previews')
  const setPolicy = useSetPreviewPolicy(appName)
  const [command, setCommand] = useState(policy.migrate_command)
  const [timeout, setTimeoutValue] = useState(
    String(policy.migrate_timeout_seconds),
  )
  const [template, setTemplate] = useState(policy.image_tag_template)
  const timeoutNum = Number(timeout)
  const timeoutValid = Number.isInteger(timeoutNum) && timeoutNum >= 0
  const strategyBlocks = policy.database_strategy !== 'none'
  const templateValid =
    template === '' || TAG_PLACEHOLDERS.some((p) => template.includes(p))

  function save(update: Parameters<typeof setPolicy.mutate>[0]) {
    setPolicy.mutate(update, {
      onSuccess: () =>
        toast.add({ title: t('migrate.saved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('migrate.error'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <label
          htmlFor="preview-migrate-command"
          className="text-sm font-medium"
        >
          {t('migrate.command')}
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            id="preview-migrate-command"
            className="min-w-0 flex-1 font-mono"
            value={command}
            disabled={strategyBlocks}
            placeholder={t('migrate.commandPlaceholder')}
            onChange={(e) => {
              setCommand(e.target.value)
            }}
            aria-describedby="preview-migrate-command-hint"
          />
          <Button
            size="sm"
            disabled={
              strategyBlocks ||
              command === policy.migrate_command ||
              setPolicy.isPending
            }
            onClick={() => {
              save({ migrate_command: command.trim() })
            }}
          >
            {t('migrate.save')}
          </Button>
        </div>
        <p
          id="preview-migrate-command-hint"
          className={
            strategyBlocks
              ? 'text-xs text-tone-warning'
              : 'text-xs text-muted-foreground'
          }
        >
          {strategyBlocks
            ? t('migrate.strategyBlocked', {
                strategy: policy.database_strategy,
              })
            : t('migrate.commandHint')}
        </p>
      </div>

      <div className="space-y-1">
        <label
          htmlFor="preview-migrate-timeout"
          className="text-sm font-medium"
        >
          {t('migrate.timeout')}
        </label>
        <div className="flex items-center gap-2">
          <Input
            id="preview-migrate-timeout"
            inputMode="numeric"
            className="w-28"
            value={timeout}
            aria-invalid={!timeoutValid}
            onChange={(e) => {
              setTimeoutValue(e.target.value)
            }}
          />
          <Button
            size="sm"
            disabled={
              !timeoutValid ||
              timeoutNum === policy.migrate_timeout_seconds ||
              setPolicy.isPending
            }
            onClick={() => {
              save({ migrate_timeout_seconds: timeoutNum })
            }}
          >
            {t('migrate.save')}
          </Button>
          <span className="text-xs text-muted-foreground">
            {t('migrate.timeoutDefault', {
              seconds: policy.effective_migrate_timeout_seconds,
            })}
          </span>
        </div>
      </div>

      {isImageApp ? (
        <div className="space-y-1">
          <label htmlFor="preview-image-tag" className="text-sm font-medium">
            {t('migrate.imageTag')}
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <Input
              id="preview-image-tag"
              className="min-w-0 flex-1 font-mono"
              value={template}
              aria-invalid={!templateValid}
              placeholder="pr-{number}"
              onChange={(e) => {
                setTemplate(e.target.value)
              }}
            />
            <Button
              size="sm"
              disabled={
                !templateValid ||
                template === policy.image_tag_template ||
                setPolicy.isPending
              }
              onClick={() => {
                save({ image_tag_template: template.trim() })
              }}
            >
              {t('migrate.save')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            {templateValid
              ? t('migrate.imageTagHint', {
                  placeholders: TAG_PLACEHOLDERS.join(' '),
                })
              : t('migrate.imageTagInvalid')}
          </p>
          {template !== '' && templateValid ? (
            <p className="text-xs text-muted-foreground">
              {t('migrate.imageTagExample', {
                number: EXAMPLE_PR,
                tag: exampleImageTag(template),
              })}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
