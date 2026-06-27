import { mockEnabled, MOCK_ENVIRONMENT } from "./mock";
import type { EfiEnvironment } from "./types";

/**
 * The EFI instance's deployment environment — `homolog` (EFI homologation,
 * `pix-h` base URL, instance `sandbox: true`) or `prod` (`pix`,
 * `sandbox: false`).
 *
 * The environment badge is MANDATORY and prominent (rule #0): money-movement
 * (refund / payout) is REFUSED while homolog, so the operator must always see
 * which rail they are looking at.
 *
 * HONEST GAP: the environment is part of the instance CONFIG (`sandbox`), which
 * the surface-query responses do NOT carry today (the adapter shapes only row
 * fields, not the instance config). So in the live path we cannot read it from
 * a surface read yet — we default to `homolog`, the SAFE assumption (it refuses
 * money-movement), rather than claiming `prod`. Under `?mock` the environment is
 * `homolog` explicitly. When a future surface read exposes the instance config
 * (`sandbox`), wire it in here — every page already reads the env through this
 * one hook and renders the badge from it.
 */
export function useEnvironment(): EfiEnvironment {
  if (mockEnabled()) return MOCK_ENVIRONMENT;
  // Live: not yet readable from a surface query — default to the safe
  // assumption (homolog refuses money-movement) rather than asserting prod.
  return "homolog";
}
