// Package message wires the SDK's RPC handler shape to the
// adapter-defined describe + execute capabilities. The helpers here
// preserve the `{ok, data?, error?}` wire envelope used by every
// Yggdrasil adapter.
package message

import (
	"encoding/json"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"go.uber.org/zap"
)

// Handler aliases adapter.Handler so the rest of this package can
// declare handler returns without importing the SDK everywhere.
type Handler = adapter.Handler

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	OK    bool      `json:"ok"`
	Data  any       `json:"data,omitempty"`
	Error *rpcError `json:"error,omitempty"`
}

// success marshals an OK envelope and returns it as the SDK-handler
// triple `(body, content-type, nil)`. The SDK will Ack + Reply.
func success(data any) ([]byte, string, error) {
	body, err := json.Marshal(rpcResponse{OK: true, Data: data})
	if err != nil {
		return nil, "", err
	}
	return body, "application/json", nil
}

// failure is the error-path counterpart. err is returned as nil so the
// SDK does NOT Nack — the structured `code` + `message` is what
// callers consume.
func failure(code string, cause error, logger *zap.Logger) ([]byte, string, error) {
	if logger != nil {
		logger.Error("adapter rpc handler failed",
			zap.String("error_code", code),
			zap.Error(cause),
		)
	}
	body, err := json.Marshal(rpcResponse{
		OK: false,
		Error: &rpcError{
			Code:    code,
			Message: cause.Error(),
		},
	})
	if err != nil {
		return nil, "", err
	}
	return body, "application/json", nil
}
