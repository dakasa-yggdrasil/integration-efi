import { useMemo, useState, type CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useCharges,
  useEfiBase,
  useEnvironment,
  formatMoney,
  sumReais,
  pixHref,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import type { ChargeItem } from "../data";
import { ChargeTable } from "./charges-parts";
import { FilterPills } from "./shared/FilterPills";
import type { FilterOption } from "./shared/FilterPills";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .ef-rc-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-rc-kpis { grid-template-columns: 1fr; } }
`;

const FILTER_LABEL: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.1em",
  textTransform: "uppercase",
  color: "var(--mut)",
  marginBottom: "var(--sp-2)",
  display: "block"
};

const NOTE: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

export function Charges() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const efiBase = useEfiBase();
  const env = useEnvironment();

  // The status filter is applied LOCALLY over the loaded window (it never
  // refetches) — the same calm UX as the Stripe roster.
  const charges = useCharges(instanceId, { windowDays: 30 });

  const [statusFilter, setStatusFilter] = useState<string | null>(null);

  const total = charges.items.length;
  const concluidasBrl = useMemo(
    () =>
      sumReais(
        charges.items
          .filter((c) => c.status.trim().toUpperCase() === "CONCLUIDA")
          .map((c) => c.valor)
      ),
    [charges.items]
  );

  const statuses = useMemo(
    () =>
      Array.from(new Set(charges.items.map((c) => c.status.trim()).filter((s) => s !== ""))).sort((a, b) =>
        a.localeCompare(b)
      ),
    [charges.items]
  );

  const filtered = useMemo<ChargeItem[]>(() => {
    if (statusFilter === null) return charges.items;
    return charges.items.filter((c) => c.status.trim() === statusFilter);
  }, [charges.items, statusFilter]);

  const statusOptions: FilterOption[] = [
    { value: null, label: "Todos os status", count: total },
    ...statuses.map((s) => ({
      value: s,
      label: s,
      count: charges.items.filter((c) => c.status.trim() === s).length
    }))
  ];

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="ef-rc-kpis">
        <KpiTile eyebrow={`Charges (${charges.windowDays}d)`} value={total} chart={kpiSubtext("refs, sem dados de pagador", false)} />
        <KpiTile eyebrow="Concluídas (R$)" value={formatMoney(concluidasBrl)} chart={kpiSubtext("na janela exibida", false)} />
        <KpiTile eyebrow="Drift de reconciliação" value="—" chart={kpiSubtext("needs-work", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || charges.isLoading) {
      return <LoadingState label="Lendo as charges recentes…" />;
    }
    if (charges.isError) {
      return (
        <EmptyState
          title="Não consegui ler as charges"
          description={charges.error instanceof Error ? charges.error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhuma charge na janela"
          description="A conta não expõe charges Pix visíveis para este token na janela atual."
        />
      );
    }

    const payHref = pixHref(efiBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* the rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Visão de <strong>ops de Pix</strong>: só referências opacas (<code>txid</code>, <code>valor</code>,{" "}
          <code>status</code>, <code>tipo</code>, <code>created</code>) — <strong>sem coluna de pagador</strong>, sem
          nome, CPF, e-mail ou chave Pix. O adapter dropa o <code>devedor</code> e esta tabela nunca o reintroduz.
        </p>

        {/* the honest reconciliation-drift gap */}
        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            O <strong>drift de reconciliação</strong> — casar as charges da EFI com{" "}
            <code>identities.webhook_event_efi</code> — precisa de um <strong>join cross-system via core</strong> (o
            lado EFI é legível aqui, mas o event-store vive no DB da enterprise e só é alcançável pelo core), então é{" "}
            <em>needs-work</em>. Esta lista é o contexto de charges recentes, não o ledger fechado — não inventamos o
            drift.
          </span>
        </div>

        {/* status filter */}
        <section>
          <span style={FILTER_LABEL}>Status</span>
          <FilterPills
            ariaLabel="Filtrar por status"
            options={statusOptions}
            selected={statusFilter}
            onSelect={setStatusFilter}
          />
        </section>

        {/* charges table */}
        {filtered.length === 0 ? (
          <EmptyState title="Nenhuma charge com esse status" description="Escolha outro status para ver mais." />
        ) : (
          <ChargeTable charges={filtered} />
        )}

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
              title="Link para a EFI nativa indisponível: o host do console ainda não é exposto por um surface read."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Pix na EFI <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || charges.isLoading || charges.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Charges & Reconciliação"
        subtitle="Charges Pix recentes por referência opaca — sem dados de pagador. O drift de reconciliação (EFI vs identities.webhook_event_efi) é needs-work."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
