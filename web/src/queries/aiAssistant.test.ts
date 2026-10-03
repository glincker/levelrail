import { describe, expect, it } from 'vitest'
import { streamAiSseResponse, type AiSseEvent } from './aiAssistant'

// Builds a fake fetch Response whose body streams the exact bytes
// internal/api/ai_chat.go's aiSSESink.write produces: a bare
// "data: <json>\n\n" per event, no named `event:` line, discriminated
// purely by the JSON payload's own `type` field. Regression coverage for
// a real bug where streamAiSseResponse required an `event:` line and
// silently dropped every real event from the backend.
function fakeSseResponse(frames: string[]): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder()
      for (const frame of frames) {
        controller.enqueue(encoder.encode(frame))
      }
      controller.close()
    },
  })
  return new Response(stream)
}

describe('streamAiSseResponse', () => {
  it('parses bare data: frames with no event: line, matching the real backend wire format', async () => {
    const frames = [
      ': connected\n\n',
      'data: {"type":"text_delta","text":"Hello"}\n\n',
      'data: {"type":"tool_call_proposed","confirmation_id":"conf_1","tool_use_id":"call_1","name":"rollback_deploy","arguments":{"app":"demo"},"read_only":false}\n\n',
      'data: {"type":"tool_result","tool_use_id":"call_1","name":"rollback_deploy","result":"ok","is_error":false}\n\n',
      'data: {"type":"done"}\n\n',
    ]
    const events: AiSseEvent[] = []
    await streamAiSseResponse(fakeSseResponse(frames), (ev) => events.push(ev))

    expect(events).toEqual([
      { type: 'text_delta', content: 'Hello' },
      {
        type: 'tool_call_proposed',
        toolCall: {
          confirmation_id: 'conf_1',
          tool_name: 'rollback_deploy',
          arguments: { app: 'demo' },
          read_only: false,
        },
      },
      {
        type: 'tool_result',
        confirmationId: '',
        result: 'ok',
        isError: false,
      },
      { type: 'done' },
    ])
  })

  it('still honors a named event: line if one is ever present', async () => {
    const events: AiSseEvent[] = []
    await streamAiSseResponse(
      fakeSseResponse(['event: done\ndata: {}\n\n']),
      (ev) => events.push(ev),
    )
    expect(events).toEqual([{ type: 'done' }])
  })

  it('ignores a frame with no data: line', async () => {
    const events: AiSseEvent[] = []
    await streamAiSseResponse(fakeSseResponse([': connected\n\n']), (ev) =>
      events.push(ev),
    )
    expect(events).toEqual([])
  })
})
