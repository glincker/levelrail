import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// vitest.config.ts does not set test.globals, so Testing Library's own
// implicit "afterEach(cleanup)" (which only registers when it finds a
// global afterEach) never runs: every render in a suite otherwise piles
// up in the same jsdom document instead of unmounting between tests.
// Invisible with a single test per file (PR #224's original
// CreateAppFromGitFields.test.tsx), but breaks any suite with 2+ tests
// that render a component, so this is wired here once for every test
// file rather than per-file.
afterEach(() => {
  cleanup()
})
