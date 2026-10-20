export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({
    runtimeGreeting: process.env.RUNTIME_GREETING ?? null,
    buildLabel: process.env.NEXT_PUBLIC_BUILD_LABEL ?? null,
  });
}
