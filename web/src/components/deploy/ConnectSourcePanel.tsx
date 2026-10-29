import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { BoxGlassIcon, GitBranchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { gitSourceKeys } from '../../queries/gitSources'
import {
  GitRepoSourcePicker,
  type GitRepoSourceValue,
} from '../GitRepoSourcePicker'
import { connectPickedRepo } from './connectSource'

export function ConnectSourcePanel({
  appName,
  onUseImage,
}: {
  appName: string
  onUseImage: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const queryClient = useQueryClient()
  const [picked, setPicked] = useState<GitRepoSourceValue | null>(null)
  const connect = useMutation({
    mutationFn: (value: GitRepoSourceValue) =>
      connectPickedRepo(appName, value),
    onSuccess: (resource) => {
      queryClient.setQueryData(gitSourceKeys.detail(appName), resource)
      toast.add({ title: t('connect.connected'), type: 'success' })
    },
  })

  return (
    <div className="space-y-5">
      <section className="space-y-3">
        <div>
          <h3 className="flex items-center gap-2 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
            <GitBranchGlassIcon className="size-4.5" aria-hidden="true" />
            {t('connect.repoTitle')}
          </h3>
          <p className="text-sm text-muted-foreground">
            {t('connect.repoDescription')}
          </p>
        </div>
        <GitRepoSourcePicker
          disabled={connect.isPending}
          onSelect={setPicked}
        />
        {connect.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('connect.failed')} {connect.error.message}
            </AlertDescription>
          </Alert>
        ) : null}
        <Button
          className="w-full"
          disabled={!picked?.repoUrl || connect.isPending}
          onClick={() => {
            if (picked) connect.mutate(picked)
          }}
        >
          {connect.isPending ? t('connect.connecting') : t('connect.connect')}
        </Button>
      </section>
      <section className="space-y-2 border-t border-border pt-4">
        <h3 className="flex items-center gap-2 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          <BoxGlassIcon className="size-4.5" aria-hidden="true" />
          {t('connect.orImage')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('connect.imageDescription')}
        </p>
        <Button variant="outline" className="w-full" onClick={onUseImage}>
          {t('image.deploy')}
        </Button>
      </section>
    </div>
  )
}
