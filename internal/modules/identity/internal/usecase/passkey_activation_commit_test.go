package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

type fakePasskeyActivationFinishStore struct {
	calls   int
	request PasskeyActivationFinishRequest
	allowed bool
	oneUse  bool
	err     error
}

func (f *fakePasskeyActivationFinishStore) Finish(_ context.Context,
	request PasskeyActivationFinishRequest) (bool, error) {
	f.calls++
	f.request = request
	if f.oneUse && f.calls > 1 {
		return false, nil
	}
	return f.allowed, f.err
}

func commitFixture(t *testing.T) (
	*PasskeyActivationCommitter, *fakePasskeyActivationFinishStore,
	*fakePasskeyActivationReaderStore, *fakeActivationAssertionProtocol, string,
) {
	t.Helper()
	proof, reader, protocol, bearer := assertionProofFixture(t)
	store := &fakePasskeyActivationFinishStore{allowed: true}
	committer, err := NewPasskeyActivationCommitter(proof, store)
	if err != nil {
		t.Fatal(err)
	}
	return committer, store, reader, protocol, bearer
}

func TestPasskeyActivationCommitHandoffUsesOneVerifiedEvidenceSnapshot(t *testing.T) {
	t.Parallel()
	committer, store, reader, protocol, bearer := commitFixture(t)
	raw := []byte(`{"type":"public-key"}`)
	challengeID := "pav_AAAAAAAAAAAAAAAAAAAAAA"
	if err := committer.Finish(context.Background(), bearer, challengeID, raw); err != nil {
		t.Fatalf("one verified evidence snapshot was rejected: %v", err)
	}
	digest, err := sessionTokenDigest(bearer)
	if err != nil || protocol.calls != 1 || store.calls != 1 ||
		store.request.SessionDigest != digest ||
		store.request.ChallengeID != challengeID ||
		store.request.Expected.Subject != reader.record.Subject ||
		store.request.Expected.Epoch != reader.record.Epoch ||
		store.request.Expected.Nonce != reader.record.Nonce ||
		store.request.Expected.SignCount != reader.record.SignCount ||
		store.request.Expected.BackupEligible != reader.record.BackupEligible ||
		store.request.Expected.BackupState != reader.record.BackupState ||
		store.request.Expected.RPID != reader.record.RPID ||
		store.request.Expected.Origin != reader.record.Origin ||
		store.request.Expected.PolicyGeneration != reader.record.PolicyGeneration ||
		!bytes.Equal(store.request.Expected.CredentialID, reader.record.CredentialID) ||
		!bytes.Equal(store.request.Expected.PublicKeyCOSE, reader.record.PublicKeyCOSE) ||
		!bytes.Equal(store.request.Verified.CredentialID, protocol.result.CredentialID) ||
		store.request.Verified.Counter != protocol.result.Counter ||
		!store.request.Verified.UserVerified ||
		len(store.request.EventID) != 26 || store.request.EventID[:4] != "evt_" {
		t.Fatal("v35 verified evidence was not bound to exact future SQL handoff")
	}
	// A future SQL adapter may retain/change its local byte slices. The
	// proof verifier's original DB snapshot must not alias those fields.
	store.request.Expected.CredentialID[0] ^= 0xff
	store.request.Expected.PublicKeyCOSE[0] ^= 0xff
	store.request.Verified.CredentialID[0] ^= 0xff
	if reader.record.CredentialID[0] != 0x5b ||
		reader.record.PublicKeyCOSE[0] != 0xa3 ||
		protocol.result.CredentialID[0] != 0x5b {
		t.Fatal("future SQL handoff mutated original expected or verified evidence")
	}
}

func TestPasskeyActivationCommitRejectsInvalidOrUncertainEvidence(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"missing challenge", "wrong bearer", "invalid proof", "counter clone",
		"unexpected credential", "unverified user", "expired challenge",
		"reader unavailable", "verifier unavailable", "database denied",
		"database unknown commit", "entropy unavailable",
	} {
		t.Run(scenario, func(t *testing.T) {
			committer, store, reader, protocol, bearer := commitFixture(t)
			id := "pav_AAAAAAAAAAAAAAAAAAAAAA"
			raw := []byte(`{"type":"public-key"}`)
			want := ErrPasskeyActivationCommitDenied
			switch scenario {
			case "missing challenge":
				id = "not-an-issued-id"
			case "wrong bearer":
				bearer = "invalid-bearer"
			case "invalid proof":
				protocol.err = ErrPasskeyAssertionDenied
			case "counter clone":
				reader.record.SignCount = 2
				protocol.result.Counter = 2
			case "unexpected credential":
				protocol.result.CredentialID = bytes.Repeat([]byte{0x44}, 32)
			case "unverified user":
				protocol.result.UserVerified = false
			case "expired challenge":
				reader.found = false
			case "reader unavailable":
				reader.err = errors.New("read failed")
				want = ErrPasskeyActivationCommitUnavailable
			case "verifier unavailable":
				protocol.err = errors.New("library unavailable")
				want = ErrPasskeyActivationCommitUnavailable
			case "database denied":
				store.allowed = false
			case "database unknown commit":
				store.err = errors.New("commit response lost")
				want = ErrPasskeyActivationCommitUnavailable
			case "entropy unavailable":
				committer.random = bytes.NewReader(nil)
				want = ErrPasskeyActivationCommitUnavailable
			}
			err := committer.Finish(context.Background(), bearer, id, raw)
			if !errors.Is(err, want) {
				t.Fatalf("%s: wrong error class: %v", scenario, err)
			}
			if scenario == "database denied" || scenario == "database unknown commit" {
				if store.calls != 1 {
					t.Fatalf("%s: SQL call unexpectedly retried/skipped", scenario)
				}
			} else if store.calls != 0 {
				t.Fatalf("%s: malformed/unverified evidence reached commit authority", scenario)
			}
		})
	}
}

func TestPasskeyActivationCommitCannotTreatReadOnlyVerificationAsReplayProtection(t *testing.T) {
	t.Parallel()
	committer, store, _, protocol, bearer := commitFixture(t)
	store.oneUse = true
	id := "pav_AAAAAAAAAAAAAAAAAAAAAA"
	raw := []byte(`{"type":"public-key"}`)
	if err := committer.Finish(context.Background(), bearer, id, raw); err != nil {
		t.Fatal(err)
	}
	if err := committer.Finish(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyActivationCommitDenied) {
		t.Fatal("replayed verified assertion was treated as an independent commit")
	}
	if protocol.calls != 2 || store.calls != 2 {
		t.Fatal("replay denial did not come from the future one-use store")
	}
	// This is only a fake store, NOT a claim of real PostgreSQL atomicity.
}

func TestPasskeyActivationCommitRequiresVerifierStoreAndLiveContext(t *testing.T) {
	t.Parallel()
	committer, store, _, _, bearer := commitFixture(t)
	if _, err := NewPasskeyActivationCommitter(nil, store); !errors.Is(err, ErrPasskeyActivationCommitUnavailable) {
		t.Fatal("nil verifier accepted")
	}
	if _, err := NewPasskeyActivationCommitter(committer.proof, nil); !errors.Is(err, ErrPasskeyActivationCommitUnavailable) {
		t.Fatal("nil SQL authority accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := committer.Finish(ctx, bearer, "pav_AAAAAAAAAAAAAAAAAAAAAA", []byte("{}")); !errors.Is(err, ErrPasskeyActivationCommitUnavailable) || store.calls != 0 {
		t.Fatal("canceled operation reached commit authority")
	}
	if err := committer.Finish(context.Background(), bearer, "pav_AAAAAAAAAAAAAAAAAAAAAA", nil); !errors.Is(err, ErrPasskeyActivationCommitDenied) || store.calls != 0 {
		t.Fatal("empty assertion reached commit authority")
	}
	if err := (*PasskeyActivationCommitter)(nil).Finish(context.Background(), bearer, "", nil); !errors.Is(err, ErrPasskeyActivationCommitUnavailable) {
		t.Fatal("nil commit coordinator accepted input")
	}
}
