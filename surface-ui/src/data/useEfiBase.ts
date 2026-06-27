import { mockEnabled, MOCK_EFI_BASE } from "./mock";

/**
 * The native EFI console host root used to build "↗" deep-links.
 *
 * HONEST GAP: the instance's EFI console host is NOT returned by any surface
 * query today (the adapter shapes only row fields), so in the live path this
 * resolves to "" and every deep-link degrades to a disabled, explained "↗".
 * Under `?mock` we return the real console host so the
 * affordance can be reviewed as a working link. When a future surface read
 * exposes the host, wire it in here — every page already routes its links
 * through this one hook.
 */
export function useEfiBase(): string {
  if (mockEnabled()) return MOCK_EFI_BASE;
  return "";
}
