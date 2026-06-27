import type { ReactNode } from "react";
import type { KpiDelta } from "@dakasa-yggdrasil/surface-toolkit";

/**
 * KPI polish helper. The toolkit's `KpiTile` renders a directional glyph
 * (↑ ok / ↓ crit / → mut) whenever a `delta` is present — which mis-reads a
 * neutral fact: a "↓ webhooks" next to "1" looks like a regression when it is
 * just a count. So we split the qualifier two ways:
 *
 *  - **Bad signal** (`bad: true`) → a real {@link KpiDelta} with `dir: "down"`,
 *    so the tile shows the crit-colored ↓ that genuinely flags "needs you"
 *    (e.g. an absent webhook, or mTLS off).
 *  - **Neutral / good fact** → no `delta` at all; the qualifier rides the
 *    `chart` slot as a plain muted line with NO arrow.
 *
 * `kpiDelta` returns the delta to spread into `delta={...}` (or `undefined`),
 * and `kpiSubtext` returns the muted node for the `chart` slot (or `undefined`).
 * Exactly one of them is non-undefined for a given (text, bad) pair.
 */
export function kpiDelta(text: string, bad: boolean): KpiDelta | undefined {
  return bad ? { dir: "down", text } : undefined;
}

const SUBTEXT_STYLE = {
  fontFamily: "var(--font-body)",
  fontSize: "var(--fs-sm)",
  fontWeight: 600,
  color: "var(--mut)"
} as const;

export function kpiSubtext(text: string, bad: boolean): ReactNode {
  if (bad) return undefined;
  return <span style={SUBTEXT_STYLE}>{text}</span>;
}
