import type { CSSProperties } from "react";

export interface FilterOption {
  /** The value to select (null = "all" / reset). */
  value: string | null;
  /** The chip label. */
  label: string;
  /** Count subscript. */
  count: number;
}

export interface FilterPillsProps {
  /** Accessible group label. */
  ariaLabel: string;
  options: FilterOption[];
  selected: string | null;
  onSelect: (v: string | null) => void;
}

function chipStyle(active: boolean): CSSProperties {
  return {
    display: "inline-flex",
    alignItems: "center",
    gap: "var(--sp-1)",
    padding: "var(--sp-1) var(--sp-3)",
    borderRadius: "999px",
    border: `1px solid ${active ? "var(--honey)" : "var(--line)"}`,
    background: active ? "var(--honey)" : "var(--cream)",
    color: active ? "var(--cream)" : "var(--body)",
    fontFamily: "var(--font-body)",
    fontSize: "var(--fs-sm)",
    fontWeight: 600,
    cursor: "pointer",
    transition: "border-color 100ms ease, background 100ms ease"
  };
}

function countStyle(active: boolean): CSSProperties {
  return {
    fontSize: "var(--fs-xs)",
    fontWeight: 700,
    color: active ? "var(--cream)" : "var(--mut)"
  };
}

/**
 * A pill-row filter. Selection is local state, so it never refetches — the table
 * just filters the already-loaded rows.
 */
export function FilterPills({ ariaLabel, options, selected, onSelect }: FilterPillsProps) {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--sp-2)" }} role="group" aria-label={ariaLabel}>
      {options.map((o) => {
        const active = selected === o.value;
        return (
          <button
            key={o.value ?? "__all__"}
            type="button"
            style={chipStyle(active)}
            onClick={() => onSelect(o.value)}
            aria-pressed={active}
          >
            <span>{o.label}</span>
            <span style={countStyle(active)}>{o.count}</span>
          </button>
        );
      })}
    </div>
  );
}
