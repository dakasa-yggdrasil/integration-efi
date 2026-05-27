// Package capabilities holds one Go file per EFI capability. Each
// function takes (ctx, *EfiClient, in map[string]any) and returns
// (output map[string]any, error). The adapter.Execute switch fans
// dispatch into these per-capability functions.
package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// CreateCharge POSTs /v2/cob (auto-generated txid) or PUTs
// /v2/cob/{txid} (caller-provided makes the call idempotent), per BCB
// PIX spec.
//
// Required input: valor.original (string), chave (string).
// Optional:       txid (idempotency), expiracao (int), devedor,
//                 infoAdicionais, solicitacaoPagador.
func CreateCharge(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	valor, _ := in["valor"].(map[string]any)
	if valor == nil || fmt.Sprint(valor["original"]) == "" {
		return nil, fmt.Errorf("create_charge: valor.original is required")
	}
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("create_charge: chave is required")
	}

	body := map[string]any{
		"valor": valor,
		"chave": chave,
	}
	if exp, ok := in["expiracao"]; ok {
		body["calendario"] = map[string]any{"expiracao": exp}
	}
	if devedor, ok := in["devedor"]; ok {
		body["devedor"] = devedor
	}
	if info, ok := in["infoAdicionais"]; ok {
		body["infoAdicionais"] = info
	}
	if sp, ok := in["solicitacaoPagador"]; ok {
		body["solicitacaoPagador"] = sp
	}

	method := http.MethodPost
	path := "/v2/cob"
	if txid, _ := in["txid"].(string); txid != "" {
		path = "/v2/cob/" + txid
		method = http.MethodPut
	}

	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, method, path, body, &resp); err != nil {
		return nil, fmt.Errorf("create_charge: %w", err)
	}
	return resp, nil
}
