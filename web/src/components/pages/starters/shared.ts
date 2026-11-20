// Shared base styles so each starter stays small; colors follow the visitor's light or dark setting.
export const BASE_CSS = `:root{color-scheme:light dark;--bg:oklch(1 0 0);--fg:oklch(0.2 0.02 265);--muted:oklch(0.5 0.03 265);--line:oklch(0.92 0.01 265);--accent:oklch(0.55 0.2 265);--accent-fg:oklch(1 0 0);--card:oklch(0.97 0.005 265)}
@media(prefers-color-scheme:dark){:root{--bg:oklch(0.18 0.015 265);--fg:oklch(0.95 0.01 265);--muted:oklch(0.72 0.03 265);--line:oklch(0.3 0.02 265);--accent:oklch(0.72 0.14 265);--accent-fg:oklch(0.18 0.015 265);--card:oklch(0.22 0.015 265)}}
*{box-sizing:border-box}
body{margin:0;font:16px/1.6 system-ui,-apple-system,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
a{color:var(--accent)}
a:focus-visible,button:focus-visible{outline:3px solid var(--accent);outline-offset:2px}
.wrap{max-width:60rem;margin:0 auto;padding:0 1.25rem}
.btn{display:inline-block;padding:.65rem 1.1rem;border-radius:.5rem;background:var(--accent);color:var(--accent-fg);text-decoration:none;font-weight:600}
.muted{color:var(--muted)}
@media(prefers-reduced-motion:reduce){*{scroll-behavior:auto!important;transition:none!important}}`

export interface StarterFile {
  path: string
  content: string
}

export interface Starter {
  id: 'blank' | 'landing' | 'docs' | 'portfolio'
  /** Builds the files for a site; the name is escaped by the caller. */
  files: (siteName: string) => StarterFile[]
}

export function page(title: string, body: string, extraCss = ''): string {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>${title}</title>
<style>${BASE_CSS}${extraCss}</style>
</head>
<body>
${body}
</body>
</html>
`
}

export function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
}
