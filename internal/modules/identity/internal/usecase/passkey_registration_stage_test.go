package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

type stubPasskeyStageStore struct {
	calls     int
	session   [32]byte
	intent    string
	evidence  PasskeyRegistrationEvidence
	candidate PasskeyRegistrationCandidate
	event     string
	ok        bool
	err       error
}

func (s *stubPasskeyStageStore) Stage(_ context.Context, session [32]byte,
	intent string, evidence PasskeyRegistrationEvidence,
	candidate PasskeyRegistrationCandidate, event string) (bool, error) {
	s.calls++
	s.session, s.intent, s.evidence, s.candidate, s.event =
		session, intent, evidence, candidate, event
	return s.ok, s.err
}

func stageFixture(t *testing.T) (*PasskeyRegistrationStager,
	*stubPasskeyStageStore, *stubPasskeyEvidenceStore,
	*fakeRegistrationProtocol, string) {
	t.Helper()
	proof, expected, protocol, bearer := passkeyProofFixture(t)
	protocol.candidate = PasskeyRegistrationCandidate{
		CredentialID: bytes.Repeat([]byte{0x61}, 32),
		PublicKeyCOSE: bytes.Repeat([]byte{0xa3}, 64),
		AAGUID: make([]byte, 16), UserPresent: true, UserVerified: true,
		AttestationFormat: "none", AttestationType: "none",
	}
	store := &stubPasskeyStageStore{ok: true}
	stager, err := NewPasskeyRegistrationStager(proof, store)
	if err != nil {
		t.Fatal(err)
	}
	return stager, store, expected, protocol, bearer
}

func TestPasskeyStagerPersistsOnlyTheVerifiedNonceAndNonActiveCandidate(t *testing.T) {
	t.Parallel()
	stager, store, expected, protocol, bearer := stageFixture(t)
	intent := "wrg_AAAAAAAAAAAAAAAAAAAAAA"
	raw := []byte(`{"type":"public-key","response":{}}`)
	if err := stager.Stage(context.Background(), bearer, intent, raw); err != nil {
		t.Fatalf("verified protocol result rejected: %v", err)
	}
	digest, err := sessionTokenDigest(bearer)
	if err != nil || store.calls != 1 || store.intent != intent ||
		store.session != digest || store.evidence != expected.evidence ||
		protocol.calls != 1 || store.event[:4] != "evt_" || len(store.event) != 26 ||
		len(store.candidate.CredentialID) != 32 || !store.candidate.UserVerified {
		t.Fatal("stager did not bind exact v31 nonce and trusted proof")
	}
}

func TestPasskeyStagerFailsClosedBeforeAnyDatabaseMutation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"protocol denies", "evidence absent", "wrong session", "too short ID",
		"too short COSE", "missing attestation format",
		"missing attestation type", "missing user verification",
		"bad backup flags", "oversized proof",
	} {
		t.Run(scenario, func(t *testing.T) {
			stager, store, evidence, protocol, bearer := stageFixture(t)
			raw := []byte(`{"type":"public-key"}`)
			switch scenario {
			case "protocol denies":
				protocol.err = ErrPasskeyProtocolDenied
			case "evidence absent":
				evidence.found = false
			case "wrong session":
				bearer = "invalid session"
			case "too short ID":
				protocol.candidate.CredentialID = []byte{1}
			case "too short COSE":
				protocol.candidate.PublicKeyCOSE = []byte{1}
			case "missing attestation format":
				protocol.candidate.AttestationFormat = ""
			case "missing attestation type":
				protocol.candidate.AttestationType = ""
			case "missing user verification":
				protocol.candidate.UserVerified = false
			case "bad backup flags":
				protocol.candidate.BackupState = true
			case "oversized proof":
				raw = make([]byte, (128<<10)+1)
			}
			if err := stager.Stage(context.Background(), bearer,
				"wrg_AAAAAAAAAAAAAAAAAAAAAA", raw); !errors.Is(err, ErrPasskeyStageDenied) ||
				store.calls != 0 {
				t.Fatalf("untrusted proof was staged: %v", err)
			}
		})
	}
}

func TestPasskeyStagerCommitAmbiguityNeverClaimsSuccess(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"db denies", "db ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			stager, store, _, _, bearer := stageFixture(t)
			if scenario == "db denies" {
				store.ok = false
			} else {
				store.err = errors.New("lost COMMIT acknowledgement")
			}
			err := stager.Stage(context.Background(), bearer,
				"wrg_AAAAAAAAAAAAAAAAAAAAAA", []byte(`{}`))
			if scenario == "db denies" && !errors.Is(err, ErrPasskeyStageDenied) ||
				scenario == "db ambiguous" && !errors.Is(err, ErrPasskeyStageUnavailable) ||
				store.calls != 1 {
				t.Fatalf("committed-or-denied DB stage incorrectly reported: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stager, store, _, _, bearer := stageFixture(t)
	if err := stager.Stage(ctx, bearer, "wrg_AAAAAAAAAAAAAAAAAAAAAA",
		[]byte(`{}`)); !errors.Is(err, ErrPasskeyStageUnavailable) || store.calls != 0 {
		t.Fatal("canceled stage touched credential store")
	}
	if _, err := NewPasskeyRegistrationStager(nil, store); !errors.Is(err, ErrPasskeyStageUnavailable) {
		t.Fatal("stager allowed absent maintained-library verifier")
	}
	if _, err := NewPasskeyRegistrationStager(stager.proof, nil); !errors.Is(err, ErrPasskeyStageUnavailable) {
		t.Fatal("stager allowed absent isolated DB transaction")
	}
}
