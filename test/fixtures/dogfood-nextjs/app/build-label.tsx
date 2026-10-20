"use client";

import { useState } from "react";

export default function BuildLabel() {
  const [clicks, setClicks] = useState(0);
  return (
    <button id="client-build-label" onClick={() => setClicks(clicks + 1)}>
      client-build:{process.env.NEXT_PUBLIC_BUILD_LABEL ?? "unset"} clicks:{clicks}
    </button>
  );
}
