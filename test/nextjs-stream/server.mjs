// Streams a chunked HTML body and an SSE feed with a visible gap between chunks.
import http from "node:http";

const port = Number(process.env.PORT ?? 3000);
const gapMs = Number(process.env.GAP_MS ?? 1000);
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

http
  .createServer(async (req, res) => {
    const sse = req.url === "/events";
    res.writeHead(200, { "Content-Type": sse ? "text/event-stream" : "text/html; charset=utf-8" });
    for (let i = 1; i <= 5; i++) {
      res.write(sse ? `data: tick ${i}\n\n` : `<p>chunk ${i} at ${new Date().toISOString()}</p>\n`);
      await sleep(gapMs);
    }
    res.end();
  })
  .listen(port, () => process.stdout.write(`listening on ${port}\n`));
