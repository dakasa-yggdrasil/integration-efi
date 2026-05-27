package capabilities

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dakasa-yggdrasil/integration-efi/providers/efi/efiapi"
)

// EnsureDueCharge PUTs /v2/cobv/{txid} — the "due charge" (cobrança
// com vencimento) variant. txid is mandatory because cobv is always
// idempotent. v2.0.0 rename of create_due_charge.
//
// Required input: txid, valor.original, chave,
//                 calendario.dataDeVencimento, devedor.cpf, devedor.nome.
// Optional:       calendario.validadeAposVencimento, multa, juros,
//                 abatimento, desconto, infoAdicionais, solicitacaoPagador.
func EnsureDueCharge(ctx context.Context, c *efiapi.EfiClient, in map[string]any) (map[string]any, error) {
	txid, _ := in["txid"].(string)
	if txid == "" {
		return nil, fmt.Errorf("ensure_due_charge: txid is required (cobv is always idempotent)")
	}
	valor, _ := in["valor"].(map[string]any)
	if valor == nil || fmt.Sprint(valor["original"]) == "" {
		return nil, fmt.Errorf("ensure_due_charge: valor.original is required")
	}
	chave, _ := in["chave"].(string)
	if chave == "" {
		return nil, fmt.Errorf("ensure_due_charge: chave is required")
	}
	calendario, _ := in["calendario"].(map[string]any)
	if calendario == nil || fmt.Sprint(calendario["dataDeVencimento"]) == "" {
		return nil, fmt.Errorf("ensure_due_charge: calendario.dataDeVencimento is required")
	}
	devedor, _ := in["devedor"].(map[string]any)
	if devedor == nil {
		return nil, fmt.Errorf("ensure_due_charge: devedor is required (cpf + nome)")
	}
	if fmt.Sprint(devedor["cpf"]) == "" {
		return nil, fmt.Errorf("ensure_due_charge: devedor.cpf is required")
	}
	if fmt.Sprint(devedor["nome"]) == "" {
		return nil, fmt.Errorf("ensure_due_charge: devedor.nome is required")
	}

	body := map[string]any{
		"valor":      valor,
		"chave":      chave,
		"calendario": calendario,
		"devedor":    devedor,
	}
	for _, opt := range []string{"multa", "juros", "abatimento", "desconto", "infoAdicionais", "solicitacaoPagador"} {
		if v, ok := in[opt]; ok {
			body[opt] = v
		}
	}

	var resp map[string]any
	if err := efiapi.DoRaw(ctx, c, http.MethodPut, "/v2/cobv/"+txid, body, &resp); err != nil {
		return nil, fmt.Errorf("ensure_due_charge: %w", err)
	}
	return resp, nil
}
