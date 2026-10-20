import { describe, expect, it } from 'vitest'
import {
  INITIAL_DEPLOY_FORM,
  buildCreateRequest,
  validateDeployForm,
  type DeployFormState,
} from './modelDeployForm'

const valid: DeployFormState = {
  ...INITIAL_DEPLOY_FORM,
  name: 'chat',
  model: 'llama3.1:8b',
}

describe('validateDeployForm', () => {
  it.each([
    [{}, null],
    [{ name: 'Chat' }, 'Name must be'],
    [{ name: '' }, 'Name must be'],
    [{ name: '-x' }, 'Name must be'],
    [{ model: '  ' }, 'Model is required'],
    [{ gpus: '0' }, 'GPUs must be'],
    [{ gpus: 'lots' }, 'GPUs must be'],
    [{ gpus: '2' }, null],
    [{ context: 'abc' }, 'Context length'],
    [{ context: '8192' }, null],
    [{ residency: 'on_demand' as const, idleMinutes: 'x' }, 'Idle time'],
    [{ residency: 'on_demand' as const, idleMinutes: '30' }, null],
    [{ residency: 'always' as const, idleMinutes: 'x' }, null],
    [{ swapGroup: 'Not Valid!' }, 'Swap group must be'],
    [{ swapGroup: 'gpu0' }, null],
    [{ swapGroup: '' }, null],
  ])('%j -> %s', (patch, want) => {
    const got = validateDeployForm({ ...valid, ...patch })
    if (want === null) expect(got).toBeNull()
    else expect(got).toContain(want)
  })
})

describe('buildCreateRequest', () => {
  it('sends all GPUs as -1 and omits empty optionals', () => {
    expect(buildCreateRequest(valid)).toEqual({
      name: 'chat',
      engine: 'ollama',
      model: 'llama3.1:8b',
      gpu_count: -1,
    })
  })

  it('includes node, context, domain and a token only where they apply', () => {
    const req = buildCreateRequest({
      ...valid,
      engine: 'vllm',
      model: 'org/m',
      node: 'node-2',
      gpus: '2',
      context: '4096',
      quantization: ' awq ',
      domain: ' llm.example.com ',
      hfToken: ' hf_x ',
    })
    expect(req).toEqual({
      name: 'chat',
      engine: 'vllm',
      model: 'org/m',
      node_id: 'node-2',
      gpu_count: 2,
      context_length: 4096,
      quantization: 'awq',
      domain: 'llm.example.com',
      hf_token: 'hf_x',
    })
  })

  it('never sends a token or quantization for ollama', () => {
    const req = buildCreateRequest({
      ...valid,
      hfToken: 'hf_x',
      quantization: 'awq',
    })
    expect(req.hf_token).toBeUndefined()
    expect(req.quantization).toBeUndefined()
  })

  it('sends residency and idle seconds only for on-demand', () => {
    expect(
      buildCreateRequest({
        ...valid,
        residency: 'on_demand',
        idleMinutes: '30',
      }),
    ).toMatchObject({ residency: 'on_demand', idle_ttl_seconds: 1800 })
    const always = buildCreateRequest({ ...valid, idleMinutes: '30' })
    expect(always.residency).toBeUndefined()
    expect(always.idle_ttl_seconds).toBeUndefined()
    const dflt = buildCreateRequest({ ...valid, residency: 'on_demand' })
    expect(dflt.residency).toBe('on_demand')
    expect(dflt.idle_ttl_seconds).toBeUndefined()
  })

  it('includes swap_group only when set', () => {
    expect(buildCreateRequest(valid).swap_group).toBeUndefined()
    expect(
      buildCreateRequest({ ...valid, swapGroup: ' gpu0 ' }).swap_group,
    ).toBe('gpu0')
  })
})
