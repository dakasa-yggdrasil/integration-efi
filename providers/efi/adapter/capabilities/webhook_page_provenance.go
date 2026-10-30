package capabilities

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Only webhook observations use this decoder. Duplicate members must not turn
// into unambiguous resource identities or page counters through last-wins JSON.
// Errors never include provider values or member names.
type webhookObservationJSON map[string]any

func (v *webhookObservationJSON) UnmarshalJSON(raw []byte) error {
	fail := errors.New("invalid or ambiguous webhook observation JSON")
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var scan func(int) error
	scan = func(depth int) error {
		if depth > 32 {
			return fail
		}
		token, err := decoder.Token()
		if err != nil {
			return fail
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					token, err := decoder.Token()
					name, ok := token.(string)
					folded := strings.ToLower(name)
					if err != nil || !ok || seen[folded] {
						return fail
					}
					seen[folded] = true
					if scan(depth+1) != nil {
						return fail
					}
				}
			case '[':
				for decoder.More() {
					if scan(depth+1) != nil {
						return fail
					}
				}
			default:
				return fail
			}
			if _, err := decoder.Token(); err != nil {
				return fail
			}
		}
		return nil
	}
	if scan(0) != nil {
		return fail
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fail
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil || decoded == nil {
		return fail
	}
	*v = decoded
	return nil
}

type webhookPageEvidence struct {
	SchemaVersion int  `json:"schemaVersion"`
	Validated     bool `json:"validated"`
	Complete      bool `json:"complete"`
}

// This proves the requested creation-time page, not that the caller's interval
// covers an account's history. Unsupported provider fields remain visible with
// false evidence. No extra request or mutation is made to complete a page.
func webhookPageProvenance(input map[string]any, response map[string]any) webhookPageEvidence {
	evidence := webhookPageEvidence{SchemaVersion: 1}
	start, end, err := webhookTimeRange(input)
	if err != nil {
		return evidence
	}
	requestedStart, _ := time.Parse(time.RFC3339, start)
	requestedEnd, _ := time.Parse(time.RFC3339, end)
	page, _, err := paginationInput(input, "page", 0, 0)
	if err != nil {
		return evidence
	}
	if cursor, exists := input["cursor"]; exists {
		text, ok := cursor.(string)
		if !ok {
			return evidence
		}
		if strings.TrimSpace(text) != "" {
			page, err = parsePaginationInteger(strings.TrimSpace(text), 0, 0)
			if err != nil {
				return evidence
			}
		}
	}
	size, explicitSize, err := paginationInput(input, "page_size", 1, 1000)
	if err != nil || !explicitSize {
		return evidence
	}
	parameters, ok := response["parametros"].(map[string]any)
	if !ok || !webhookTimeEquals(parameters["inicio"], requestedStart) || !webhookTimeEquals(parameters["fim"], requestedEnd) {
		return evidence
	}
	pagination, ok := parameters["paginacao"].(map[string]any)
	if !ok {
		return evidence
	}
	current, currentOK := webhookPageInteger(pagination["paginaAtual"])
	pageSize, sizeOK := webhookPageInteger(pagination["itensPorPagina"])
	pages, pagesOK := webhookPageInteger(pagination["quantidadeDePaginas"])
	total, totalOK := webhookPageInteger(pagination["quantidadeTotalDeItens"])
	rows, rowsOK := response["webhooks"].([]any)
	if !currentOK || !sizeOK || !pagesOK || !totalOK || !rowsOK || current != int64(page) || pageSize != int64(size) || len(rows) > size {
		return evidence
	}
	expectedPages := (total + pageSize - 1) / pageSize
	if total == 0 {
		if current != 0 || (pages != 0 && pages != 1) || len(rows) != 0 {
			return evidence
		}
	} else if pages != expectedPages || current >= pages {
		return evidence
	}
	expectedRows := total - current*pageSize
	if expectedRows > pageSize {
		expectedRows = pageSize
	}
	if expectedRows != int64(len(rows)) {
		return evidence
	}
	seen := map[string]bool{}
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			return evidence
		}
		key, keyOK := record["chave"].(string)
		address, addressOK := record["webhookUrl"].(string)
		created, createdOK := record["criacao"].(string)
		if !keyOK || key == "" || len([]rune(key)) > 77 || key != strings.TrimSpace(key) || strings.IndexFunc(key, unicode.IsControl) >= 0 || seen[key] || !addressOK || !webhookObservedURLValid(address) || !createdOK {
			return evidence
		}
		// Keep raw identity unchanged, but never let an alternate UUID spelling
		// turn an exact-key comparison into a false absence observation.
		if parsed, err := uuid.Parse(key); err == nil && parsed.String() != key {
			return evidence
		}
		at, err := time.Parse(time.RFC3339Nano, created)
		if err != nil || at.Before(requestedStart) || at.After(requestedEnd) {
			return evidence
		}
		seen[key] = true
	}
	evidence.Validated = true
	evidence.Complete = current == 0 && pages == 1 && total == int64(len(rows))
	return evidence
}

func webhookPageInteger(value any) (int64, bool) {
	// Provider JSON counters must be bounded whole JSON numbers, never
	// coerced strings, missing fields or fractions.
	n, ok := value.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || n < 0 || n > math.MaxInt32 {
		return 0, false
	}
	return int64(n), true
}

func webhookTimeEquals(value any, requested time.Time) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, text)
	return err == nil && at.Equal(requested)
}

func webhookObservedURLValid(raw string) bool {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.Contains(raw, "#") {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	_, err = url.ParseQuery(parsed.RawQuery)
	return err == nil
}
