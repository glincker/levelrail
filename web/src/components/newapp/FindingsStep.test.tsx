import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FindingsStep } from './FindingsStep'
import { NewAppI18n } from '../../test/newAppI18n'
import { nextAnalysis } from '../../test/newAppFixtures'
import { editsFromAnalysis } from '../../lib/newApp'
import type { RawImportAnalysis } from '../../queries/importAnalyze'

describe('FindingsStep', () => {
  it('renders when the server sends null for list fields', () => {
    const raw: RawImportAnalysis = {
      ...nextAnalysis,
      env: null,
      databases: null,
      warnings: null,
      build: { ...nextAnalysis.build, alternatives: null },
    }
    render(
      <NewAppI18n>
        <FindingsStep
          analysis={raw}
          source={{
            provider: 'manual',
            repoUrl: 'https://github.com/vercel/next-learn',
            branch: 'main',
          }}
          edits={editsFromAnalysis({ ...nextAnalysis, env: [], databases: [] })}
          onEdits={vi.fn()}
          onReanalyze={vi.fn()}
          onBack={vi.fn()}
          onNext={vi.fn()}
        />
      </NewAppI18n>,
    )
    expect(
      screen.getByRole('heading', { name: /environment/i }),
    ).toBeInTheDocument()
  })
})
