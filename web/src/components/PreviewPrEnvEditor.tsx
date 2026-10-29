import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LockGlassIcon,
  SlidersHorizontalGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useDeletePrPreviewEnv,
  usePrPreviewEnv,
  useSetPrPreviewEnv,
} from '../queries/previewPrEnv'

// PreviewPrEnvEditor sets env vars for one pull request's preview only. They
// apply on that PR's next deploy and are removed with the preview.
export function PreviewPrEnvEditor({
  appName,
  prNumber,
}: {
  appName: string
  prNumber: number
}) {
  const { t } = useTranslation('previews')
  const [open, setOpen] = useState(false)
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const [secret, setSecret] = useState(false)
  const list = usePrPreviewEnv(appName, prNumber, open)
  const setEnv = useSetPrPreviewEnv(appName, prNumber)
  const del = useDeletePrPreviewEnv(appName, prNumber)
  const overrides = list.data ?? []
  const idBase = `pr-env-${prNumber}`

  function save() {
    setEnv.mutate(
      { key: key.trim(), value, secret },
      {
        onSuccess: () => {
          setKey('')
          setValue('')
          setSecret(false)
          toast.add({
            title: t('prEnv.saved', { number: prNumber }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <div className="w-full">
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-expanded={open}
        onClick={() => {
          setOpen((v) => !v)
        }}
      >
        <SlidersHorizontalGlassIcon size="1.2em" />
        {t('prEnv.button')}
      </Button>
      {open ? (
        <div className="mt-2 space-y-3 rounded-md border border-border p-3">
          <p className="text-xs text-muted-foreground">{t('prEnv.help')}</p>
          {list.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{list.error.message}</AlertDescription>
            </Alert>
          ) : overrides.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('prEnv.empty')}</p>
          ) : (
            <ul className="space-y-1">
              {overrides.map((o) => (
                <li
                  key={o.id}
                  className="flex items-center justify-between gap-2 text-sm"
                >
                  <span className="flex min-w-0 flex-wrap items-center gap-2 font-mono">
                    <span>{o.key}</span>
                    {o.secret ? (
                      <span className="flex items-center gap-1 text-muted-foreground">
                        <LockGlassIcon className="size-4" aria-hidden="true" />
                        {t('prEnv.hidden')}
                      </span>
                    ) : (
                      <span className="break-all text-muted-foreground">
                        {o.value}
                      </span>
                    )}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    disabled={del.isPending}
                    onClick={() => {
                      del.mutate(o.id)
                    }}
                    aria-label={t('prEnv.remove', { key: o.key })}
                  >
                    <TrashGlassIcon className="size-4.5 text-muted-foreground" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault()
              save()
            }}
          >
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="space-y-1 text-xs" htmlFor={`${idBase}-key`}>
                <span>{t('prEnv.key')}</span>
                <Input
                  id={`${idBase}-key`}
                  className="font-mono"
                  placeholder="FEATURE_FLAG"
                  autoComplete="off"
                  autoCapitalize="off"
                  spellCheck={false}
                  value={key}
                  onChange={(e) => {
                    setKey(e.target.value)
                  }}
                />
              </label>
              <label className="space-y-1 text-xs" htmlFor={`${idBase}-value`}>
                <span>{t('prEnv.value')}</span>
                <Input
                  id={`${idBase}-value`}
                  type={secret ? 'password' : 'text'}
                  className="font-mono"
                  autoComplete="off"
                  value={value}
                  onChange={(e) => {
                    setValue(e.target.value)
                  }}
                />
              </label>
            </div>
            <label
              className="flex items-center gap-2 text-xs"
              htmlFor={`${idBase}-secret`}
            >
              <Checkbox
                id={`${idBase}-secret`}
                checked={secret}
                onCheckedChange={(checked) => {
                  setSecret(checked === true)
                }}
              />
              {t('prEnv.secret')}
            </label>
            <Button
              type="submit"
              size="sm"
              disabled={setEnv.isPending || key.trim() === '' || value === ''}
            >
              {setEnv.isPending ? t('prEnv.saving') : t('prEnv.save')}
            </Button>
            {setEnv.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{setEnv.error.message}</AlertDescription>
              </Alert>
            ) : null}
          </form>
        </div>
      ) : null}
    </div>
  )
}
