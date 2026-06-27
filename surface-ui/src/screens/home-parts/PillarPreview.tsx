import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router-dom";
import { Pill } from "@dakasa-yggdrasil/surface-toolkit";
import type { PillTone } from "@dakasa-yggdrasil/surface-toolkit";

export interface PillarRow {
  key: string;
  /** Primary label. */
  title: string;
  /** Secondary muted line. */
  sub?: string;
  /** Optional status pill. */
  tagLabel?: string;
  tagTone?: PillTone;
}

export interface PillarPreviewProps {
  kicker: string;
  /** The headline figure (e.g. "ativo", "8", "—"). */
  value: ReactNode;
  /** Small unit after the value (e.g. "mTLS", "needs-work"). */
  unit?: string;
  rows: PillarRow[];
  /** Route this pillar links to (e.g. "/webhook"). */
  to: string;
  /** Empty-rows message — honest, terse. */
  emptyLabel?: string;
}

const CARD: CSSProperties = {
  display: "block",
  textDecoration: "none",
  color: "inherit",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-lg)",
  padding: "var(--sp-5)",
  boxShadow: "var(--sh-soft)",
  containerType: "inline-size"
};

const KICKER: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "space-between",
  width: "100%",
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.12em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

const ROW: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--sp-2)",
  padding: "var(--sp-2) var(--sp-3)",
  background: "var(--sand)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-sm)",
  minWidth: 0
};

// A small per-card hover lift, applied via a scoped stylesheet so the warm
// border + "↗" emphasis match the reference without inline JS handlers. The "↗"
// here is intra-app (it links to the pillar's detail route, not native EFI).
const HOVER = `
  .ef-pillar { transition: border-color 120ms ease, box-shadow 120ms ease, transform 120ms ease; }
  .ef-pillar:hover { border-color: var(--honey); box-shadow: var(--sh-lift, var(--sh-soft)); transform: translateY(-1px); }
  .ef-pillar:hover .ef-pillar-arrow { color: var(--honey); }
`;

/**
 * A calm, restrained pillar card: kicker (+ ↗) → big number → 2-3 supporting
 * rows with hard numbers. The whole card links to the pillar's detail route.
 * No blurb, no editorial — supports the pulse, never competes with the band.
 */
export function PillarPreview({
  kicker,
  value,
  unit,
  rows,
  to,
  emptyLabel = "Sem itens agora."
}: PillarPreviewProps) {
  return (
    <Link to={to} className="ef-pillar" style={CARD} aria-label={kicker}>
      <style>{HOVER}</style>
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
        <span style={KICKER}>
          <span>{kicker}</span>
          <span className="ef-pillar-arrow" aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700 }}>
            ↗
          </span>
        </span>

        <span
          style={{
            fontFamily: "var(--font-heading)",
            fontSize: "var(--fs-2xl)",
            fontWeight: 600,
            lineHeight: 1,
            color: "var(--ink)"
          }}
        >
          {value}
          {unit ? (
            <span style={{ fontFamily: "var(--font-body)", fontSize: "var(--fs-md)", color: "var(--mut)", marginLeft: 6 }}>
              {unit}
            </span>
          ) : null}
        </span>

        <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
          {rows.length === 0 ? (
            <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)" }}>{emptyLabel}</span>
          ) : (
            rows.map((r) => (
              <div key={r.key} style={ROW}>
                <div style={{ display: "flex", flexDirection: "column", minWidth: 0, flex: 1 }}>
                  <span
                    style={{
                      fontSize: "var(--fs-sm)",
                      fontWeight: 500,
                      color: "var(--ink)",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap"
                    }}
                    title={r.title}
                  >
                    {r.title}
                  </span>
                  {r.sub ? <span style={{ fontSize: "var(--fs-xs)", color: "var(--mut)" }}>{r.sub}</span> : null}
                </div>
                {r.tagLabel ? <Pill label={r.tagLabel} tone={r.tagTone ?? "neutral"} preserveCase /> : null}
              </div>
            ))
          )}
        </div>
      </div>
    </Link>
  );
}
