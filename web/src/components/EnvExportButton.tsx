import { DownloadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { formatEnvExport } from '../lib/envParse'
import { Button } from '@/components/ui/button'

// Downloads the saved variables as a .env file. Secret keys are written
// empty with a comment, their values never leave the server.
export function EnvExportButton({
  env,
  secretKeys,
  filename,
}: {
  env: Record<string, string>
  secretKeys: string[]
  filename: string
}) {
  const handleClick = () => {
    const blob = new Blob([formatEnvExport(env, secretKeys)], {
      type: 'text/plain',
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  return (
    <Button type="button" variant="outline" size="sm" onClick={handleClick}>
      <DownloadSimpleIcon />
      Export .env
    </Button>
  )
}
