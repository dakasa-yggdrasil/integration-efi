import type { CSSProperties } from "react";
import { Link } from "react-router-dom";
import { Pill } from "@dakasa-yggdrasil/surface-toolkit";
import type { PillTone } from "@dakasa-yggdrasil/surface-toolkit";

/** A single nav destination — a compact card linking to a detail page. */
export interface NavCardSpec {
  key: string;
  /** Card label (the destination name). */
  label: string;
  /** The hard number (or terse string like "—") shown large. */
  value: number | string;
  /** Small unit after the value (e.g. "mTLS", "needs-work"). */
  unit?: string;
  /** Route this card links to. */
  to: string;
  /** Optional status pill (a bad signal worth flagging on Home). */
  tagLabel?: string;
  tagTone?: PillTone;
}

/** A titled group of nav cards (e.g. "Ingestão", "Dinheiro"). */
export interface NavGroupSpec {
  key: string;
  title: string;
  cards: NavCardSpec[];
}

export interface NavGroupsProps {
  groups: NavGroupSpec[];
}

const GROUPS_CSS = `
  .ef-nav-group { display: flex; flex-direction: column; gap: var(--sp-3); }
  .ef-nav-grid {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  @container (max-width: 460px) { .ef-nav-grid { grid-template-columns: 1fr; } }
  .ef-nav-card { transition: border-color 120ms ease, box-shadow 120ms ease, transform 120ms ease; }
  .ef-nav-card:hover { border-color: var(--honey); box-shadow: var(--sh-lift, var(--sh-soft)); transform: translateY(-1px); }
  .ef-nav-card:hover .ef-nav-arrow { color: var(--honey); }
  .ef-nav-card:hover .ef-nav-label { color: var(--honey); }
`;

const GROUP_TITLE: CSSProperties = {
  margin: 0,
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.14em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

const CARD: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-2)",
  textDecoration: "none",
  color: "inherit",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)",
  padding: "var(--sp-4)",
  boxShadow: "var(--sh-soft)",
  minWidth: 0
};

function NavCard({ card }: { card: NavCardSpec }) {
  return (
    <Link to={card.to} className="ef-nav-card" style={CARD} aria-label={card.label}>
      <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "space-between", gap: "var(--sp-2)" }}>
        <span
          className="ef-nav-label"
          style={{ fontSize: "var(--fs-sm)", fontWeight: 600, color: "var(--ink)", transition: "color 100ms ease" }}
        >
          {card.label}
        </span>
        {/* intra-app navigation arrow (links to the detail route, not native EFI) */}
        <span className="ef-nav-arrow" aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700 }}>
          ↗
        </span>
      </span>
      <span style={{ display: "inline-flex", alignItems: "baseline", gap: "var(--sp-2)", minWidth: 0, flexWrap: "wrap" }}>
        <span
          style={{
            fontFamily: "var(--font-heading)",
            fontSize: "var(--fs-xl)",
            fontWeight: 600,
            lineHeight: 1,
            color: "var(--ink)"
          }}
        >
          {card.value}
        </span>
        {card.unit ? <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)" }}>{card.unit}</span> : null}
        {card.tagLabel ? (
          <span style={{ marginLeft: "auto" }}>
            <Pill label={card.tagLabel} tone={card.tagTone ?? "neutral"} preserveCase />
          </span>
        ) : null}
      </span>
    </Link>
  );
}

/**
 * The grouped Home navigation: titled sections (Ingestão / Dinheiro), each a
 * compact grid of nav cards. Every card shows a hard number (or honest "—" for
 * an un-readable fact) + a hover-revealed "↗" and links to its detail page — a
 * calm, scannable index. Bad signals (webhook sem mTLS, charges needs-work)
 * carry a status pill so the operator's eye lands on them first. Stays
 * technical, no AI-look copy.
 */
export function NavGroups({ groups }: NavGroupsProps) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-5)" }}>
      <style>{GROUPS_CSS}</style>
      {groups.map((g) => (
        <div key={g.key} className="ef-nav-group">
          <h3 style={GROUP_TITLE}>{g.title}</h3>
          <div className="ef-nav-grid">
            {g.cards.map((c) => (
              <NavCard key={c.key} card={c} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
