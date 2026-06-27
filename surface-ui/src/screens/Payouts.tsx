import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  useCollaboratorScope,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useEnvironment,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID
} from "../data";
import { GatedAction } from "./shared/GatedAction";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .ef-po-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-po-kpis { grid-template-columns: 1fr; } }
`;

export function Payouts() {
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const env = useEnvironment();

  // No payout data is read at all (needs-work), so the instance handle is only
  // used to keep the chrome consistent; we never query payout state.
  void instanceId;

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="ef-po-kpis">
        <KpiTile eyebrow="Payouts recentes" value="—" chart={kpiSubtext("sem op de leitura", false)} />
        <KpiTile eyebrow="Próximo prólabore" value="—" chart={kpiSubtext("decide o cash-loop", false)} />
        <KpiTile eyebrow="Estado" value="—" chart={kpiSubtext("confirm + observe", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading) {
      return <LoadingState label="Preparando a página de payouts…" />;
    }
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* rule-#0 reminder — surface confirms, never decides */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          O cash-loop decide quem/quanto; a EFI executa o Pix. Aqui é confirm + observe.
        </p>

        {/* what's needs-work */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Histórico de payouts: sem op de leitura.
        </p>

        {/* money-movement: create_payout — admin-tier, gated + disabled, refused in homolog */}
        <GatedAction
          need="efi.payouts.create"
          perms={scope.perms}
          env={env}
          eyebrow="Remediação"
          label="Disparar payout (Pix de saída) — admin, em breve."
          hint="create_payout é admin e recusado em homolog."
        />
      </div>
    );
  }

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Payouts & Prólabore"
        subtitle="Confirm + observe — o cash-loop decide."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
