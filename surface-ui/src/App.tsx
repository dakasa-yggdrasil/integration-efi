import { Routes, Route, Navigate } from "react-router-dom";
import { Home } from "./screens/Home";
import { Webhook } from "./screens/Webhook";
import { Charges } from "./screens/Charges";
import { ChargeDetail } from "./screens/ChargeDetail";
import { Payouts } from "./screens/Payouts";
import { Refunds } from "./screens/Refunds";

/**
 * Collaborator-root router for the EFI (Pix) operator surface — surface #8 of
 * the 9-surface family, the SECOND payment-rail surface (mirrors the Stripe
 * template 1:1). The surface opens on a technical account-pulse Home, with four
 * pillar detail screens — Webhook & mTLS, Charges & Reconciliação, Payouts &
 * Prólabore, and Refunds — plus a charge drill-down (`/charge/:txid`) opened
 * from a txid in the Charges roster. The Home nav is grouped by function
 * (Ingestão / Dinheiro).
 *
 * CRITICAL RULE #0 (HARDEST in the family — EFI is the contract's FORBIDDEN
 * "pay your bill" example): this is a finance-OPS view for the platform team,
 * NEVER a customer Pix/billing UI. Charge rows carry ONLY opaque refs (txid,
 * valor, status, tipo, created) — never payer nome/CPF/email/pixKey. A
 * prominent ENVIRONMENT badge (homolog / prod) is mandatory. Money-movement
 * (refund / payout) is admin-tier and OUT of v1 — a gated, disabled "Em breve"
 * affordance that states it would be REFUSED while homolog, never a
 * transactional button. The surface NEVER decides who/how-much to pay — that
 * decision lives in the cash-loop workflow; the surface is confirm + observe
 * only. The warm Atelier theme is applied per-screen;
 * `BrowserRouter basename="/s/efi"` lives in main.tsx.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/webhook" element={<Webhook />} />
      <Route path="/charges" element={<Charges />} />
      <Route path="/charge/:txid" element={<ChargeDetail />} />
      <Route path="/payouts" element={<Payouts />} />
      <Route path="/refunds" element={<Refunds />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
