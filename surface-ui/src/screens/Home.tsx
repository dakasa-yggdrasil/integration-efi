import type { CSSProperties } from "react";
import {
  useCollaboratorScope,
  useDefaultInstance,
  LoadingState,
  EmptyState
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useEfiPulse,
  useWebhookSubscriptions,
  useEnvironment,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID,
  MOCK_INSTANCE_LABEL
} from "../data";
import { KpiStrip, AttentionBand, NavGroups } from "./home-parts";
import type { NavGroupSpec } from "./home-parts";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";

/* ---------------------------------------------------------------- layout */

const PAGE: CSSProperties = {
  containerType: "inline-size",
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)",
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-6)",
  fontFamily: "var(--font-body)",
  color: "var(--body)"
};

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-xl)",
  fontWeight: 500,
  color: "var(--ink)"
};

const EYEBROW: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.2em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

// Narrow-host header reflow (by host width, not viewport).
const HEADER_REFLOW = `
  @container (max-width: 560px) {
    .ef-home-header { flex-direction: column; align-items: flex-start; }
  }
`;

/* ---------------------------------------------------------------- helpers */

// Technical, one-line read of the account — bare finance-OPS facts joined by
// "·". No editorializing. "webhook mTLS ativo" only when a subscription exists
// with mTLS enforced; otherwise the honest pending phrasing.
function headline(parts: {
  label: string;
  env: string;
  hasWebhook: boolean;
  mtlsOk: boolean;
  charges: number;
  windowDays: number;
}): string {
  const webhookFact = !parts.hasWebhook
    ? "sem webhook"
    : parts.mtlsOk
      ? "webhook mTLS ativo"
      : "webhook sem mTLS";
  return [
    `EFI Pix · ${parts.env}`,
    webhookFact,
    `${parts.charges} ${parts.charges === 1 ? "charge" : "charges"} na janela ${parts.windowDays}d`
  ].join(" · ");
}

/* ---------------------------------------------------------------- screen */

export function Home() {
  // The collaborator + instance context is resolved over the network (/me,
  // provisioning-status, manifests). Under `?mock` we stub it entirely so the
  // surface renders fully offline for live-review — admin tier, every EFI perm,
  // a fake instance handle. The hooks stay called unconditionally to keep hook
  // order stable; only their values are overridden. Dead code in prod (the gate
  // is DEV + `?mock`), and the real (non-mock) path below is untouched.
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");

  const scope = mock ? mockCollaboratorScope() : liveScope;
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const instanceLabel = mock ? MOCK_INSTANCE_LABEL : "EFI Pix";
  const env = useEnvironment();

  const pulse = useEfiPulse(instanceId);
  const webhooks = useWebhookSubscriptions(instanceId);

  if (scope.isLoading || instanceLoading) {
    return (
      <div className="atelier" style={{ padding: "var(--sp-7)" }}>
        <LoadingState label="Carregando…" />
      </div>
    );
  }

  if (scope.isError) {
    return (
      <div className="atelier" style={{ padding: "var(--sp-7)" }}>
        <EmptyState
          title="Não consegui carregar seu contexto"
          description="Falha ao resolver colaborador e instância. Recarregue em instantes."
        />
      </div>
    );
  }

  const mtlsOk = pulse.hasWebhook && pulse.webhooksMtlsOff === 0;

  // The identity line: what the surface IS — a finance-OPS view, with the hard
  // rule that money-movement and payer data are never here.
  const identityLine = [
    "Webhook & mTLS · charges & conciliação · payouts · refunds",
    "ops de Pix — sem dados de pagador, sem mover dinheiro",
    "quem/quanto paga é o cash-loop, não esta surface"
  ].join(" · ");

  // Grouped nav — a calm, scannable index over every detail page, organized by
  // function. Ingestão = what comes IN (the mTLS-hardened webhook + the recent
  // charges roster / reconciliação). Dinheiro = money OUT (payouts/prólabore +
  // refunds), both honestly needs-work / admin-em-breve — the surface NEVER
  // decides who/how-much to pay; that is the cash-loop. Each card carries a hard
  // number (or honest "—"); bad signals (webhook sem mTLS) surface a pill so the
  // operator's eye lands on them first.
  const navGroups: NavGroupSpec[] = [
    {
      key: "ingestao",
      title: "Ingestão",
      cards: [
        {
          key: "webhook",
          label: "Webhook & mTLS",
          value: pulse.hasWebhook ? (mtlsOk ? "ativo" : "sem mTLS") : "—",
          unit: pulse.hasWebhook ? "mTLS" : "ausente",
          to: "/webhook",
          tagLabel: pulse.webhooksMtlsOff > 0 ? `${pulse.webhooksMtlsOff} sem mTLS` : undefined,
          tagTone: "crit"
        },
        {
          key: "charges",
          label: "Charges & Reconciliação",
          value: pulse.charges,
          unit: pulse.charges === 1 ? "charge" : "charges",
          to: "/charges",
          tagLabel: "drift needs-work",
          tagTone: "neutral"
        }
      ]
    },
    {
      key: "dinheiro",
      title: "Dinheiro",
      cards: [
        {
          key: "payouts",
          label: "Payouts & Prólabore",
          value: "—",
          unit: "via cash-loop",
          to: "/payouts",
          tagLabel: "needs-work",
          tagTone: "neutral"
        },
        {
          key: "refunds",
          label: "Refunds",
          value: "—",
          unit: "admin · em breve",
          to: "/refunds",
          tagLabel: "em breve",
          tagTone: "neutral"
        }
      ]
    }
  ];

  return (
    <div className="atelier" style={PAGE}>
      <style>{HEADER_REFLOW}</style>

      {/* ---------- header (account identity + MANDATORY env badge) ---------- */}
      <header
        className="ef-home-header"
        style={{ display: "flex", justifyContent: "space-between", gap: "var(--sp-5)", alignItems: "flex-start" }}
      >
        <div style={{ minWidth: 0 }}>
          <span style={EYEBROW}>Conta</span>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: "var(--sp-3)",
              flexWrap: "wrap",
              marginTop: "var(--sp-1)"
            }}
          >
            <span
              style={{
                fontFamily: "var(--font-heading)",
                fontSize: "var(--fs-xl)",
                fontWeight: 500,
                color: "var(--ink)",
                lineHeight: 1.15
              }}
            >
              {instanceLabel}
            </span>
            {/* MANDATORY prominent environment badge (rule #0). */}
            <EnvironmentBadge env={env} size="lg" />
          </div>
          <div style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", fontFamily: "var(--font-mono, var(--font-body))" }}>
            {identityLine}
          </div>
        </div>
        <div style={{ textAlign: "right", fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.7 }}>
          <span style={EYEBROW}>Webhook</span>
          <div
            style={{
              fontFamily: "var(--font-heading)",
              fontSize: "var(--fs-md)",
              color: mtlsOk ? "var(--ok)" : "var(--body)",
              marginTop: "var(--sp-1)"
            }}
          >
            {pulse.hasWebhook ? (mtlsOk ? "mTLS ativo" : "sem mTLS") : "ausente"}
          </div>
        </div>
      </header>

      {/* ---------- technical headline ---------- */}
      <section>
        <h1
          style={{
            margin: 0,
            fontFamily: "var(--font-heading)",
            fontSize: "var(--fs-xl)",
            fontWeight: 400,
            lineHeight: 1.3,
            letterSpacing: "-0.01em",
            color: "var(--ink)"
          }}
        >
          {headline({
            label: instanceLabel,
            env,
            hasWebhook: pulse.hasWebhook,
            mtlsOk,
            charges: pulse.charges,
            windowDays: pulse.chargeWindowDays
          })}
        </h1>
      </section>

      {/* ---------- KPI strip ---------- */}
      <section>
        <KpiStrip pulse={pulse} />
      </section>

      {/* ---------- precisa de você (euphemized — honest readable signal) ---------- */}
      <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
        <h2 style={SECTION_TITLE}>Precisa de você</h2>
        <AttentionBand webhooks={webhooks} />
      </section>

      {/* ---------- grouped nav (Ingestão · Dinheiro) ---------- */}
      <section>
        <NavGroups groups={navGroups} />
      </section>
    </div>
  );
}
