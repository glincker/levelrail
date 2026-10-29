import { useTranslation } from 'react-i18next'
import { useRouter } from '@tanstack/react-router'
import {
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useValidateGitAccess, type GitAccessCheck } from '../queries/gitAccess'

function CheckIcon({ status }: { status: GitAccessCheck['status'] }) {
  if (status === 'pass') {
    return (
      <CircleCheckGlassIcon
        className="size-4.5 shrink-0 text-green-600"
        aria-hidden="true"
      />
    )
  }
  if (status === 'fail') {
    return (
      <CircleXGlassIcon
        className="size-4.5 shrink-0 text-destructive"
        aria-hidden="true"
      />
    )
  }
  return (
    <CircleMinusGlassIcon
      className="size-4.5 shrink-0 text-muted-foreground"
      aria-hidden="true"
    />
  )
}

export function GitAccessValidate({ path }: { path: string }) {
  const { t } = useTranslation('common')
  const router = useRouter()
  const validate = useValidateGitAccess(path)
  const res = validate.data
  const fix = res?.fix
  const link = fix?.link

  return (
    <div className="space-y-3">
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={validate.isPending}
        onClick={() => {
          validate.mutate()
        }}
      >
        <ShieldCheckGlassIcon
          className="size-4 text-tone-success"
          aria-hidden="true"
        />
        {validate.isPending
          ? t('gitAccess.validating')
          : res
            ? t('gitAccess.recheck')
            : t('gitAccess.validate')}
      </Button>
      {validate.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{validate.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {res ? (
        <div className="space-y-2 text-sm" role="status">
          <ul className="space-y-1.5">
            {res.checks.map((c) => (
              <li key={c.id} className="flex items-start gap-2">
                <CheckIcon status={c.status} />
                <span>
                  <span className="font-medium">
                    {t(`gitAccess.checks.${c.id}`)}
                  </span>
                  <span className="block text-muted-foreground">
                    {c.detail}
                  </span>
                </span>
              </li>
            ))}
          </ul>
          {res.ok ? (
            <p className="font-medium text-green-700 dark:text-green-400">
              {t('gitAccess.ok')}
            </p>
          ) : null}
          {fix ? (
            <Alert variant="destructive">
              <AlertDescription className="space-y-2">
                <span className="block">{fix.hint}</span>
                {link ? (
                  <Button
                    type="button"
                    size="sm"
                    onClick={() => {
                      router.history.push(link)
                    }}
                  >
                    {t('gitAccess.fix')}
                  </Button>
                ) : null}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
