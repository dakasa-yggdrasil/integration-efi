import type { CSSProperties, ReactNode } from "react";
import { useParams, useLocation, Link } from "react-router-dom";
import {
  TierTwoShell,
  KpiTile,
  Chip,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useChargeDetail,
  useEfiBase,
  useEnvironment,
  formatMoney,
  sumReais,
  pixHref,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import { DevolucoesTable } from "./charges-parts";
import { StatusDot, chargeStatusTone } from "./shared/StatusDot";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";
import { formatCreated, relativeCreated } from "../data";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .ef-cd-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-cd-kpis { grid-template-columns: 1fr; } }
`;

const BACK_LINK: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: "var(--sp-1)",
  fontSize: "var(--fs-sm)",
  fontWeight: 600,
  color: "var(--mut)",
  textDecoration: "none",
  transition: "color 100ms ease"
};

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-lg)",
  fontWeight: 500,
  color: "var(--ink)"
};

const MONO: CSSProperties = { fontFamily: "var(--font-mono, var(--font-body))" };

/** The cob / cobv kind chip (immediate vs due charge). */
function TipoChip({ tipo }: { tipo: string }) {
  const t = tipo.trim().toLowerCase();
  if (t === "cobv") return <Chip label="cobv" tone="team" preserveCase />;
  if (t === "cob") return <Chip label="cob" tone="neutral" preserveCase />;
  return <Chip label={tipo || "—"} tone="neutral" preserveCase />;
}

/** A consistent section frame: heading (+count) then its body. */
function Section({ title, count, children }: { title: string; count?: number; children: ReactNode }) {
  return (
    <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: "var(--sp-2)" }}>
        <h3 style={SECTION_TITLE}>{title}</h3>
        {count !== undefined ? (
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", fontWeight: 600 }}>{count}</span>
        ) : null}
      </div>
      {children}
    </section>
  );
}

/**
 * Render the charge's validity window. An immediate cob carries the expiry as a
 * number of SECONDS ("3600" → "1 h"); a cobv carries a due DATE ("2026-07-10").
 * Both are opaque operational refs — never reinterpreted as payer data.
 */
function expiracaoLabel(tipo: string, expiracao: string): string {
  const raw = expiracao.trim();
  if (raw === "") return "—";
  if (tipo.trim().toLowerCase() === "cobv") return `vence ${raw}`;
  // cob: expiracao is a validity window in seconds.
  const secs = Number(raw);
  if (!Number.isFinite(secs) || secs <= 0) return raw;
  if (secs % 3600 === 0) return `expira em ${secs / 3600} h`;
  if (secs % 60 === 0) return `expira em ${secs / 60} min`;
  return `expira em ${secs} s`;
}

/**
 * The charge drill-down (`/charge/:txid`) — opened from a txid in the Charges
 * roster. Reads `charge-detail` (param `txid`) and renders a header (txid mono,
 * valor, status dot, tipo chip, created + expiração), a devoluções section
 * (money-already-moved history), and the MANDATORY environment badge. RULE #0:
 * opaque refs only; NO payer identity anywhere, NO payer column. Back-link
 * carries the query string (e.g. `?mock`) so DEV review survives the round trip.
 */
export function ChargeDetail() {
  const params = useParams<{ txid: string }>();
  const txid = params.txid ?? "";
  const { search } = useLocation();

  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const efiBase = useEfiBase();
  const env = useEnvironment();

  const { detail, isLoading, isError, error } = useChargeDetail(instanceId, txid);

  const backLink = (
    <Link to={`/charges${search}`} style={BACK_LINK} className="ef-cd-back">
      <span aria-hidden="true">←</span>
      <span>Charges</span>
    </Link>
  );

  const devolvidoBrl =
    detail !== null ? sumReais(detail.devolucoes.map((d) => d.valor)) : 0;

  const subtitle =
    detail && !isLoading && !isError ? (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
        {backLink}
        <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-3)", flexWrap: "wrap" }}>
          <StatusDot tone={chargeStatusTone(detail.status)} label={detail.status || "—"} />
          <TipoChip tipo={detail.tipo} />
          {detail.devolucoes.length > 0 ? <Chip label="com devolução" tone="warn" preserveCase /> : null}
          <span style={{ color: "var(--mut)", fontSize: "var(--fs-sm)" }} title={relativeCreated(detail.created)}>
            {formatCreated(detail.created)}
          </span>
          <span style={{ color: "var(--mut)", fontSize: "var(--fs-sm)" }}>{expiracaoLabel(detail.tipo, detail.expiracao)}</span>
          <EnvironmentBadge env={env} size="sm" />
        </div>
      </div>
    ) : (
      backLink
    );

  const kpis =
    detail && !isLoading && !isError ? (
      <div style={{ containerType: "inline-size", width: "100%" }}>
        <style>{KPI_GRID}</style>
        <div className="ef-cd-kpis">
          <KpiTile eyebrow="Valor" value={formatMoney(detail.valor)} chart={kpiSubtext("ref opaca, sem pagador", false)} />
          <KpiTile
            eyebrow="Devolvido (R$)"
            value={formatMoney(devolvidoBrl)}
            chart={kpiSubtext(devolvidoBrl > 0 ? "movido (histórico)" : "nenhuma", devolvidoBrl > 0)}
          />
          <KpiTile
            eyebrow="Devoluções"
            value={detail.devolucoes.length}
            chart={kpiSubtext(detail.devolucoes.length === 1 ? "registro" : "registros", false)}
          />
        </div>
      </div>
    ) : undefined;

  function body() {
    if (instanceLoading || isLoading) {
      return <LoadingState label="Lendo o detalhe da charge…" />;
    }
    if (isError) {
      return (
        <EmptyState
          title="Não consegui ler esta charge"
          description={error instanceof Error ? error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (!detail) {
      return (
        <EmptyState
          title="Charge não encontrada"
          description={`Nenhuma charge "${txid}" visível para este token.`}
        />
      );
    }

    const payHref = pixHref(efiBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        <style>{".ef-cd-back:hover { color: var(--honey); }"}</style>

        {/* rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Ref opaca (<code style={MONO}>{detail.txid}</code>) — sem dados de pagador. Devoluções: histórico, somente
          leitura.
        </p>

        {/* identity refs */}
        <Section title="Referências">
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
            <RefRow label="Txid" value={detail.txid} mono />
            <RefRow label="Tipo" value={detail.tipo === "cobv" ? "cobv (cobrança com vencimento)" : "cob (cobrança imediata)"} />
            <RefRow label="Expiração" value={expiracaoLabel(detail.tipo, detail.expiracao)} />
          </div>
        </Section>

        {/* devoluções */}
        <Section title="Devoluções" count={detail.devolucoes.length}>
          {detail.devolucoes.length === 0 ? (
            <EmptyState title="Sem devoluções" description="Nenhuma devolução registrada para esta charge." />
          ) : (
            <DevolucoesTable devolucoes={detail.devolucoes} />
          )}
        </Section>

        {/* deep-link to native Pix area */}
        <div>
          {payHref ? (
            <a
              href={payHref}
              target="_blank"
              rel="noreferrer"
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: "var(--sp-2)",
                fontSize: "var(--fs-sm)",
                fontWeight: 700,
                color: "var(--honey)",
                textDecoration: "none"
              }}
            >
              Pix na EFI <span aria-hidden="true">↗</span>
            </a>
          ) : (
            <span
              title="Link para a EFI nativa indisponível."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Pix na EFI <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || isLoading || isError || !detail;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Charge"
        title={txid || "Charge"}
        subtitle={subtitle}
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}

// A label + opaque-ref row (no "↗" — EFI has no per-charge native deep-link).
function RefRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--sp-3)",
        padding: "var(--sp-2) var(--sp-3)",
        background: "var(--sand)",
        border: "1px solid var(--line)",
        borderRadius: "var(--r-sm)",
        minWidth: 0
      }}
    >
      <span
        style={{
          fontSize: "var(--fs-xs)",
          fontWeight: 700,
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: "var(--mut)",
          width: "8.5em",
          flex: "0 0 auto"
        }}
      >
        {label}
      </span>
      <span
        style={{
          ...(mono ? MONO : {}),
          fontSize: "var(--fs-sm)",
          color: "var(--ink)",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
          flex: 1
        }}
        title={value}
      >
        {value || "—"}
      </span>
    </div>
  );
}
