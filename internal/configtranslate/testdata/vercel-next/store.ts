import { kv } from "@vercel/kv";
import { ImageResponse } from "@vercel/og";
import { Analytics } from "@vercel/analytics/react";

export async function hits(): Promise<number> {
  return kv.incr("hits");
}
