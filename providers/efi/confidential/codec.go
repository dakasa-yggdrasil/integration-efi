package confidential

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/nacl/box"
)

var ErrInvalid = errors.New("efi: invalid confidential request")
var ErrExpired = errors.New("efi: confidential authorization expired")
var ErrHeld = errors.New("efi: confidential operation held")

type Receiver struct {
	public  RuntimePublic
	private *[32]byte
}

func NewReceiver(version string) (*Receiver, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, ErrHeld
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, ErrHeld
	}
	return &Receiver{RuntimePublic{SchemaVersion, Algorithm, version, id.String(), base64.StdEncoding.EncodeToString(pub[:])}, priv}, nil
}
func (r *Receiver) Metadata() RuntimePublic { return r.public }

func DecodePublic(raw []byte) (RuntimePublic, error) {
	var p RuntimePublic
	if len(raw) > MaxPublicMetadata || DecodeStrict(raw, &p) != nil || p.SchemaVersion != SchemaVersion || p.Algorithm != Algorithm || !canonicalUUID(p.KeyID) || !validVersion(p.AdapterVersion) {
		return RuntimePublic{}, ErrInvalid
	}
	if _, err := PublicKey(p.PublicKey); err != nil {
		return RuntimePublic{}, ErrInvalid
	}
	return p, nil
}

// PublicKey rejects noncanonical encoding and low-order X25519 peers. NaCl
// anonymous boxes themselves do not reject all such peers or authenticate sender.
func PublicKey(encoded string) (*[32]byte, error) {
	b, err := Decode32(encoded)
	if err != nil {
		return nil, ErrInvalid
	}
	peer, err := ecdh.X25519().NewPublicKey(b[:])
	if err != nil {
		return nil, ErrInvalid
	}
	var probe [32]byte
	probe[0] = 1
	private, err := ecdh.X25519().NewPrivateKey(probe[:])
	if err != nil {
		return nil, ErrInvalid
	}
	if _, err := private.ECDH(peer); err != nil {
		return nil, ErrInvalid
	}
	return b, nil
}
func Decode32(encoded string) (*[32]byte, error) {
	b, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != encoded {
		return nil, ErrInvalid
	}
	var out [32]byte
	copy(out[:], b)
	return &out, nil
}
func Fingerprint(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// Tag frames each field as uint32 big-endian byte length followed by exact UTF-8
// bytes. Domain is KeyTagDomain or URLTagDomain. Step/op are intentionally absent.
func Tag(secret *[32]byte, domain, invocation, value string) (string, error) {
	if secret == nil || (domain != KeyTagDomain && domain != URLTagDomain) || !canonicalUUID(invocation) {
		return "", ErrInvalid
	}
	h := hmac.New(sha256.New, secret[:])
	for _, field := range []string{domain, invocation, value} {
		if uint64(len(field)) > uint64(^uint32(0)) {
			return "", ErrInvalid
		}
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(field)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CanonicalJSON uses Go encoding/json's compact, recursively sorted object keys,
// HTML escaping and UseNumber decimal spelling. No trailing LF. It is the intent
// encoding only; B7 independently hashes jq -cS of COMPLETE LegacyOutput plus LF.
func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	var generic any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&generic) != nil {
		return nil, ErrInvalid
	}
	return json.Marshal(generic)
}
func IntentHash(intent Intent) (string, error) {
	raw, err := CanonicalJSON(intent)
	if err != nil || len(raw) > MaxInputPlaintext {
		return "", ErrInvalid
	}
	return Fingerprint(raw), nil
}

func SealInput(request Request, recipient RuntimePublic, randomness io.Reader) (Input, error) {
	pub, err := PublicKey(recipient.PublicKey)
	if err != nil {
		return Input{}, ErrInvalid
	}
	if request.Binding.ReceiverKeyID != recipient.KeyID || request.Binding.ReceiverPublicKeySHA256 != Fingerprint(pub[:]) {
		return Input{}, ErrInvalid
	}
	hash, err := IntentHash(request.Intent)
	if err != nil || hash != request.IntentSHA256 {
		return Input{}, ErrInvalid
	}
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > MaxInputPlaintext {
		return Input{}, ErrInvalid
	}
	sealed, err := box.SealAnonymous(nil, raw, pub, randomness)
	if err != nil {
		return Input{}, ErrHeld
	}
	return Input{InputEnvelope{SchemaVersion, Algorithm, recipient.KeyID, base64.StdEncoding.EncodeToString(sealed)}}, nil
}

func (r *Receiver) OpenInput(input InputEnvelope) (Request, error) {
	var req Request
	if r == nil || r.private == nil || input.SchemaVersion != SchemaVersion || input.Algorithm != Algorithm || input.RecipientKeyID != r.public.KeyID {
		return req, ErrInvalid
	}
	sealed, err := decodeCiphertext(input.Ciphertext, MaxInputPlaintext)
	if err != nil {
		return req, err
	}
	pub, err := PublicKey(r.public.PublicKey)
	if err != nil {
		return req, ErrInvalid
	}
	plain, ok := box.OpenAnonymous(nil, sealed, pub, r.private)
	if !ok || len(plain) > MaxInputPlaintext || DecodeStrict(plain, &req) != nil {
		return req, ErrInvalid
	}
	hash, err := IntentHash(req.Intent)
	if err != nil || hash != req.IntentSHA256 || req.SchemaVersion != SchemaVersion || req.Binding.ReceiverKeyID != r.public.KeyID || req.Binding.ReceiverPublicKeySHA256 != Fingerprint(pub[:]) {
		return Request{}, ErrInvalid
	}
	resultPub, err := PublicKey(req.ResultPublicKey)
	if err != nil || Fingerprint(resultPub[:]) != req.Binding.ResultPublicKeySHA256 {
		return Request{}, ErrInvalid
	}
	k, err := Decode32(req.ComparisonSecret)
	if err != nil || Fingerprint(k[:]) != req.Binding.ComparisonSecretSHA256 {
		return Request{}, ErrInvalid
	}
	return req, nil
}

func SealOutput(req Request, legacy, guard json.RawMessage, proof Proof, randomness io.Reader) (Output, error) {
	return SealOutputWithConfirmation(req, legacy, guard, nil, proof, randomness)
}

func SealOutputWithConfirmation(req Request, legacy, guard, confirmation json.RawMessage, proof Proof, randomness io.Reader) (Output, error) {
	pub, err := PublicKey(req.ResultPublicKey)
	if err != nil {
		return Output{}, ErrInvalid
	}
	plain := PlainOutput{SchemaVersion, req.Binding, req.IntentSHA256, legacy, guard, confirmation}
	raw, err := json.Marshal(plain)
	if err != nil || len(raw) > MaxOutputPlaintext {
		return Output{}, ErrHeld
	}
	sealed, err := box.SealAnonymous(nil, raw, pub, randomness)
	if err != nil {
		return Output{}, ErrHeld
	}
	out := Output{Privacy: Privacy{SchemaVersion, Algorithm, base64.StdEncoding.EncodeToString(sealed)}, Proof: proof}
	if req.Binding.Purpose == "ensure" {
		out.ID = req.Binding.AuditResourceID
	}
	return out, nil
}

// OpenOutput only opens a bounded box. The caller MUST compare Binding and intent
// to its immutable attempt and independently recompute the complete public proof.
func OpenOutput(output Output, public, private *[32]byte) (PlainOutput, error) {
	var plain PlainOutput
	if output.Privacy.SchemaVersion != SchemaVersion || output.Privacy.Algorithm != Algorithm || public == nil || private == nil {
		return plain, ErrInvalid
	}
	sealed, err := decodeCiphertext(output.Privacy.Ciphertext, MaxOutputPlaintext)
	if err != nil {
		return plain, err
	}
	raw, ok := box.OpenAnonymous(nil, sealed, public, private)
	if !ok || len(raw) > MaxOutputPlaintext || DecodeStrict(raw, &plain) != nil || plain.SchemaVersion != SchemaVersion {
		return PlainOutput{}, ErrInvalid
	}
	return plain, nil
}

func decodeCiphertext(encoded string, maximum int) ([]byte, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(maximum+box.AnonymousOverhead) {
		return nil, ErrInvalid
	}
	b, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(b) < box.AnonymousOverhead || len(b) > maximum+box.AnonymousOverhead || base64.StdEncoding.EncodeToString(b) != encoded {
		return nil, ErrInvalid
	}
	return b, nil
}

func ValidateTime(binding Binding, now time.Time, permitExpiredConfirm bool) error {
	issued, err := time.Parse(time.RFC3339Nano, binding.IssuedAt)
	if err != nil || issued.UTC().Format(time.RFC3339Nano) != binding.IssuedAt {
		return ErrInvalid
	}
	expires, err := time.Parse(time.RFC3339Nano, binding.ExpiresAt)
	if err != nil || expires.UTC().Format(time.RFC3339Nano) != binding.ExpiresAt || !expires.After(issued) || expires.After(issued.Add(5*time.Minute)) || issued.After(now.Add(30*time.Second)) {
		return ErrInvalid
	}
	if !now.Before(expires) && !(permitExpiredConfirm && binding.Purpose == "confirm") {
		return ErrExpired
	}
	return nil
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func validVersion(value string) bool {
	if len(value) > 32 {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 8 {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// DecodeStrict rejects duplicate members (including inside RawMessage), unknown
// typed fields and trailing values without exposing field names or payload text.
func DecodeStrict(raw []byte, dest any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if uniqueValue(d, 0) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	if d.Decode(dest) != nil {
		return ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	// encoding/json matches typed fields case-insensitively and accepts omitted
	// zero fields. Round-trip canonical equality closes both ambiguities while
	// preserving every member inside explicitly open RawMessage DTOs.
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var original any
	if d.Decode(&original) != nil {
		return ErrInvalid
	}
	want, err := CanonicalJSON(original)
	if err != nil {
		return ErrInvalid
	}
	got, err := CanonicalJSON(dest)
	if err != nil || !bytes.Equal(want, got) {
		return ErrInvalid
	}
	return nil
}
func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 64 {
		return ErrInvalid
	}
	t, err := d.Token()
	if err != nil {
		return ErrInvalid
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			t, err := d.Token()
			if err != nil {
				return ErrInvalid
			}
			key, ok := t.(string)
			if !ok || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
			if uniqueValue(d, depth+1) != nil {
				return ErrInvalid
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrInvalid
		}
	case '[':
		for d.More() {
			if uniqueValue(d, depth+1) != nil {
				return ErrInvalid
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// SafeText validates public binding identifiers, never returns the rejected text.
func SafeText(s string, maximum int) bool {
	return s != "" && len(s) <= maximum && !strings.ContainsAny(s, "\x00\r\n\t") && strings.TrimSpace(s) == s
}
