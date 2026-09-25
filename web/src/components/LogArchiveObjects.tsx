import { DownloadSimpleIcon, FilesIcon } from '@phosphor-icons/react/dist/ssr'
import { logArchiveDownloadUrl, useLogArchiveObjects } from '../queries/storage'

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`
}

// Archived objects for one app (or all apps) in a destination, each
// downloadable as the raw gzip NDJSON the archiver wrote.
export function LogArchiveObjects({
  appName,
  targetId,
}: {
  appName: string
  targetId: string
}) {
  const objects = useLogArchiveObjects(targetId, appName)

  return (
    <section
      aria-labelledby={`archive-objects-${appName}`}
      className="space-y-3"
    >
      <h2
        id={`archive-objects-${appName}`}
        className="flex items-center gap-2 text-sm font-semibold text-foreground"
      >
        <FilesIcon className="size-4" aria-hidden="true" />
        Archived objects
      </h2>
      {objects.isPending ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : objects.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {objects.error.message}
        </p>
      ) : objects.data.objects.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing archived yet. Objects appear here after the first run.
        </p>
      ) : (
        <ul className="rounded-lg border border-border">
          {objects.data.objects.map((o) => (
            <li
              key={o.key}
              className="flex items-center justify-between gap-3 border-b border-border px-3 py-2 text-sm last:border-b-0"
            >
              <span className="min-w-0 truncate font-mono text-xs text-foreground">
                {o.key}
              </span>
              <span className="flex shrink-0 items-center gap-3 text-muted-foreground">
                {formatBytes(o.size)}
                <a
                  href={logArchiveDownloadUrl(targetId, o.key)}
                  download
                  className="inline-flex items-center gap-1 text-foreground underline-offset-4 hover:underline"
                >
                  <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
                  Download
                </a>
              </span>
            </li>
          ))}
        </ul>
      )}
      {objects.data?.next ? (
        <p className="text-xs text-muted-foreground">
          Showing the first 100 objects. Use the CLI (logs ls --cursor) to page
          further.
        </p>
      ) : null}
    </section>
  )
}
