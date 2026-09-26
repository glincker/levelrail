import type { PreviewCardMeta } from '../types/preview'

export interface PreviewCardProps {
  appName: string
  meta?: PreviewCardMeta
  commitSha?: string
}

const SHA_LENGTH = 7

// PreviewCard is the fallback preview of the metadata mode: no image exists,
// so it is composed here from the page title and description the server kept.
export function PreviewCard({
  appName,
  meta,
  commitSha,
}: Readonly<PreviewCardProps>) {
  return (
    <div
      data-testid="preview-card"
      className="absolute inset-0 flex flex-col gap-1 overflow-hidden bg-muted/60 p-2"
    >
      <div className="flex items-center gap-1.5">
        {meta?.theme_color ? (
          <svg
            className="size-2.5 shrink-0 rounded-full"
            viewBox="0 0 10 10"
            aria-hidden="true"
          >
            <rect width="10" height="10" fill={meta.theme_color} />
          </svg>
        ) : null}
        <span className="truncate text-[11px] font-medium text-foreground">
          {appName}
        </span>
      </div>
      {meta?.title ? (
        <p className="line-clamp-2 text-[11px] leading-snug text-foreground">
          {meta.title}
        </p>
      ) : null}
      {meta?.description ? (
        <p className="line-clamp-2 text-[10px] leading-snug text-muted-foreground">
          {meta.description}
        </p>
      ) : null}
      {commitSha ? (
        <span className="mt-auto font-mono text-[10px] text-muted-foreground">
          {commitSha.slice(0, SHA_LENGTH)}
        </span>
      ) : null}
    </div>
  )
}
