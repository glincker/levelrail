// Talks only to the operator's own control plane; no third-party calls.
export interface AppRow {
  name: string;
}

interface ListShape {
  items?: AppRow[];
  apps?: AppRow[];
}

export class Api {
  constructor(
    private readonly baseUrl: string,
    private readonly token: string,
  ) {}

  private headers(): Record<string, string> {
    return { authorization: `Bearer ${this.token}`, "content-type": "application/json" };
  }

  private url(path: string): string {
    return `${this.baseUrl.replace(/\/+$/, "")}${path}`;
  }

  async listApps(): Promise<AppRow[]> {
    const res = await fetch(this.url("/api/v1/apps"), { headers: this.headers() });
    if (!res.ok) throw new Error(`list apps failed: ${res.status}`);
    const body = (await res.json()) as AppRow[] | ListShape;
    if (Array.isArray(body)) return body;
    return body.items ?? body.apps ?? [];
  }

  async deploy(app: string, image: string): Promise<void> {
    const res = await fetch(this.url(`/api/v1/apps/${encodeURIComponent(app)}/deploys`), {
      method: "POST",
      headers: this.headers(),
      body: JSON.stringify({ image }),
    });
    if (!res.ok) throw new Error(`deploy failed: ${res.status}`);
  }

  /** Streams SSE data lines of one deploy's log to onLine until the server closes. */
  async tailDeployLog(app: string, deployId: string, onLine: (line: string) => void, signal: AbortSignal): Promise<void> {
    const path = `/api/v1/apps/${encodeURIComponent(app)}/deploys/${encodeURIComponent(deployId)}/logs`;
    const res = await fetch(this.url(path), { headers: { ...this.headers(), accept: "text/event-stream" }, signal });
    if (!res.ok || !res.body) throw new Error(`log stream failed: ${res.status}`);
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split("\n");
      buf = lines.pop() ?? "";
      for (const l of lines) if (l.startsWith("data:")) onLine(l.slice(5).trimStart());
    }
  }
}
