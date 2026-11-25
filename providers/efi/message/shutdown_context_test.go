package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ad "github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter"
	sdk "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	sdkhttp "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc/http"
	"go.uber.org/zap"
)

type blockedProvider struct {
	started, canceled chan struct{}
	release           chan struct{}
	once              sync.Once
	calls             atomic.Int32
	body              []byte
}

func (f *blockedProvider) finish() { f.once.Do(func() { close(f.release) }) }
func awaitContextSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("missing original lifecycle: " + what)
	}
}

func newBlockedEFI(t *testing.T, operation string, blockOAuth bool) *blockedProvider {
	t.Helper()
	f := &blockedProvider{started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/oauth/token" && !blockOAuth {
			_, _ = io.WriteString(w, `{"access_token":"fixture-original"}`)
			return
		}
		if f.calls.Add(1) == 1 {
			close(f.started)
		}
		select {
		case <-r.Context().Done():
			close(f.canceled)
		case <-f.release:
			_, _ = io.WriteString(w, `{"txid":"fixture","webhookUrl":"https://ci.example/webhook","status":"ATIVA"}`)
		}
	}))
	t.Cleanup(func() { f.finish(); server.Close() })
	input := map[string]any{"txid": "fixture"}
	if operation == ad.OperationObserveWebhookSubscriptions {
		input = map[string]any{"chave": "fixture"}
	}
	if operation == ad.OperationObserveAutomaticWebhooks {
		input = map[string]any{"kind": "cobr"}
	}
	f.body, _ = json.Marshal(map[string]any{"operation": operation, "input": input, "integration": map[string]any{
		"instance": map[string]any{"name": "efi-context-fixture"}, "instance_spec": map[string]any{
			"config":      map[string]any{"base_url": server.URL, "mtls_enabled": false},
			"credentials": map[string]any{"efi_client_key_id": "fixture-id", "efi_client_secret": "fixture-only"}}}})
	return f
}
func TestOriginalEFIContextCancelsOAuthReconcileAndLegacyReads(t *testing.T) {
	for _, operation := range []string{ad.OperationObserveCharges, ad.OperationObserveWebhookSubscriptions, ad.OperationObserveAutomaticWebhooks} {
		for _, oauth := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/oauth=%t", operation, oauth), func(t *testing.T) {
				f := newBlockedEFI(t, operation, oauth)
				a := sdk.New(sdk.Config{Provider: ad.Provider})
				ad.WireReconcilers(a, "efi-context-fixture")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				result := make(chan []byte, 1)
				done := make(chan struct{})
				t.Cleanup(func() { cancel(); f.finish(); awaitContextSignal(t, done, "handler cleanup") })
				go func() {
					defer close(done)
					body, _, _ := ExecuteHandler(zap.NewNop(), a)(ctx, rpc.Delivery{Body: f.body})
					result <- body
				}()
				awaitContextSignal(t, f.started, "original OAuth/provider request")
				cancel()
				awaitContextSignal(t, f.canceled, "provider cancellation")
				select {
				case body := <-result:
					var response rpcResponse
					if json.Unmarshal(body, &response) != nil || response.OK {
						t.Fatal("cancelled original request became success")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("original handler did not return")
				}
				if f.calls.Load() != 1 {
					t.Fatal("cancelled original request was replayed")
				}
			})
		}
	}
}

type replyObservedHTTP struct {
	*sdkhttp.Transport
	ready, stopping, reply, releaseReply, settled chan struct{}
}

func (o *replyObservedHTTP) Consume(cfg rpc.ConsumerConfig) (rpc.Subscription, error) {
	if cfg.Endpoint != "execute" {
		return o.Transport.Consume(cfg)
	}
	handler := cfg.Handler
	cfg.Handler = func(ctx context.Context, d rpc.Delivery) error {
		reply, ack := d.ReplyFn, d.AckFn
		d.ReplyFn = func(ctx context.Context, body []byte, kind string) error {
			close(o.reply)
			<-o.releaseReply
			return reply(ctx, body, kind)
		}
		d.AckFn = func() error { err := ack(); close(o.settled); return err }
		return handler(ctx, d)
	}
	sub, err := o.Transport.Consume(cfg)
	if err != nil {
		return nil, err
	}
	closer, ok := sub.(contextSubscription)
	if !ok {
		return nil, fmt.Errorf("HTTP subscription has no bounded drain")
	}
	close(o.ready)
	return &observedHTTPSubscription{contextSubscription: closer, stopping: o.stopping}, nil
}

type contextSubscription interface {
	rpc.Subscription
	StopAdmission()
	CloseContext(context.Context) error
}

type observedHTTPSubscription struct {
	contextSubscription
	stopping chan struct{}
	once     sync.Once
}

func (s *observedHTTPSubscription) StopAdmission() {
	s.contextSubscription.StopAdmission()
	s.once.Do(func() { close(s.stopping) })
}

func (s *observedHTTPSubscription) CloseContext(ctx context.Context) error {
	return s.contextSubscription.CloseContext(ctx)
}

func TestPlannedStopJoinsOriginalEFIProviderReplyAndSettlement(t *testing.T) {
	f := newBlockedEFI(t, ad.OperationObserveCharges, false)
	mux := http.NewServeMux()
	o := &replyObservedHTTP{Transport: sdkhttp.New(&sdkhttp.Transport{Mux: mux}), ready: make(chan struct{}), stopping: make(chan struct{}),
		reply: make(chan struct{}), releaseReply: make(chan struct{}), settled: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(o.releaseReply) }) }
	a := sdk.New(sdk.Config{Provider: ad.Provider, DefaultTimeout: 10 * time.Second}).Transport(o)
	ad.WireReconcilers(a, "efi-context-fixture")
	a.Register("execute", ExecuteHandler(zap.NewNop(), a))
	ctx, stop := context.WithCancel(context.Background())
	runDone, requestDone := make(chan struct{}), make(chan struct{})
	var runErr error
	requestStarted := false
	t.Cleanup(func() {
		stop()
		f.finish()
		release()
		if requestStarted {
			awaitContextSignal(t, requestDone, "original HTTP request cleanup")
		}
		awaitContextSignal(t, runDone, "original SDK drain cleanup")
	})
	go func() { runErr = a.Run(ctx); close(runDone) }()
	awaitContextSignal(t, o.ready, "SDK execute subscription")
	requestStarted = true
	go func() {
		defer close(requestDone)
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/rpc/execute", bytes.NewReader(f.body)))
	}()
	awaitContextSignal(t, f.started, "original provider request")
	stop()
	awaitContextSignal(t, o.stopping, "the original HTTP subscription drain")
	select {
	case <-runDone:
		t.Fatal("planned stop returned with original provider HTTP still active")
	case <-f.canceled:
		t.Fatal("planned stop canceled the admitted provider HTTP request")
	case <-time.After(100 * time.Millisecond):
	}
	f.finish()
	awaitContextSignal(t, o.reply, "original reply")
	select {
	case <-runDone:
		t.Fatal("planned stop returned before original reply/settlement")
	case <-o.settled:
		t.Fatal("original request settled before its reply returned")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	awaitContextSignal(t, o.settled, "original settlement")
	awaitContextSignal(t, requestDone, "original request return")
	select {
	case <-runDone:
		if runErr != nil {
			t.Fatal("original drain failed", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("planned stop did not receive the original completed drain")
	}
	if f.calls.Load() != 1 {
		t.Fatal("planned stop retried the original provider request")
	}
}
