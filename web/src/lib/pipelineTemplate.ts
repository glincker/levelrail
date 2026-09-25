// Starter pipeline shown in the editor for a new pipeline: test, then build,
// then deploy behind a manual approval.
export const NEW_PIPELINE_YAML = `version: 1
name: release
on:
  push:
    branches: [main]
  manual: {}
stages: [test, build, deploy]
jobs:
  test:
    stage: test
    image: golang:1.23
    steps:
      - run: go test ./...
  build:
    stage: build
    steps:
      - uses: build
  ship:
    stage: deploy
    steps:
      - uses: approval
        with:
          message: Deploy to production?
          approvers: deploy
      - uses: deploy
`

export function lineOffsets(text: string, line: number): [number, number] {
  const lines = text.split('\n')
  const idx = Math.min(Math.max(line, 1), lines.length) - 1
  let start = 0
  for (let i = 0; i < idx; i++) {
    start += (lines[i]?.length ?? 0) + 1
  }
  return [start, start + (lines[idx]?.length ?? 0)]
}

export function parseInputLines(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    const eq = line.indexOf('=')
    if (line && eq > 0) {
      out[line.slice(0, eq).trim()] = line.slice(eq + 1).trim()
    }
  }
  return out
}
