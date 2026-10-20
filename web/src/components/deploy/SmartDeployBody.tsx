import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useApp } from '../../queries/apps'
import { useGitSource } from '../../queries/gitSources'
import { ConnectSourcePanel } from './ConnectSourcePanel'
import { GitDeployPanel } from './GitDeployPanel'
import { ImageDeployPanel } from './ImageDeployPanel'

export type DeployMode = 'auto' | 'image' | 'connect'

type SheetTab = 'existing-image' | 'build-from-source'

// What the sheet shows depends on what the app deploys from: a connected
// repository gets commits and suggestions, an image-only app a pre-filled
// image form, an app with neither a guided connect.
export function SmartDeployBody({
  appName,
  tab,
  mode,
  onModeChange,
  onClose,
}: {
  appName: string
  tab: SheetTab
  mode: DeployMode
  onModeChange: (mode: DeployMode) => void
  onClose: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const { data: app } = useApp(appName)
  const source = useGitSource(appName)

  if (source.isPending) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        {t('git.loading')}
      </p>
    )
  }

  const connected = source.data

  if (connected && mode !== 'image') {
    return (
      <div className="space-y-4">
        <GitDeployPanel app={app} source={connected} onClose={onClose} />
        <Button
          variant="ghost"
          size="sm"
          className="w-full"
          onClick={() => {
            onModeChange('image')
          }}
        >
          {t('git.useImage')}
        </Button>
      </div>
    )
  }

  const showConnect =
    !connected &&
    mode !== 'image' &&
    (mode === 'connect' || tab === 'build-from-source' || !app.image)

  if (showConnect) {
    return (
      <ConnectSourcePanel
        appName={appName}
        onUseImage={() => {
          onModeChange('image')
        }}
      />
    )
  }

  return (
    <ImageDeployPanel
      app={app}
      onClose={onClose}
      onConnectRepo={() => {
        onModeChange(connected ? 'auto' : 'connect')
      }}
    />
  )
}
