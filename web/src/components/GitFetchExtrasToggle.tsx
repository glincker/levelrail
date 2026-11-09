import { useTranslation } from 'react-i18next'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import type { GitSourceResource } from '../types/gitSource'
import { useSetGitDeploySettings } from '../queries/gitDeploySettings'

type Field = 'fetch_submodules' | 'fetch_lfs'

const FIELDS: { field: Field; key: 'submodules' | 'lfs' }[] = [
  { field: 'fetch_submodules', key: 'submodules' },
  { field: 'fetch_lfs', key: 'lfs' },
]

// Submodule and LFS fetch toggles; each switch saves on change.
export function GitFetchExtrasToggle({
  appName,
  source,
}: {
  appName: string
  source: GitSourceResource
}) {
  const { t } = useTranslation('gitSource')
  const save = useSetGitDeploySettings(appName)
  const onChange = (field: Field, checked: boolean) => {
    save.mutate(
      { [field]: checked },
      {
        onSuccess: () =>
          toast.add({ title: t('fetchExtras.saved'), type: 'success' }),
        onError: (err) =>
          toast.add({
            title: t('fetchExtras.failed'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }
  return (
    <fieldset className="space-y-2 rounded-xl border p-3">
      <legend className="px-1 text-sm font-medium">
        {t('fetchExtras.title')}
      </legend>
      <p className="text-muted-foreground text-xs">{t('fetchExtras.note')}</p>
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        {FIELDS.map(({ field, key }) => (
          <div key={field} className="flex items-center gap-2">
            <Switch
              id={`git-fetch-${key}`}
              checked={source[field] === true}
              disabled={save.isPending}
              onCheckedChange={(checked) => onChange(field, checked)}
            />
            <Label htmlFor={`git-fetch-${key}`}>
              {t(`fetchExtras.${key}`)}
            </Label>
          </div>
        ))}
      </div>
    </fieldset>
  )
}
