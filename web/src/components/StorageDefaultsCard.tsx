import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useClearStorageDefault,
  useSetStorageDefault,
  useStorageDefaults,
} from '../queries/storage'
import type { StorageDataClass, StorageDestination } from '../types/storage'

const NODE_CLASSES: StorageDataClass[] = ['backups', 'dumps']

export function StorageDefaultsCard({
  destinations,
}: {
  destinations: StorageDestination[]
}) {
  const { t } = useTranslation('settings')
  const defaults = useStorageDefaults()
  const set = useSetStorageDefault()
  const clear = useClearStorageDefault()
  if (!defaults.data || destinations.length === 0) return null
  const current = new Map(defaults.data.defaults.map((d) => [d.data_class, d]))

  return (
    <div className="space-y-3">
      <h2 className="text-base font-semibold text-foreground">
        {t('storageDestinations.defaults.title')}
      </h2>
      <p className="text-sm text-muted-foreground">
        {t('storageDestinations.defaults.description')}
      </p>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {defaults.data.classes.map((cls) => {
          const choice = current.get(cls)?.target_id ?? ''
          const className = t(`storageDestinations.classes.${cls}`)
          return (
            <li
              key={cls}
              className="flex flex-wrap items-center justify-between gap-3 p-3"
            >
              <span className="text-sm font-medium text-foreground">
                {className}
              </span>
              <div className="flex items-center gap-2">
                <Select
                  value={choice}
                  onValueChange={(targetId) => {
                    if (!targetId) return
                    set.mutate(
                      { dataClass: cls, targetId },
                      {
                        onSuccess: () => {
                          const picked = destinations.find(
                            (d) => d.id === targetId,
                          )
                          toast.add({
                            title: t('storageDestinations.defaults.saved', {
                              class: className,
                              name: picked?.name ?? targetId,
                            }),
                            type: 'success',
                          })
                        },
                        onError: (error) => {
                          toast.add({ title: error.message, type: 'error' })
                        },
                      },
                    )
                  }}
                >
                  <SelectTrigger
                    className="w-56"
                    aria-label={className}
                    disabled={set.isPending}
                  >
                    <SelectValue
                      placeholder={t('storageDestinations.defaults.unset')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {destinations
                      .filter(
                        (d) => d.kind === 's3' || NODE_CLASSES.includes(cls),
                      )
                      .map((d) => (
                        <SelectItem key={d.id} value={d.id}>
                          {d.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                {choice ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={clear.isPending}
                    onClick={() => {
                      clear.mutate(cls, {
                        onSuccess: () => {
                          toast.add({
                            title: t('storageDestinations.defaults.cleared', {
                              class: className,
                            }),
                            type: 'success',
                          })
                        },
                      })
                    }}
                  >
                    {t('storageDestinations.defaults.clear')}
                  </Button>
                ) : null}
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
