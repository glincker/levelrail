import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'

// PreviewLogsLink opens a preview app's own logs page.
export function PreviewLogsLink({ previewApp }: { previewApp: string }) {
  const { t } = useTranslation('previews')
  return (
    <Button
      size="sm"
      variant="ghost"
      nativeButton={false}
      render={<Link to="/apps/$name/logs" params={{ name: previewApp }} />}
    >
      {t('row.logs')}
    </Button>
  )
}
