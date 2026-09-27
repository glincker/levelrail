import Image from "next/image";
import BuildLabel from "./build-label";

export const dynamic = "force-dynamic";

export default function Home() {
  const greeting = process.env.RUNTIME_GREETING ?? "unset";
  return (
    <main>
      <h1>dogfood-nextjs</h1>
      <p id="runtime-greeting">runtime:{greeting}</p>
      <p id="server-build-label">server-build:{process.env.NEXT_PUBLIC_BUILD_LABEL ?? "unset"}</p>
      <BuildLabel />
      <Image src="/logo.png" alt="logo" width={64} height={64} priority />
    </main>
  );
}
