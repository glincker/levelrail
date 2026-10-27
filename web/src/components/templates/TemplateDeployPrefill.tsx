import { useEffect } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { ApiError } from '../../lib/apiError'
import { useQuery } from '@tanstack/react-query'
import { serviceTemplateQueryOptions } from '../../queries/serviceTemplates'

/** Resolves /templates?deploy=<slug> on this instance and opens that template's deploy page, or says it is unknown. */
export function TemplateDeployPrefill({ slug }: { slug: string }) {
  const { t } = useTranslation('templates')
  const navigate = useNavigate()
  const query = useQuery({
    ...serviceTemplateQueryOptions(slug),
    // An unknown slug is a final answer, not worth retrying.
    retry: (count, err) =>
      !(err instanceof ApiError && err.status === 404) && count < 2,
  })

  useEffect(() => {
    if (query.data) {
      void navigate({
        to: '/templates/$id',
        params: { id: query.data.id },
        replace: true,
      })
    }
  }, [query.data, navigate])

  if (query.isError) {
    const notFound =
      query.error instanceof ApiError && query.error.status === 404
    return (
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title={notFound ? t('prefill.notFoundTitle') : t('prefill.errorTitle')}
        description={
          notFound
            ? t('prefill.notFoundDescription', { slug })
            : query.error.message
        }
        action={
          <Button
            nativeButton={false}
            render={<Link to="/templates" search={{ deploy: undefined }} />}
          >
            {t('prefill.browse')}
          </Button>
        }
      />
    )
  }

  return (
    <div role="status" aria-label={t('prefill.loading', { slug })}>
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  )
}
