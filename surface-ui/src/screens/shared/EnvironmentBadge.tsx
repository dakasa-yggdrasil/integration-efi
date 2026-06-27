import type { CSSProperties } from "react";
import type { EfiEnvironment } from "../../data";

export interface EnvironmentBadgeProps {
  env: EfiEnvironment;
  /** Render a larger badge (Home header) vs the compact tier-2 chip. */
  size?: "lg" | "sm";
}

/**
 * The MANDATORY, prominent environment badge (rule #0): homolog vs prod. EFI
 * homologation (`sandbox: true`, `pix-h` base URL) is the safe rail where
 * money-movement would be REFUSED; production (`sandbox: false`) is real money.
 * The operator must always see which rail they are on, so this badge is loud,
 * not a quiet pill — homolog gets the warm honey tone, prod gets the crit tone
 * (deliberately attention-grabbing: prod moves real Pix money).
 *
 * The environment is honest: in the live path it is not yet readable from a
 * surface query (it is instance CONFIG, `sandbox`), so {@link useEnvironment}
 * defaults to the safe `homolog` and this badge says `homolog` until a future
 * surface read exposes the instance config. We never claim `prod` without the
 * read.
 */
export function EnvironmentBadge({ env, size = "lg" }: EnvironmentBadgeProps) {
  const isProd = env === "prod";
  const lg = size === "lg";

  const wrap: CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    gap: "var(--sp-2)",
    padding: lg ? "var(--sp-2) var(--sp-3)" : "2px var(--sp-2)",
    borderRadius: "999px",
    border: `1px solid ${isProd ? "var(--crit)" : "var(--honey)"}`,
    background: isProd ? "var(--crit)" : "var(--honey)",
    color: "var(--cream)",
    fontFamily: "var(--font-body)",
    fontSize: lg ? "var(--fs-sm)" : "var(--fs-xs)",
    fontWeight: 700,
    letterSpacing: "0.04em",
    textTransform: "uppercase",
    lineHeight: 1.1,
    whiteSpace: "nowrap"
  };

  const dot: CSSProperties = {
    width: lg ? 8 : 6,
    height: lg ? 8 : 6,
    borderRadius: "50%",
    background: "var(--cream)",
    flex: "0 0 auto",
    opacity: 0.9
  };

  return (
    <span
      style={wrap}
      title={
        isProd
          ? "Produção — Pix real (sandbox: false)."
          : "Homologação (sandbox: true, base pix-h). Dinheiro recusado aqui."
      }
    >
      <span aria-hidden="true" style={dot} />
      {isProd ? "Prod" : "Homolog"}
    </span>
  );
}
