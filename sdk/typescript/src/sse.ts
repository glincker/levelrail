export interface SseEvent {
  event: string;
  id: string | undefined;
  /** Parsed JSON when the payload is JSON, otherwise the raw text. */
  data: unknown;
}

/** Parses a text/event-stream body into events. Stops when the body ends. */
export async function* parseSse(body: ReadableStream<Uint8Array>): AsyncGenerator<SseEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true }).replace(/\r\n?/g, "\n");
      let idx: number;
      while ((idx = buf.indexOf("\n\n")) >= 0) {
        const ev = parseBlock(buf.slice(0, idx));
        buf = buf.slice(idx + 2);
        if (ev) yield ev;
      }
    }
    const tail = parseBlock(buf);
    if (tail) yield tail;
  } finally {
    await reader.cancel().catch(() => undefined);
  }
}

function parseBlock(block: string): SseEvent | undefined {
  let event = "message";
  let id: string | undefined;
  const data: string[] = [];
  for (const line of block.split("\n")) {
    if (!line || line.startsWith(":")) continue;
    const c = line.indexOf(":");
    const field = c < 0 ? line : line.slice(0, c);
    const val = c < 0 ? "" : line.slice(c + 1).replace(/^ /, "");
    if (field === "event") event = val;
    else if (field === "id") id = val;
    else if (field === "data") data.push(val);
  }
  if (data.length === 0) return undefined;
  const raw = data.join("\n");
  try {
    return { event, id, data: JSON.parse(raw) };
  } catch {
    return { event, id, data: raw };
  }
}
