import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Input } from '@/components/ui/input'
import { searchDocs } from '../lib/docsSearch'
import type { DocsManifest } from '../types/docs'

// Plain client-side search over the bundled manifest's titles and
// headings (docsSearch.ts), scoped to /help: no network call, works
// offline the same as the rest of this route. Takes the manifest as a
// prop (loaded by routes/help.tsx's own loader) rather than importing it
// directly, so this component never pulls the manifest into whichever
// chunk happens to import it.
export function HelpSearchBox({ manifest }: { manifest: DocsManifest }) {
  const [query, setQuery] = useState('')
  const results = useMemo(() => searchDocs(manifest, query), [query, manifest])

  return (
    <div className="relative">
      <MagnifyingGlassIcon className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
      <Input
        value={query}
        onChange={(event) => {
          setQuery(event.target.value)
        }}
        placeholder="Search docs..."
        className="pl-8"
        aria-label="Search docs"
      />
      {query.trim() ? (
        <div className="absolute top-full right-0 left-0 z-20 mt-1 max-h-80 overflow-y-auto rounded-lg border border-border bg-popover p-1 shadow-md">
          {results.length === 0 ? (
            <p className="px-2 py-3 text-sm text-muted-foreground">
              No matches for &ldquo;{query}&rdquo;
            </p>
          ) : (
            results.slice(0, 20).map((result, index) => (
              <Link
                key={`${result.path}-${result.heading?.id ?? 'title'}-${index}`}
                to="/help/$"
                params={{ _splat: result.path.slice(1) }}
                hash={result.heading?.id}
                onClick={() => {
                  setQuery('')
                }}
                className="block rounded-md px-2 py-1.5 text-sm hover:bg-muted"
              >
                <span className="text-foreground">{result.title}</span>
                {result.heading ? (
                  <span className="ml-1.5 text-muted-foreground">
                    &rsaquo; {result.heading.text}
                  </span>
                ) : null}
              </Link>
            ))
          )}
        </div>
      ) : null}
    </div>
  )
}
