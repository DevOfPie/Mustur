package web

// A virtual authenticator: enough of a security key to finish a real ceremony.
//
// Until this existed, every test here started after the passkey — invite,
// redeem, mint a session cookie — and the ceremony was the one part of
// authentication nothing exercised. That is the wrong part to leave untested.
// The library does the cryptography, but the wiring around it is Mustur's: what
// challenge was stored, whether it can be spent twice, which account a user
// handle resolves to, whether a credential registered on one site works on
// another. All of that is reachable without a browser.
//
// What this is not: a claim about real authenticators. It signs with ES256 and
// says nothing about attestation, user presence hardware, or a phone. It is a
// correct client of this server's protocol, which is what makes it able to
// prove the server is a correct server.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

type authenticator struct {
	key *ecdsa.PrivateKey
	id  []byte
	// count is the signature counter. Real authenticators either move it or
	// leave it at zero forever; this one moves it, because a counter that never
	// moves would not exercise the server's handling of one that does.
	count uint32
}

func newAuthenticator(t *testing.T) *authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 20)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	return &authenticator{key: key, id: id}
}

// b64 is WebAuthn's encoding everywhere it crosses JSON.
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// creation is what the server sends to begin a registration, cut down to what
// an authenticator actually needs from it.
type creation struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"publicKey"`
}

// request is the same for a sign-in.
type request struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RPID      string `json:"rpId"`
	} `json:"publicKey"`
}

// create registers this authenticator against the challenge the server issued
// and returns the body a browser would post back.
func (a *authenticator) create(t *testing.T, origin string, begin []byte) []byte {
	t.Helper()
	var opts creation
	if err := json.Unmarshal(begin, &opts); err != nil {
		t.Fatalf("the server's registration options did not parse: %v", err)
	}
	if opts.PublicKey.Challenge == "" || opts.PublicKey.RP.ID == "" {
		t.Fatalf("the server issued no challenge or no relying party: %s", begin)
	}
	clientData := a.clientData(t, "webauthn.create", opts.PublicKey.Challenge, origin)

	// The attested credential comes with the key inside the authenticator data,
	// which is what makes registration self-contained.
	authData := a.authData(opts.PublicKey.RP.ID, true)
	att, err := cbor.Marshal(map[string]any{
		// "none": this authenticator attests to nothing about itself, which is
		// what a passkey on a phone does and what this server asks for.
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"id":    b64(a.id),
		"rawId": b64(a.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"attestationObject": b64(att),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// get signs an assertion. handle is the user handle the server committed to at
// registration, which a discoverable credential returns without being asked.
func (a *authenticator) get(t *testing.T, origin string, begin []byte, handle []byte) []byte {
	t.Helper()
	var opts request
	if err := json.Unmarshal(begin, &opts); err != nil {
		t.Fatalf("the server's sign-in options did not parse: %v", err)
	}
	if opts.PublicKey.Challenge == "" {
		t.Fatalf("the server issued no challenge: %s", begin)
	}
	clientData := a.clientData(t, "webauthn.get", opts.PublicKey.Challenge, origin)
	authData := a.authData(opts.PublicKey.RPID, false)

	digest := sha256.Sum256(append(append([]byte{}, authData...), hash(clientData)...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"id":    b64(a.id),
		"rawId": b64(a.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"authenticatorData": b64(authData),
			"signature":         b64(sig),
			"userHandle":        b64(handle),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func (a *authenticator) clientData(t *testing.T, kind, challenge, origin string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":        kind,
		"challenge":   challenge,
		"origin":      origin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// authData is the structure every WebAuthn response carries: which site this
// is for, what the person did, and — at registration — the key itself.
func (a *authenticator) authData(rpID string, attested bool) []byte {
	rpHash := sha256.Sum256([]byte(rpID))
	out := append([]byte{}, rpHash[:]...)

	// Present and verified. Both are the truthful answer for an authenticator
	// that just asked somebody for a fingerprint, which is what the server asks
	// for and what it must accept.
	flags := byte(0x01 | 0x04)
	if attested {
		flags |= 0x40
	}
	out = append(out, flags)

	a.count++
	var counter [4]byte
	binary.BigEndian.PutUint32(counter[:], a.count)
	out = append(out, counter[:]...)

	if !attested {
		return out
	}
	out = append(out, make([]byte, 16)...) // AAGUID: this authenticator claims no model
	var idLen [2]byte
	binary.BigEndian.PutUint16(idLen[:], uint16(len(a.id)))
	out = append(out, idLen[:]...)
	out = append(out, a.id...)
	return append(out, a.coseKey()...)
}

// coseKey is the public key in the form WebAuthn stores it: an ES256 key as a
// CBOR map with integer labels.
func (a *authenticator) coseKey() []byte {
	x := make([]byte, 32)
	y := make([]byte, 32)
	a.key.X.FillBytes(x)
	a.key.Y.FillBytes(y)
	key, err := cbor.Marshal(map[int]any{
		1:  2,  // kty: EC2
		3:  -7, // alg: ES256
		-1: 1,  // crv: P-256
		-2: x,
		-3: y,
	})
	if err != nil {
		// Unreachable: the map holds only integers and byte slices.
		panic(err)
	}
	return key
}

func hash(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
