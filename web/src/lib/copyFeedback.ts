const raw = Number(import.meta.env.VITE_COPY_FEEDBACK_MS)

/** How long a "Copied" confirmation stays; override with VITE_COPY_FEEDBACK_MS. */
export const COPY_FEEDBACK_MS = Number.isFinite(raw) && raw > 0 ? raw : 1500
