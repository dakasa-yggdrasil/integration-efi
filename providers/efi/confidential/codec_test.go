package confidential

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// These vectors contain only synthetic inputs. Standard NaCl key generation and
// expected crypto outputs execute in CI, not while preparing this fixture.
type interopVector struct {
	Version                int             `json:"fixture_version"`
	Attribution            string          `json:"attribution"`
	ReceiverSeed           string          `json:"receiver_private_seed_ascii"`
	ResultSeed             string          `json:"result_private_seed_ascii"`
	ComparisonSeed         string          `json:"comparison_secret_ascii"`
	InputEphemeralSeed     string          `json:"input_ephemeral_seed_ascii"`
	OutputEphemeralSeed    string          `json:"output_ephemeral_seed_ascii"`
	ReceiverKeyID          string          `json:"receiver_key_id"`
	AlternateReceiverKeyID string          `json:"alternate_receiver_key_id"`
	InvocationID           string          `json:"invocation_id"`
	AuditResourceID        string          `json:"audit_resource_id"`
	TargetKey              string          `json:"target_key"`
	CanonicalBase          string          `json:"canonical_base"`
	ExactLegacyURL         string          `json:"exact_legacy_url"`
	IssuedAt               string          `json:"issued_at"`
	ExpiresAt              string          `json:"expires_at"`
	CanonicalIntent        json.RawMessage `json:"canonical_intent_fixture"`
	CanonicalNote          string          `json:"canonical_intent_fixture_note"`
}

type interopState struct {
	vector                      interopVector
	receiver                    *Receiver
	resultPublic, resultPrivate *[32]byte
	comparison                  *[32]byte
	request                     Request
}

func loadInteropVector(t *testing.T) interopVector {
	t.Helper()
	raw, err := os.ReadFile("testdata/interop-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var v interopVector
	if err := json.Unmarshal(raw, &v); err != nil || v.Version != 1 {
		t.Fatal("invalid shared synthetic input vector")
	}
	for _, seed := range []string{v.ReceiverSeed, v.ResultSeed, v.ComparisonSeed, v.InputEphemeralSeed, v.OutputEphemeralSeed} {
		if len(seed) != 32 {
			t.Fatal("shared synthetic seed is not exactly32 bytes")
		}
	}
	return v
}

func independentFingerprint(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func independentCanonical(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var generic any
	if err := d.Decode(&generic); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newInteropState(t *testing.T) interopState {
	t.Helper()
	v := loadInteropVector(t)
	receiverPublic, receiverPrivate, err := box.GenerateKey(strings.NewReader(v.ReceiverSeed))
	if err != nil {
		t.Fatal(err)
	}
	resultPublic, resultPrivate, err := box.GenerateKey(strings.NewReader(v.ResultSeed))
	if err != nil {
		t.Fatal(err)
	}
	var comparison [32]byte
	copy(comparison[:], v.ComparisonSeed)
	runtime := RuntimePublic{1, "nacl-box-seal-anonymous-v1", "2.5.7", v.ReceiverKeyID, base64.StdEncoding.EncodeToString(receiverPublic[:])}
	var intent Intent
	if err := json.Unmarshal(v.CanonicalIntent, &intent); err != nil {
		t.Fatal(err)
	}
	intent.Binding.ReceiverPublicKeySHA256 = independentFingerprint(receiverPublic[:])
	intent.Binding.ResultPublicKeySHA256 = independentFingerprint(resultPublic[:])
	intent.Binding.ComparisonSecretSHA256 = independentFingerprint(comparison[:])
	intent.ResultPublicKey = base64.StdEncoding.EncodeToString(resultPublic[:])
	intent.ComparisonSecret = base64.StdEncoding.EncodeToString(comparison[:])
	req := Request{Intent: intent, IntentSHA256: independentFingerprint(independentCanonical(t, intent))}
	return interopState{v, &Receiver{public: runtime, private: receiverPrivate}, resultPublic, resultPrivate, &comparison, req}
}

func marshalTest(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func standardInput(t *testing.T, state interopState, plaintext []byte) InputEnvelope {
	t.Helper()
	publicBytes, err := base64.StdEncoding.DecodeString(state.receiver.Metadata().PublicKey)
	if err != nil || len(publicBytes) != 32 {
		t.Fatal("invalid fixture public key")
	}
	var public [32]byte
	copy(public[:], publicBytes)
	sealed, err := box.SealAnonymous(nil, plaintext, &public, strings.NewReader(state.vector.InputEphemeralSeed))
	if err != nil {
		t.Fatal(err)
	}
	return InputEnvelope{1, "nacl-box-seal-anonymous-v1", state.vector.ReceiverKeyID, base64.StdEncoding.EncodeToString(sealed)}
}

func standardOutput(t *testing.T, state interopState, plaintext []byte) Output {
	t.Helper()
	sealed, err := box.SealAnonymous(nil, plaintext, state.resultPublic, strings.NewReader(state.vector.OutputEphemeralSeed))
	if err != nil {
		t.Fatal(err)
	}
	return Output{Privacy: Privacy{1, "nacl-box-seal-anonymous-v1", base64.StdEncoding.EncodeToString(sealed)}}
}

func TestSharedIntentHasLiteralCanonicalEncodingAndIndependentSHA256(t *testing.T) {
	v := loadInteropVector(t)
	var intent Intent
	if err := DecodeStrict(v.CanonicalIntent, &intent); err != nil {
		t.Fatal("complete canonicalization-only intent fixture was refused")
	}
	// This literal is independently ordered and includes explicit zero/false
	// fields. The fixture's placeholder crypto values test encoding only.
	want := `{"binding":{"audit_resource_id":"00000000-0000-4000-8000-000000000043","comparison_secret_sha256":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","expires_at":"2026-10-31T12:05:00Z","instance_name":"efi-test-fixture","invocation_id":"00000000-0000-4000-8000-000000000042","issued_at":"2026-10-31T12:00:00Z","mode":"efi-observe","operation":"observe_webhook_subscriptions","purpose":"list","receiver_key_id":"00000000-0000-4000-8000-000000000011","receiver_public_key_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","release_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","result_public_key_sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","source_root":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","step_id":"list-before","workflow_id":"observe-test-fixture","workflow_version":1},"comparison_secret":"canonicalization-only-placeholder-not-a-secret","parameters":{"before_state":"","canonical_base":"https://receiver.invalid/base","chave":"00000000-0000-4000-8000-000000000001","fim":"2099-12-31T23:59:59.000Z","inicio":"1970-01-01T00:00:00.000Z","page":0,"page_size":1000,"skip_mtls_validation":false},"result_public_key":"canonicalization-only-placeholder-not-a-public-key","schemaVersion":1}`
	got, err := CanonicalJSON(intent)
	if err != nil || string(got) != want {
		t.Fatal("intent encoding differs from the literal shared wire contract")
	}
	hash, err := IntentHash(intent)
	if err != nil || hash != independentFingerprint([]byte(want)) {
		t.Fatal("intent hash differs from independent SHA256 of literal canonical bytes")
	}
	if bytes.HasSuffix(got, []byte("\n")) {
		t.Fatal("intent encoding acquired B7's separate LF convention")
	}
	encoded, err := CanonicalJSON(map[string]any{"z": json.Number("9007199254740993"), "a": "<&>"})
	if err != nil || string(encoded) != `{"a":"\u003c\u0026\u003e","z":9007199254740993}` {
		t.Fatal("canonical JSON lost exact integer spelling or Go HTML escaping")
	}
}

func TestAnonymousInputInteroperatesWithIndependentStandardNaClBothWays(t *testing.T) {
	state := newInteropState(t)
	input, err := SealInput(state.request, state.receiver.Metadata(), strings.NewReader(state.vector.InputEphemeralSeed))
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, _ := base64.StdEncoding.DecodeString(state.receiver.Metadata().PublicKey)
	var public [32]byte
	copy(public[:], publicBytes)
	cipher, err := base64.StdEncoding.Strict().DecodeString(input.ConfidentialInput.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	plain, ok := box.OpenAnonymous(nil, cipher, &public, state.receiver.private)
	if !ok || !bytes.Equal(plain, marshalTest(t, state.request)) {
		t.Fatal("standard NaCl could not open the complete codec request")
	}
	standard := standardInput(t, state, marshalTest(t, state.request))
	if standard.Ciphertext != input.ConfidentialInput.Ciphertext {
		t.Fatal("deterministic test-only entropy produced a different standard NaCl wire box")
	}
	got, err := state.receiver.OpenInput(standard)
	if err != nil || !bytes.Equal(marshalTest(t, got), marshalTest(t, state.request)) {
		t.Fatal("codec could not open the independently sealed complete request")
	}
	wire := string(marshalTest(t, input))
	for _, private := range []string{state.vector.TargetKey, state.vector.CanonicalBase, state.request.ResultPublicKey, state.request.ComparisonSecret} {
		if strings.Contains(wire, private) {
			t.Fatal("private request parameter escaped the encrypted execute input")
		}
	}
}

func TestAnonymousOutputPreservesCompleteLegacyDTOAndIndependentStandardInterop(t *testing.T) {
	state := newInteropState(t)
	legacy := json.RawMessage(`{"provider":"efi","chave":"00000000-0000-4000-8000-000000000001","webhookUrl":"https://receiver.invalid/base","items":[{"chave":"00000000-0000-4000-8000-000000000001"}],"unknown":{"email":"other-account@example.invalid","cpf":"other-cpf-01234567890","large":9007199254740993,"nested":["<&>",{"keep":true}]}}`)
	proof := Proof{SchemaVersion: 1, Kind: "single", Outcome: "confirmed"}
	output, err := SealOutput(state.request, legacy, nil, proof, strings.NewReader(state.vector.OutputEphemeralSeed))
	if err != nil {
		t.Fatal(err)
	}
	cipher, _ := base64.StdEncoding.Strict().DecodeString(output.Privacy.Ciphertext)
	plain, ok := box.OpenAnonymous(nil, cipher, state.resultPublic, state.resultPrivate)
	if !ok {
		t.Fatal("standard NaCl could not open complete codec output")
	}
	var decoded PlainOutput
	if err := json.Unmarshal(plain, &decoded); err != nil || decoded.Binding != state.request.Binding || decoded.IntentSHA256 != state.request.IntentSHA256 {
		t.Fatal("plaintext binding was not preserved")
	}
	var expected, actual any
	for _, item := range []struct {
		raw  []byte
		dest *any
	}{{legacy, &expected}, {decoded.LegacyOutput, &actual}} {
		d := json.NewDecoder(bytes.NewReader(item.raw))
		d.UseNumber()
		if err := d.Decode(item.dest); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(independentCanonical(t, expected), independentCanonical(t, actual)) || !bytes.Contains(decoded.LegacyOutput, []byte("9007199254740993")) {
		t.Fatal("complete unknown fields, aliases or exact numbers were lost from legacy DTO")
	}
	independent := standardOutput(t, state, marshalTest(t, PlainOutput{SchemaVersion: 1, Binding: state.request.Binding, IntentSHA256: state.request.IntentSHA256, LegacyOutput: legacy}))
	opened, err := OpenOutput(independent, state.resultPublic, state.resultPrivate)
	if err != nil || !bytes.Equal(independentCanonical(t, opened), independentCanonical(t, decoded)) {
		t.Fatal("codec could not open the independently sealed output")
	}
	wire := string(marshalTest(t, output))
	for _, private := range []string{"other-account@example.invalid", "other-cpf-01234567890", state.vector.TargetKey, state.vector.CanonicalBase, `"unknown"`} {
		if strings.Contains(wire, private) {
			t.Fatal("complete legacy provider data escaped the encrypted result")
		}
	}
}

func TestStrictDecodeRejectsCaseOmissionsDuplicatesAndUnknownPrivateMembers(t *testing.T) {
	state := newInteropState(t)
	valid := string(marshalTest(t, state.request))
	for name, raw := range map[string]string{
		"wrong-case":          strings.Replace(valid, `"schemaVersion":`, `"SchemaVersion":`, 1),
		"missing-zero":        strings.Replace(valid, `"page":0,`, "", 1),
		"missing-false":       strings.Replace(valid, `,"skip_mtls_validation":false`, "", 1),
		"duplicate":           strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		"escaped-duplicate":   strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schema\u0056ersion":1`, 1),
		"case-collision":      strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"SchemaVersion":1`, 1),
		"unknown-private-key": strings.TrimSuffix(valid, "}") + `,"other-account@example.invalid":"other-cpf-01234567890"}`,
		"trailing-value":      valid + `{}`,
		"null":                "null",
	} {
		t.Run(name, func(t *testing.T) {
			var request Request
			err := DecodeStrict([]byte(raw), &request)
			if !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "other-account") || strings.Contains(err.Error(), "other-cpf") {
				t.Fatal("invalid typed/duplicate private input was accepted or reflected")
			}
			if _, err := state.receiver.OpenInput(standardInput(t, state, []byte(raw))); !errors.Is(err, ErrInvalid) {
				t.Fatal("sealed malformed input bypassed strict decoding")
			}
		})
	}
	duplicateLegacy := PlainOutput{SchemaVersion: 1, Binding: state.request.Binding, IntentSHA256: state.request.IntentSHA256, LegacyOutput: json.RawMessage(`{"unknown":{"chave":"first","chave":"second"}}`)}
	if _, err := OpenOutput(standardOutput(t, state, marshalTest(t, duplicateLegacy)), state.resultPublic, state.resultPrivate); !errors.Is(err, ErrInvalid) {
		t.Fatal("nested duplicate hidden inside legacy RawMessage was accepted")
	}
	deep := `{"legacy_output":` + strings.Repeat("[", 66) + `0` + strings.Repeat("]", 66) + `}`
	var generic any
	if err := DecodeStrict([]byte(deep), &generic); !errors.Is(err, ErrInvalid) {
		t.Fatal("excessive nesting was accepted")
	}
}

func TestInputAndOutputRejectTamperCrossReceiverAndEncodingDrift(t *testing.T) {
	state := newInteropState(t)
	valid := standardInput(t, state, marshalTest(t, state.request))
	cipher, _ := base64.StdEncoding.DecodeString(valid.Ciphertext)
	cipher[len(cipher)/2] ^= 1
	for name, envelope := range map[string]InputEnvelope{
		"tamper":      {1, Algorithm, state.vector.ReceiverKeyID, base64.StdEncoding.EncodeToString(cipher)},
		"wrong-keyid": {1, Algorithm, state.vector.AlternateReceiverKeyID, valid.Ciphertext},
		"schema":      {2, Algorithm, state.vector.ReceiverKeyID, valid.Ciphertext},
		"algorithm":   {1, "different", state.vector.ReceiverKeyID, valid.Ciphertext},
		"base64-LF":   {1, Algorithm, state.vector.ReceiverKeyID, valid.Ciphertext + "\n"},
		"empty":       {1, Algorithm, state.vector.ReceiverKeyID, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := state.receiver.OpenInput(envelope); !errors.Is(err, ErrInvalid) {
				t.Fatal("tampered input envelope accepted")
			}
		})
	}
	otherSeed := []byte(state.vector.ReceiverSeed)
	otherSeed[15] ^= 1 // Unlike clamped edge bits, this changes the X25519 key.
	otherPublic, otherPrivate, err := box.GenerateKey(bytes.NewReader(otherSeed))
	if err != nil {
		t.Fatal(err)
	}
	other := &Receiver{public: state.receiver.Metadata(), private: otherPrivate}
	other.public.PublicKey = base64.StdEncoding.EncodeToString(otherPublic[:])
	if _, err := other.OpenInput(valid); !errors.Is(err, ErrInvalid) {
		t.Fatal("ciphertext opened on another receiver with the same claimed keyID")
	}
	output := standardOutput(t, state, marshalTest(t, PlainOutput{SchemaVersion: 1, Binding: state.request.Binding, IntentSHA256: state.request.IntentSHA256, LegacyOutput: json.RawMessage(`{}`)}))
	if _, err := OpenOutput(output, otherPublic, otherPrivate); !errors.Is(err, ErrInvalid) {
		t.Fatal("output opened for the wrong result recipient")
	}
	cipher, _ = base64.StdEncoding.DecodeString(output.Privacy.Ciphertext)
	cipher[len(cipher)-1] ^= 1
	output.Privacy.Ciphertext = base64.StdEncoding.EncodeToString(cipher)
	if _, err := OpenOutput(output, state.resultPublic, state.resultPrivate); !errors.Is(err, ErrInvalid) {
		t.Fatal("tampered output was accepted")
	}
}

func TestCodecRejectsLowOrderAndNoncanonicalPublicKeys(t *testing.T) {
	state := newInteropState(t)
	zero := make([]byte, 32)
	one := make([]byte, 32)
	one[0] = 1
	valid := state.receiver.Metadata().PublicKey
	for _, encoded := range []string{
		base64.StdEncoding.EncodeToString(zero), base64.StdEncoding.EncodeToString(one),
		valid + "\n", strings.TrimRight(valid, "="), base64.StdEncoding.EncodeToString(make([]byte, 31)),
	} {
		if _, err := PublicKey(encoded); !errors.Is(err, ErrInvalid) {
			t.Fatal("low-order or noncanonical public key was accepted")
		}
	}
	for _, encoded := range []string{base64.StdEncoding.EncodeToString(zero), base64.StdEncoding.EncodeToString(one)} {
		req := state.request
		req.ResultPublicKey = encoded
		rawKey, _ := base64.StdEncoding.DecodeString(encoded)
		req.Binding.ResultPublicKeySHA256 = independentFingerprint(rawKey)
		req.IntentSHA256 = independentFingerprint(independentCanonical(t, req.Intent))
		if _, err := state.receiver.OpenInput(standardInput(t, state, marshalTest(t, req))); !errors.Is(err, ErrInvalid) {
			t.Fatal("sealed input with low-order result key was accepted")
		}
	}
}

func TestCodecRejectsHashFingerprintAndCompleteSizeFailures(t *testing.T) {
	state := newInteropState(t)
	for name, modify := range map[string]func(*Request){
		"intent-hash":         func(r *Request) { r.IntentSHA256 = strings.Repeat("0", 64) },
		"receiver-hash":       func(r *Request) { r.Binding.ReceiverPublicKeySHA256 = strings.Repeat("0", 64) },
		"result-hash":         func(r *Request) { r.Binding.ResultPublicKeySHA256 = strings.Repeat("0", 64) },
		"comparison-hash":     func(r *Request) { r.Binding.ComparisonSecretSHA256 = strings.Repeat("0", 64) },
		"comparison-encoding": func(r *Request) { r.ComparisonSecret += "\n" },
	} {
		t.Run(name, func(t *testing.T) {
			req := state.request
			modify(&req)
			if name != "intent-hash" {
				req.IntentSHA256 = independentFingerprint(independentCanonical(t, req.Intent))
			}
			if _, err := state.receiver.OpenInput(standardInput(t, state, marshalTest(t, req))); !errors.Is(err, ErrInvalid) {
				t.Fatal("changed private intent/fingerprint was accepted")
			}
		})
	}
	oversize := state.request
	oversize.Parameters.Key = strings.Repeat("x", MaxInputPlaintext)
	oversize.IntentSHA256 = independentFingerprint(independentCanonical(t, oversize.Intent))
	if _, err := SealInput(oversize, state.receiver.Metadata(), strings.NewReader(state.vector.InputEphemeralSeed)); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversize complete request was sealed")
	}
	if _, err := state.receiver.OpenInput(standardInput(t, state, marshalTest(t, oversize))); !errors.Is(err, ErrInvalid) {
		t.Fatal("independently sealed oversize input was opened")
	}
	legacy := json.RawMessage(`{"unknown":"` + strings.Repeat("x", MaxOutputPlaintext) + `"}`)
	if _, err := SealOutput(state.request, legacy, nil, Proof{}, strings.NewReader(state.vector.OutputEphemeralSeed)); !errors.Is(err, ErrHeld) {
		t.Fatal("oversize complete output was truncated or sealed")
	}
	if _, err := OpenOutput(standardOutput(t, state, marshalTest(t, PlainOutput{SchemaVersion: 1, Binding: state.request.Binding, IntentSHA256: state.request.IntentSHA256, LegacyOutput: legacy})), state.resultPublic, state.resultPrivate); !errors.Is(err, ErrInvalid) {
		t.Fatal("independently sealed oversize result was opened")
	}
	if _, err := SealInput(state.request, state.receiver.Metadata(), io.LimitReader(strings.NewReader("too short"), 9)); !errors.Is(err, ErrHeld) {
		t.Fatal("entropy failure did not hold input sealing")
	}
	if _, err := SealOutput(state.request, json.RawMessage(`{}`), nil, Proof{}, io.LimitReader(strings.NewReader("too short"), 9)); !errors.Is(err, ErrHeld) {
		t.Fatal("entropy failure did not hold output sealing")
	}
}

func TestHMACTagMatchesLiteralFrameAndIndependentStandardMAC(t *testing.T) {
	state := newInteropState(t)
	if KeyTagDomain != "efi.webhook.confidential.key.v1" || URLTagDomain != "efi.webhook.confidential.url.v1" {
		t.Fatal("shared tag protocol domains changed")
	}
	frame := []byte("\x00\x00\x00\x1fefi.webhook.confidential.key.v1\x00\x00\x00\x2400000000-0000-4000-8000-000000000042\x00\x00\x00\x2400000000-0000-4000-8000-000000000001")
	mac := hmac.New(sha256.New, []byte(state.vector.ComparisonSeed))
	_, _ = mac.Write(frame)
	got, err := Tag(state.comparison, KeyTagDomain, state.vector.InvocationID, state.vector.TargetKey)
	if err != nil || got != hex.EncodeToString(mac.Sum(nil)) || len(got) != 64 || strings.ToLower(got) != got {
		t.Fatal("key tag differs from independent HMAC of the literal framed bytes")
	}
	// For the exact legacy URL use independently written binary length fields;
	// the literal key case above independently fixes the frame convention.
	var urlFrame bytes.Buffer
	for _, part := range []string{"efi.webhook.confidential.url.v1", state.vector.InvocationID, state.vector.ExactLegacyURL} {
		if err := binary.Write(&urlFrame, binary.BigEndian, uint32(len([]byte(part)))); err != nil {
			t.Fatal(err)
		}
		_, _ = urlFrame.WriteString(part)
	}
	mac = hmac.New(sha256.New, []byte(state.vector.ComparisonSeed))
	_, _ = mac.Write(urlFrame.Bytes())
	urlTag, err := Tag(state.comparison, URLTagDomain, state.vector.InvocationID, state.vector.ExactLegacyURL)
	if err != nil || urlTag != hex.EncodeToString(mac.Sum(nil)) || urlTag == got {
		t.Fatal("exact legacy URL tag differs from independent domain-separated HMAC")
	}
	for _, changed := range []string{strings.ToUpper(state.vector.ExactLegacyURL), state.vector.CanonicalBase, " " + state.vector.ExactLegacyURL} {
		tag, err := Tag(state.comparison, URLTagDomain, state.vector.InvocationID, changed)
		if err != nil || tag == urlTag {
			t.Fatal("tag applied unreviewed trimming/case/query normalization")
		}
	}
	composed, _ := Tag(state.comparison, KeyTagDomain, state.vector.InvocationID, "é")
	decomposed, _ := Tag(state.comparison, KeyTagDomain, state.vector.InvocationID, "e\u0301")
	if composed == decomposed {
		t.Fatal("tag normalized exact UTF8 values")
	}
	unicodeFrame := []byte("\x00\x00\x00\x1fefi.webhook.confidential.key.v1\x00\x00\x00\x2400000000-0000-4000-8000-000000000042\x00\x00\x00\x02\xc3\xa9")
	mac = hmac.New(sha256.New, []byte(state.vector.ComparisonSeed))
	_, _ = mac.Write(unicodeFrame)
	if composed != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("tag length-framed runes instead of exact UTF8 bytes")
	}
	stepChanged := state.request
	stepChanged.Binding.StepID = "confirm-after"
	if independentFingerprint(independentCanonical(t, stepChanged.Intent)) == state.request.IntentSHA256 {
		t.Fatal("step binding was not part of the independent intent")
	}
	sameTag, err := Tag(state.comparison, KeyTagDomain, stepChanged.Binding.InvocationID, stepChanged.Parameters.Key)
	if err != nil || sameTag != got {
		t.Fatal("same invocation's cross-step equality tag changed")
	}
	for _, test := range []struct {
		secret *[32]byte
		domain string
		id     string
	}{{nil, KeyTagDomain, state.vector.InvocationID}, {state.comparison, "unreviewed-domain", state.vector.InvocationID}, {state.comparison, KeyTagDomain, "ABCDEFAB-0000-4000-8000-000000000042"}, {state.comparison, KeyTagDomain, "00000000-0000-0000-0000-000000000000"}} {
		if _, err := Tag(test.secret, test.domain, test.id, "value"); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid HMAC context was accepted")
		}
	}
}

func TestAuthorizationExpiresAtExactBoundaryButNotRecoveryDecryption(t *testing.T) {
	state := newInteropState(t)
	issued, _ := time.Parse(time.RFC3339Nano, state.vector.IssuedAt)
	expires, _ := time.Parse(time.RFC3339Nano, state.vector.ExpiresAt)
	for _, test := range []struct {
		name    string
		now     time.Time
		purpose string
		permit  bool
		want    error
	}{
		{"before-expiry", expires.Add(-time.Nanosecond), "ensure", false, nil},
		{"exact-expiry", expires, "ensure", false, ErrExpired},
		{"after-expiry", expires.Add(time.Hour), "ensure", false, ErrExpired},
		{"confirm-expired-allowed", expires.Add(time.Hour), "confirm", true, nil},
		{"confirm-expired-not-allowed", expires, "confirm", false, ErrExpired},
		{"nonconfirm-cannot-bypass", expires, "list", true, ErrExpired},
		{"issued-30seconds-ahead", issued.Add(-30 * time.Second), "ensure", false, nil},
		{"issued-too-far-ahead", issued.Add(-30*time.Second - time.Nanosecond), "ensure", false, ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			binding := state.request.Binding
			binding.Purpose = test.purpose
			if err := ValidateTime(binding, test.now, test.permit); !errors.Is(err, test.want) {
				t.Fatalf("authorization boundary: got %v want %v", err, test.want)
			}
		})
	}
	for _, modify := range []func(*Binding){
		func(b *Binding) { b.ExpiresAt = issued.Format(time.RFC3339Nano) },
		func(b *Binding) { b.ExpiresAt = expires.Add(time.Nanosecond).Format(time.RFC3339Nano) },
		func(b *Binding) { b.IssuedAt = "2026-10-31T12:00:00+00:00" },
		func(b *Binding) { b.ExpiresAt = "2026-10-31T12:05:00.000Z" },
	} {
		binding := state.request.Binding
		modify(&binding)
		if err := ValidateTime(binding, issued, true); !errors.Is(err, ErrInvalid) {
			t.Fatal("malformed/noncanonical authorization interval accepted")
		}
	}
	output := standardOutput(t, state, marshalTest(t, PlainOutput{SchemaVersion: 1, Binding: state.request.Binding, IntentSHA256: state.request.IntentSHA256, LegacyOutput: json.RawMessage(`{"provider":"efi"}`)}))
	// Opening an original durable result is not a new provider authorization.
	if _, err := OpenOutput(output, state.resultPublic, state.resultPrivate); err != nil {
		t.Fatal("original result recovery was incorrectly coupled to authorization clock")
	}
}

func TestPublicMetadataIsClosedBoundedAndContainsNoReceiverPrivateKey(t *testing.T) {
	state := newInteropState(t)
	valid := string(marshalTest(t, state.receiver.Metadata()))
	if _, err := DecodePublic([]byte(valid)); err != nil {
		t.Fatal("complete synthetic public runtime metadata was refused")
	}
	for _, raw := range []string{
		strings.Replace(valid, `"schemaVersion":`, `"SchemaVersion":`, 1),
		strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(valid, `"adapterVersion":"2.5.7"`, `"adapterVersion":"2.5.7-unreviewed"`, 1),
		strings.TrimSuffix(valid, "}") + `,"privateKey":"must-not-escape"}`,
		valid + strings.Repeat(" ", MaxPublicMetadata),
	} {
		_, err := DecodePublic([]byte(raw))
		if !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "must-not-escape") {
			t.Fatal("invalid public metadata was accepted or reflected")
		}
	}
	for _, private := range []string{state.vector.ReceiverSeed, base64.StdEncoding.EncodeToString(state.receiver.private[:]), state.vector.ComparisonSeed, state.request.ComparisonSecret} {
		if strings.Contains(valid, private) {
			t.Fatal("public runtime metadata exposed private receiver/comparison material")
		}
	}
}
