import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useGitSource } from '../queries/gitSources'
import {
  useSetPrCommentSetting,
  type PrCommentSetting,
} from '../queries/previewEnvironments'

// Off-by-default switches for /levelrail pull request comment commands. The
// fork switch only shows once commands are on.
export function PrCommentCommandsSetting({ appName }: { appName: string }) {
  const { t } = useTranslation('previews')
  const gitSource = useGitSource(appName)
  const setSetting = useSetPrCommentSetting(appName)

  if (!gitSource.data) return null
  const commands = gitSource.data.pr_comment_commands ?? false
  const forks = gitSource.data.pr_comment_commands_forks ?? false

  function change(key: PrCommentSetting, value: boolean) {
    const toastKey =
      key === 'pr_comment_commands'
        ? value
          ? 'toast.enabled'
          : 'toast.disabled'
        : value
          ? 'toast.forksEnabled'
          : 'toast.forksDisabled'
    setSetting.mutate(
      { key, value },
      {
        onSuccess: () => {
          toast.add({ title: t(`prCommands.${toastKey}`), type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: t('prCommands.toast.errorTitle'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-foreground">
            {t('prCommands.title')}
          </p>
          <p className="text-sm text-muted-foreground">
            {t('prCommands.description')}
          </p>
        </div>
        <Switch
          checked={commands}
          onCheckedChange={(next) => {
            change('pr_comment_commands', next)
          }}
          disabled={setSetting.isPending}
          aria-label={t('prCommands.enabledLabel')}
        />
      </div>
      {commands ? (
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">
              {t('prCommands.forksTitle')}
            </p>
            <p className="text-sm text-muted-foreground">
              {t('prCommands.forksDescription')}
            </p>
          </div>
          <Switch
            checked={forks}
            onCheckedChange={(next) => {
              change('pr_comment_commands_forks', next)
            }}
            disabled={setSetting.isPending}
            aria-label={t('prCommands.forksLabel')}
          />
        </div>
      ) : null}
    </div>
  )
}
