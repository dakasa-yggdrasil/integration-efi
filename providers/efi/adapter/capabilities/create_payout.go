package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// CreatePayout PUTs /v3/gn/pix/{idEnvio} per EFI's "envio" (payout)
// API.
//
// SAFETY CRITICAL: this moves real money. Tier classification is
// IntermediateIrreversible — DO NOT dead-letter on transient errors.
// Caller workflows MUST enforce token-bucket rate limiting per EFI's
// 500 tx/sec contract.
//
// Required input: idEnvio (caller-supplied idempotency key), valor,
//                 pagador.chave, AND either favorecido.chave OR
//                 (favorecido.cpf + favorecido.contaBanco).
//
// Idempotency: enforced server-side by EFI on `idEnvio`. We mark this
// capability `Idempotent: false` in the contract for safety
// classification — the caller workflow MUST treat retries as opaque
// queries (not as new send attempts).
func CreatePayout(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	idEnvio, _ := in["idEnvio"].(string)
	if idEnvio == "" {
		return nil, fmt.Errorf("create_payout: idEnvio is required")
	}
	valor, _ := in["valor"].(string)
	if valor == "" {
		return nil, fmt.Errorf("create_payout: valor is required")
	}
	pagador, _ := in["pagador"].(map[string]any)
	if pagador == nil || fmt.Sprint(pagador["chave"]) == "" {
		return nil, fmt.Errorf("create_payout: pagador.chave is required")
	}
	favorecido, _ := in["favorecido"].(map[string]any)
	if favorecido == nil {
		return nil, fmt.Errorf("create_payout: favorecido is required (chave OR cpf+contaBanco)")
	}
	hasChave := fmt.Sprint(favorecido["chave"]) != ""
	hasCpfAndConta := fmt.Sprint(favorecido["cpf"]) != "" && favorecido["contaBanco"] != nil
	if !hasChave && !hasCpfAndConta {
		return nil, fmt.Errorf("create_payout: favorecido must have either chave OR (cpf + contaBanco)")
	}

	body := map[string]any{
		"valor":      valor,
		"pagador":    pagador,
		"favorecido": favorecido,
	}

	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodPut, "/v3/gn/pix/"+idEnvio, body, &resp); err != nil {
		return nil, fmt.Errorf("create_payout: %w", err)
	}
	return resp, nil
}
