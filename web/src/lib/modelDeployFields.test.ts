import { describe, expect, it } from 'vitest'
import { INITIAL_DEPLOY_FORM, deployFieldErrors } from './modelDeployForm'
import { nodeGpuFacts } from './models'

describe('deployFieldErrors', () => {
  it('flags the empty name and model on a fresh form', () => {
    expect(deployFieldErrors(INITIAL_DEPLOY_FORM)).toEqual({
      name: true,
      model: true,
    })
  })

  it('flags bad advanced values individually', () => {
    const errs = deployFieldErrors({
      ...INITIAL_DEPLOY_FORM,
      name: 'chat',
      model: 'llama3.1:8b',
      gpus: 'two',
      context: '-1',
      swapGroup: 'Bad Group',
    })
    expect(errs).toEqual({ gpus: true, context: true, swapGroup: true })
  })
})

describe('nodeGpuFacts', () => {
  it('lists distinct GPU names and free VRAM', () => {
    const facts = nodeGpuFacts({
      devices: [{ name: 'A100' }, { name: 'A100' }],
      total_vram_mib: 81920,
      used_vram_mib: 20480,
    })
    expect(facts).toEqual({
      gpuNames: 'A100',
      freeMib: 61440,
      totalMib: 81920,
    })
  })
})
