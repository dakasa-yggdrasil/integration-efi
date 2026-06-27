import type { CSSProperties } from "react";
import { CapabilityGate } from "@dakasa-yggdrasil/surface-toolkit";
import type { EfiEnvironment } from "../../data";

export interface GatedActionProps {
  /** Capability the viewer must hold for this affordance to render at all. */
  need: string;
  /** The viewer's held perms (from useCollaboratorScope().perms). */
  perms: string[];
  /** The instance environment — drives the homolog-refusal line. */
  env: EfiEnvironment;
  /** Eyebrow (e.g. "Remediação"). */
  eyebrow: string;
  /** The ops-remediation line — what this WOULD do, framed as remediation. */
  label: string;
  /** The disabled button caption (defaults to "Em breve"). */
  cta?: string;
  /** Tooltip on the disabled button. */
  hint?: string;
}

const CARD: CSSProperties = {
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)",
  padding: "var(--sp-4) var(--sp-5)",
  fontSize: "var(--fs-sm)",
  color: "var(--body)"
};

const EYEBROW: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.2em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

const REFUSE_NOTE: CSSProperties = {
  marginTop: "var(--sp-3)",
  display: "flex",
  alignItems: "center",
  gap: "var(--sp-2)",
  fontSize: "var(--fs-xs)",
  color: "var(--mut)",
  lineHeight: 1.5
};

/**
 * The money-movement affordance, gated + disabled. RULE #0 (HARDEST here):
 * refund / payout are admin-tier and OUT of v1, so this is NEVER a transactional
 * button — it renders only for a viewer who holds the capability (so the
 * read-first surface doesn't tease an action they can't have), and even then it
 * is a disabled "Em breve" framed as ops-remediation, not a "Pagar" / "Estornar"
 * call to action.
 *
 * It ALSO states, prominently, that the action would be REFUSED while the
 * instance is in `homolog` — EFI homologation never moves real money, so even
 * once the write path is wired the button stays refused on the homolog rail.
 * And the surface NEVER decides who/how-much: the value + destination of a
 * payout come from the cash-loop workflow, not from here.
 */
export function GatedAction({ need, perms, env, eyebrow, label, cta = "Em breve", hint }: GatedActionProps) {
  const homolog = env !== "prod";
  return (
    <CapabilityGate need={need} perms={perms} fallback={null}>
      <section style={CARD}>
        <span style={EYEBROW}>{eyebrow}</span>
        <div
          style={{
            marginTop: "var(--sp-2)",
            display: "flex",
            alignItems: "center",
            gap: "var(--sp-3)",
            flexWrap: "wrap"
          }}
        >
          <span style={{ flex: 1, minWidth: 0, color: "var(--mut)", lineHeight: 1.5 }}>{label}</span>
          <button
            type="button"
            disabled
            title={
              hint ??
              "Movimentação de dinheiro chega numa próxima etapa (admin) e seria recusada enquanto homolog."
            }
            style={{
              fontFamily: "var(--font-body)",
              fontSize: "var(--fs-xs)",
              fontWeight: 600,
              padding: "var(--sp-1) var(--sp-3)",
              borderRadius: "var(--r-sm)",
              border: "1px solid var(--line)",
              background: "var(--cream)",
              color: "var(--mut)",
              cursor: "not-allowed",
              whiteSpace: "nowrap"
            }}
          >
            {cta}
          </button>
        </div>
        <p style={REFUSE_NOTE}>
          <span aria-hidden="true" style={{ color: homolog ? "var(--warn)" : "var(--crit)", fontWeight: 700 }}>
            ●
          </span>
          {homolog ? (
            <span>
              Ambiente <strong>homolog</strong>: mesmo com o caminho de escrita ligado, mover dinheiro seria{" "}
              <strong>recusado</strong> aqui — só vale em <strong>prod</strong>. E quem/quanto pagar é decidido pelo{" "}
              <strong>workflow do cash-loop</strong>, nunca por esta surface.
            </span>
          ) : (
            <span>
              Quem/quanto pagar é decidido pelo <strong>workflow do cash-loop</strong>, nunca por esta surface —
              aqui só se confirma e observa.
            </span>
          )}
        </p>
      </section>
    </CapabilityGate>
  );
}
