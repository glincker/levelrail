import { readFileSync } from "node:fs";

export const dynamic = "force-dynamic";

export function GET() {
  return new Response(readFileSync("MARKER", "utf8"));
}
