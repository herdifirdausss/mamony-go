package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// All references below are deterministic synthetic placeholders. They are not
// signatures, credentials, actual observations or authority evidence.
func testID(n int) string     { return fmt.Sprintf("%032x", n) }
func testDigest(n int) string { return fmt.Sprintf("%064x", n) }
func hd(p uint32) string      { return testDigest(100 + int(p)) }
func ver(n uint32) *uint32    { return &n }
func str(s string) *string    { return &s }
func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, e := raw(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func mustEnvelope(t *testing.T, p Payload) []byte {
	t.Helper()
	b, e := envelope(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func fixture(t *testing.T, n int) []byte {
	t.Helper()
	p := Payload{JournalID: testID(1), ControllerID: testID(2), StorageBindingDigest: testDigest(3), Slots: []Slot{}, Histories: []History{}, RestartReviews: []RestartReview{}}
	for i := 1; i <= n; i++ {
		b := Binding{testID(100 + i), testDigest(200 + i)}
		p.Slots = append(p.Slots, Slot{Key{testID(3), testID(10 + i), "IDENTITY"}, testID(50), b, b})
	}
	b, e := Encode(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func read(t *testing.T, b []byte) Snapshot {
	t.Helper()
	v, e := Decode(b)
	if e != nil {
		t.Fatalf("decode valid fixture: %v", e)
	}
	return v
}
func payload(t *testing.T, b []byte) Payload { t.Helper(); return read(t, b).Payload }
func state(t *testing.T, b []byte) *model {
	t.Helper()
	v := payload(t, b)
	m, e := replay(v)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func badDecode(t *testing.T, b []byte) {
	t.Helper()
	before := append([]byte(nil), b...)
	v, e := Decode(b)
	if !errors.Is(e, ErrInvalidSnapshot) || !reflect.DeepEqual(v, Snapshot{}) || !bytes.Equal(b, before) {
		t.Fatalf("invalid input not refused atomically as INVALID_SNAPSHOT: %v", e)
	}
}

// wireWithPayload preserves the exact supplied bytes and computes the documented
// integrity digest independently of the implementation codec. Invalid semantic
// inputs thus cannot be rejected merely because their digest is stale.
func wireWithPayload(p []byte) []byte {
	sum := sha256.Sum256(append([]byte("mamony-migration-journal-v1\n"), p...))
	return []byte(fmt.Sprintf(`{"format":"mamony-migration-journal-v1","payload":%s,"digest":"%x"}`, p, sum))
}
func rawPayload(t *testing.T, b []byte) []byte {
	t.Helper()
	var e struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Payload) == 0 {
		t.Fatal("missing payload")
	}
	return e.Payload
}
func replaceOnce(t *testing.T, b []byte, old, new string) []byte {
	t.Helper()
	if bytes.Count(b, []byte(old)) != 1 {
		t.Fatalf("expected exactly one token %q", old)
	}
	return bytes.Replace(b, []byte(old), []byte(new), 1)
}
func mutateSnapshot(t *testing.T, b []byte, f func(*Payload)) []byte {
	t.Helper()
	p := payload(t, b)
	f(&p)
	return mustEnvelope(t, p)
}
func roundTrip(t *testing.T, b []byte) {
	t.Helper()
	v := read(t, b)
	c, e := Encode(v.Payload)
	if e != nil || !bytes.Equal(c, b) {
		t.Fatalf("round trip: %v", e)
	}
}
func request(t *testing.T, b []byte, kind, action, opID string, scope []Key, auth string, body any) Request {
	t.Helper()
	s, e := typed[Snapshot](b)
	if e != nil {
		t.Fatal(e)
	}
	r := mustRaw(t, body)
	d, e := bodyDigest(action, r)
	if e != nil {
		t.Fatal(e)
	}
	return Request{Assertion{kind, s.Payload.JournalID, s.Payload.Revision, s.Digest, action, opID, append([]Key(nil), scope...), d, auth}, r}
}
func accept(t *testing.T, b []byte, r Request) []byte {
	t.Helper()
	before := append([]byte(nil), b...)
	rb := append([]byte(nil), r.Body...)
	assertion := r.Assertion
	assertion.Scope = append([]Key(nil), r.Assertion.Scope...)
	out, e := Apply(b, r)
	if e != nil {
		t.Fatalf("%s refused: %v", r.Assertion.Action, e)
	}
	if !bytes.Equal(before, b) || !bytes.Equal(rb, r.Body) || !reflect.DeepEqual(assertion, r.Assertion) {
		t.Fatal("input mutation")
	}
	s := read(t, out)
	if s.Payload.Revision != r.Assertion.SourceRevision+1 {
		t.Fatal("wrong revision")
	}
	return out
}
func refuse(t *testing.T, b []byte, r Request, want error) {
	t.Helper()
	before := append([]byte(nil), b...)
	body := append([]byte(nil), r.Body...)
	assertion := r.Assertion
	assertion.Scope = append([]Key(nil), r.Assertion.Scope...)
	out, e := Apply(b, r)
	if !errors.Is(e, want) || out != nil || !bytes.Equal(b, before) || !bytes.Equal(body, r.Body) || !reflect.DeepEqual(assertion, r.Assertion) {
		t.Fatalf("%s: expected atomic refusal %v, got %v", r.Assertion.Action, want, e)
	}
}
func opening(t *testing.T, b []byte, slot int, start, target uint32) MigrationOpening {
	t.Helper()
	p := payload(t, b)
	s := p.Slots[slot]
	return MigrationOpening{s.Key, s.Current, testDigest(301), testDigest(302), target, start, hd(start), testDigest(303)}
}
func openReq(t *testing.T, b []byte, opID string, x MigrationOpening) Request {
	return request(t, b, "ADMISSION", "OPEN_MIGRATION", opID, []Key{x.Slot}, x.AuthorizationDigest, migrationOpenBody{opID, x})
}
func begin(t *testing.T, b []byte, slot int, start, target uint32) ([]byte, string) {
	t.Helper()
	opID := testID(500 + int(payload(t, b).Revision))
	return accept(t, b, openReq(t, b, opID, opening(t, b, slot, start, target))), opID
}
func observation(t *testing.T, b []byte, kind, phase string, p uint32, a *uint32, effect, opEffect, cause, verification, cleanup string) MigrationObservation {
	t.Helper()
	r := payload(t, b).Revision + 1
	return MigrationObservation{r, kind, phase, a, effect, p, hd(p), opEffect, cause, verification, cleanup, testDigest(800 + int(r)), testDigest(900 + int(r))}
}
func appendReq(t *testing.T, b []byte, opID, kind string, x MigrationObservation) Request {
	m := state(t, b)
	return request(t, b, kind, "APPEND_MIGRATION", opID, []Key{m.ops[opID].migration.Slot}, x.AuthorizationDigest, migrationAppendBody{opID, x})
}
func terminalReq(t *testing.T, b []byte, opID, disposition string, x MigrationObservation) Request {
	m := state(t, b)
	action, kind := "COMPLETE_MIGRATION", "ACTIVE"
	if disposition == "STOPPED" {
		action = "STOP_MIGRATION"
	}
	if disposition == "RECONCILED" {
		action = "RECONCILE_MIGRATION"
		kind = "REVIEW"
	}
	return request(t, b, kind, action, opID, []Key{m.ops[opID].migration.Slot}, x.AuthorizationDigest, migrationTerminalBody{opID, MigrationResolution{x.Revision, disposition, x}})
}
func progress(t *testing.T, b []byte, opID string, p uint32) []byte {
	x := observation(t, b, "PROGRESS", "VERSION", p, ver(p), "COMMITTED", "MUTATED", "IN_PROGRESS", "VERIFIED", "PENDING")
	return accept(t, b, appendReq(t, b, opID, "ACTIVE", x))
}
func holdUnknown(t *testing.T, b []byte, opID string, p uint32, a *uint32) []byte {
	effect := "MUTATED"
	if p == state(t, b).ops[opID].migration.StartingPrefix {
		effect = "UNKNOWN"
	}
	phase := "VERSION"
	if a == nil {
		phase = "UNKNOWN_PHASE"
	}
	x := observation(t, b, "HOLD", phase, p, a, "UNKNOWN", effect, "UNCERTAIN", "UNVERIFIED", "UNVERIFIED")
	return accept(t, b, appendReq(t, b, opID, "ACTIVE", x))
}
func finalObs(t *testing.T, b []byte, p uint32, a *uint32, effect, opEffect, cause string, review bool) MigrationObservation {
	phase := "CLEANUP"
	if review {
		phase = "REVIEW"
	}
	return observation(t, b, "FINAL", phase, p, a, effect, opEffect, cause, "VERIFIED", "SETTLED")
}
func completed(t *testing.T, b []byte, opID string, p uint32) []byte {
	x := finalObs(t, b, p, ver(p), "COMMITTED", "MUTATED", "COMPLETE", false)
	return accept(t, b, terminalReq(t, b, opID, "COMPLETED", x))
}
func restoreOpening(t *testing.T, b []byte, parent *string) RestoreOpening {
	m := state(t, b)
	s := m.p.Slots[0]
	scope := m.scope(s.Key.EnvironmentID, s.ClusterID)
	prior := []BoundAnchor{}
	for _, k := range scope {
		prior = append(prior, BoundAnchor{k, m.p.Slots[m.byKey[k]].Current, m.anchor(k)})
	}
	return RestoreOpening{s.Key.EnvironmentID, s.ClusterID, scope, prior, testDigest(401), testDigest(402), testDigest(403), parent}
}
func restoreOpenReq(t *testing.T, b []byte, opID string, x RestoreOpening) Request {
	kind := "RESTORE"
	if x.LinkedMigrationID != nil {
		kind = "REVIEW"
	}
	return request(t, b, kind, "OPEN_RESTORE", opID, x.AffectedSlots, x.AuthorizationDigest, restoreOpenBody{opID, x})
}
func beginRestore(t *testing.T, b []byte, parent *string) ([]byte, string) {
	opID := testID(1500 + int(payload(t, b).Revision))
	return accept(t, b, restoreOpenReq(t, b, opID, restoreOpening(t, b, parent))), opID
}
func restoreObs(t *testing.T, b []byte, phase, mutation, settlement string) RestoreObservation {
	r := payload(t, b).Revision + 1
	return RestoreObservation{r, phase, mutation, settlement, testDigest(5000 + int(r)), testDigest(6000 + int(r))}
}
func restoreAppendReq(t *testing.T, b []byte, opID, kind string, x RestoreObservation) Request {
	return request(t, b, kind, "APPEND_RESTORE", opID, state(t, b).ops[opID].restore.AffectedSlots, x.AuthorizationDigest, restoreAppendBody{opID, x})
}
func restoreFinal(t *testing.T, b []byte, opID string, abort bool, p uint32) RestoreResolution {
	m := state(t, b)
	r := m.p.Revision + 1
	x := RestoreResolution{r, "REBOUND", []BoundAnchor{}, "CHANGED", "VERIFIED", "SETTLED", testDigest(7000 + int(r)), testDigest(8000 + int(r))}
	if abort {
		x.Disposition = "ABORTED_NO_MUTATION"
		x.NewBindings = nil
		x.Mutation = "NONE"
		return x
	}
	dbs := map[dbKey]string{}
	for i, k := range m.ops[opID].restore.AffectedSlots {
		d := dbOf(k)
		inc, ok := dbs[d]
		if !ok {
			inc = testID(10000 + int(r)*10 + i)
			dbs[d] = inc
		}
		x.NewBindings = append(x.NewBindings, BoundAnchor{k, Binding{inc, testDigest(700 + i)}, &Anchor{p, hd(p)}})
	}
	return x
}
func restoreTerminalReq(t *testing.T, b []byte, opID string, x RestoreResolution) Request {
	action := "REBIND_RESTORE"
	if x.Disposition == "ABORTED_NO_MUTATION" {
		action = "ABORT_RESTORE"
	}
	return request(t, b, "REVIEW", action, opID, state(t, b).ops[opID].restore.AffectedSlots, x.AuthorizationDigest, restoreTerminalBody{opID, x})
}
func pairReq(t *testing.T, b []byte, rid, mid string, rr RestoreResolution, mo MigrationObservation) Request {
	return request(t, b, "REVIEW", "PAIR_REBIND", rid, state(t, b).ops[rid].restore.AffectedSlots, rr.AuthorizationDigest, pairBody{rid, rr, mid, MigrationResolution{mo.Revision, "RECONCILED", mo}})
}
func restartReq(t *testing.T, b []byte, id string) Request {
	m := state(t, b)
	x := RestartReview{id, m.p.Revision + 1, testDigest(31), testDigest(32), testDigest(33)}
	return request(t, b, "RESTART", "RECORD_RESTART", id, m.roster(), x.AuthorizationDigest, restartBody{x})
}
func linkedFixture(t *testing.T) ([]byte, string, string) {
	b, mid := begin(t, fixture(t, 2), 0, 0, 2)
	b = holdUnknown(t, b, mid, 0, ver(1))
	b, rid := beginRestore(t, b, &mid)
	return b, mid, rid
}
func pairResolved(t *testing.T) ([]byte, string, string) {
	b, mid, rid := linkedFixture(t)
	rr := restoreFinal(t, b, rid, false, 0)
	o := finalObs(t, b, 1, ver(1), "COMMITTED", "MUTATED", "UNCERTAIN", true)
	o.AuthorizationDigest = rr.AuthorizationDigest
	return accept(t, b, pairReq(t, b, rid, mid, rr, o)), mid, rid
}

// Build legal large histories through the shared model transition engine rather
// than fabricating snapshots. Every complete value is subsequently replayed by
// Encode/Decode; no external effects or fault injection are involved.
func many(t *testing.T, n, slots int, full bool, reviews int) []byte {
	t.Helper()
	m := state(t, fixture(t, slots))
	next := func(action, id, kind string, v any) {
		t.Helper()
		r := m.p.Revision + 1
		if e := m.apply([]event{{r, action, id, kind, mustRaw(t, v)}}); e != nil {
			t.Fatalf("construct legal history %s at %d: %v", action, r, e)
		}
	}
	for j := 0; j < n; j++ {
		rid := testID(3000 + j)
		s := m.p.Slots[0]
		keys := m.roster()
		prior := []BoundAnchor{}
		for _, k := range keys {
			prior = append(prior, BoundAnchor{k, m.p.Slots[m.byKey[k]].Current, m.anchor(k)})
		}
		o := RestoreOpening{s.Key.EnvironmentID, s.ClusterID, keys, prior, testDigest(41), testDigest(42), testDigest(43), nil}
		next("OPEN_RESTORE", rid, "RESTORE", o)
		if full {
			for i, phase := range []string{"PREPARED", "OBSERVED", "OBSERVED", "UNCERTAIN"} {
				mut := "CHANGED"
				if i == 0 {
					mut = "NONE"
				}
				next("APPEND_RESTORE", rid, "RESTORE", RestoreObservation{m.p.Revision + 1, phase, mut, "SETTLED", testDigest(44), testDigest(45)})
			}
		}
		r := m.p.Revision + 1
		bindings := []BoundAnchor{}
		for i, k := range keys {
			bindings = append(bindings, BoundAnchor{k, Binding{testID(20000 + j*10 + i), testDigest(46 + i)}, &Anchor{0, hd(0)}})
		}
		next("REBIND_RESTORE", rid, "RESTORE", RestoreResolution{r, "REBOUND", bindings, "CHANGED", "VERIFIED", "SETTLED", testDigest(47), testDigest(48)})
	}
	for i := 0; i < reviews; i++ {
		id := testID(4000 + i)
		next("RECORD_RESTART", id, "", RestartReview{id, m.p.Revision + 1, testDigest(51), testDigest(52), testDigest(53)})
	}
	b, e := Encode(m.p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestSnapshotValidation(t *testing.T) {
	base := fixture(t, 1)
	open, mid := begin(t, base, 0, 0, 2)
	done := completed(t, open, mid, 2)
	pair, _, _ := pairResolved(t)
	review := accept(t, base, restartReq(t, base, testID(700)))
	t.Run("Genesis", func(t *testing.T) {
		s := read(t, base)
		if s.Payload.Revision != 0 || len(s.Payload.Histories) != 0 {
			t.Fatal("not genesis")
		}
	})
	t.Run("CanonicalRoundTrip", func(t *testing.T) { roundTrip(t, open) })
	t.Run("ResolvedHistoryRoundTrip", func(t *testing.T) { roundTrip(t, done) })
	t.Run("LinkedRestoreRoundTrip", func(t *testing.T) { roundTrip(t, pair) })
	t.Run("RestartReviewRoundTrip", func(t *testing.T) { roundTrip(t, review) })
	t.Run("DeterministicDigest", func(t *testing.T) {
		const canonicalPayload = `{"journal_id":"00000000000000000000000000000001","controller_id":"00000000000000000000000000000002","storage_binding_digest":"0000000000000000000000000000000000000000000000000000000000000003","revision":0,"slots":[{"key":{"environment_id":"00000000000000000000000000000003","database_id":"0000000000000000000000000000000b","capability":"IDENTITY"},"cluster_id":"00000000000000000000000000000032","initial_binding":{"incarnation_id":"00000000000000000000000000000065","receipt_digest":"00000000000000000000000000000000000000000000000000000000000000c9"},"current_binding":{"incarnation_id":"00000000000000000000000000000065","receipt_digest":"00000000000000000000000000000000000000000000000000000000000000c9"}}],"histories":[],"restart_reviews":[]}`
		const expectedDigest = "1318565a8df0072eb1ea0f3d55f3ed466f18a1c38fa7fd3cf581acc373e30f41"
		expected := []byte(`{"format":"mamony-migration-journal-v1","payload":` + canonicalPayload + `,"digest":"` + expectedDigest + `"}`)
		s := read(t, base)
		if !bytes.Equal(base, expected) || s.Digest != expectedDigest || !bytes.Equal(wireWithPayload([]byte(canonicalPayload)), expected) {
			t.Fatal("canonical byte vector or independently pinned digest mismatch")
		}
		roundTrip(t, base)
	})
	t.Run("RestoreAnchorReplay", func(t *testing.T) {
		m := state(t, pair)
		for _, k := range m.roster() {
			if !equalAnchor(m.anchor(k), &Anchor{0, hd(0)}) {
				t.Fatal("anchor not rebound")
			}
		}
	})
	t.Run("ReplayedHistoryValid", func(t *testing.T) {
		m := state(t, done)
		if m.blocked() || m.anchor(m.roster()[0]).Prefix != 2 {
			t.Fatal("bad replay")
		}
	})
	t.Run("EmptyOrOversize", func(t *testing.T) { badDecode(t, nil); badDecode(t, bytes.Repeat([]byte{'0'}, MaxEnvelope+1)) })
	t.Run("NonASCIIOrBOM", func(t *testing.T) {
		badDecode(t, append([]byte{0xef, 0xbb, 0xbf}, base...))
		badDecode(t, bytes.Replace(base, []byte(Format), []byte("é"), 1))
	})
	t.Run("MalformedOrTrailing", func(t *testing.T) {
		badDecode(t, base[:len(base)-1])
		badDecode(t, append(append([]byte(nil), base...), []byte("{}")...))
	})
	t.Run("DuplicateOrCaseVariantKey", func(t *testing.T) {
		for _, key := range []string{"format", "FORMAT"} {
			badDecode(t, append([]byte(fmt.Sprintf("{\"%s\":\"%s\",", key, Format)), base[1:]...))
		}
	})
	t.Run("UnknownOrMissingField", func(t *testing.T) {
		badDecode(t, append([]byte(`{"x":0,`), base[1:]...))
		badDecode(t, bytes.Replace(base, []byte(`"revision":0,`), nil, 1))
		p := rawPayload(t, base)
		badDecode(t, wireWithPayload(replaceOnce(t, p, `"revision":0,`, ``)))
		badDecode(t, wireWithPayload(replaceOnce(t, p, `"revision":0,`, `"revision":0,"extra":0,`)))
	})
	t.Run("WrongTypeOrNull", func(t *testing.T) {
		p := rawPayload(t, base)
		var v struct {
			Slots json.RawMessage `json:"slots"`
		}
		if e := json.Unmarshal(p, &v); e != nil {
			t.Fatal(e)
		}
		badDecode(t, wireWithPayload(replaceOnce(t, p, `"slots":`+string(v.Slots), `"slots":null`)))
		badDecode(t, wireWithPayload(replaceOnce(t, p, `"revision":0`, `"revision":"0"`)))
		// Retain the original combined invalid-input scenario as an additional guard.
		badDecode(t, bytes.Replace(base, []byte(`"slots":[`), []byte(`"slots":null,"extra":[`), 1))
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) { p.Histories = nil }))
	})
	t.Run("NoncanonicalOrderOrEscape", func(t *testing.T) {
		badDecode(t, bytes.Replace(base, []byte(`"format"`), []byte(`"f\u006frmat"`), 1))
		s := read(t, base)
		p := mustRaw(t, s.Payload)
		badDecode(t, []byte(fmt.Sprintf(`{"payload":%s,"format":"%s","digest":"%s"}`, p, Format, s.Digest)))
		badDecode(t, append(append([]byte(nil), base...), '\n'))
	})
	t.Run("NoncanonicalIntegerOrOverflow", func(t *testing.T) {
		for _, v := range []string{"00", "0.0", "0e0", "-0", "9007199254740992", "18446744073709551616"} {
			badDecode(t, bytes.Replace(base, []byte(`"revision":0`), []byte(`"revision":`+v), 1))
		}
	})
	t.Run("InvalidIDOrDigest", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) { p.JournalID = strings.Repeat("0", 32) }))
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) { p.StorageBindingDigest = strings.Repeat("A", 64) }))
	})
	t.Run("UnsupportedVersion", func(t *testing.T) {
		badDecode(t, bytes.Replace(base, []byte(Format), []byte("mamony-migration-journal-v2"), 1))
	})
	t.Run("DigestMismatch", func(t *testing.T) { s := read(t, base); s.Digest = testDigest(999); badDecode(t, mustRaw(t, s)) })
	t.Run("InvalidRosterOrBinding", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) { p.Slots[0].Current.IncarnationID = testID(999) }))
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) { p.Slots = append(p.Slots, p.Slots[0]) }))
	})
	t.Run("DuplicateHistoryID", func(t *testing.T) {
		b, _ := begin(t, done, 0, 2, 2)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) { p.Histories[1].ID = p.Histories[0].ID }))
	})
	t.Run("InvalidReferenceOrRevision", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, open, func(p *Payload) { p.Histories[0].OpenRevision = 2; p.Revision = 2 }))
	})
	t.Run("InvalidObservationOrResolution", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, done, func(p *Payload) {
			r, _ := typed[MigrationResolution](p.Histories[0].Resolution)
			r.Final.Cleanup = "UNVERIFIED"
			p.Histories[0].Resolution = mustRaw(t, r)
		}))
	})
	t.Run("ExcessDepthOrCount", func(t *testing.T) {
		badDecode(t, []byte(strings.Repeat("[", 18)+"0"+strings.Repeat("]", 18)))
		badDecode(t, mutateSnapshot(t, base, func(p *Payload) {
			for len(p.Slots) < 9 {
				p.Slots = append(p.Slots, p.Slots[0])
			}
		}))
	})
	t.Run("CrossSlotIncarnationMismatch", func(t *testing.T) {
		p := payload(t, fixture(t, 2))
		s := p.Slots[0]
		s.Key.Capability = "LEDGER"
		s.Initial.ReceiptDigest = testDigest(999)
		s.Current = s.Initial
		// Same logical database, distinct receipts, sorted capabilities.
		p.Slots = []Slot{p.Slots[0], s, p.Slots[1]}
		good, err := Encode(p)
		if err != nil {
			t.Fatalf("shared-incarnation positive control: %v", err)
		}
		roundTrip(t, good)
		badDecode(t, mutateSnapshot(t, good, func(p *Payload) {
			p.Slots[1].Initial.IncarnationID = testID(999)
			p.Slots[1].Current = p.Slots[1].Initial
		}))
		fenced, rid := beginRestore(t, good, nil)
		r := restoreFinal(t, fenced, rid, false, 0)
		roundTrip(t, accept(t, fenced, restoreTerminalReq(t, fenced, rid, r)))
		r.NewBindings[1].Binding.IncarnationID = testID(999)
		refuse(t, fenced, restoreTerminalReq(t, fenced, rid, r), ErrInvalidTransition)
	})
	t.Run("CrossOperationPrefixRegression", func(t *testing.T) {
		b, _ := begin(t, done, 0, 2, 3)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			x, _ := typed[MigrationOpening](p.Histories[1].Opening)
			x.StartingPrefix = 1
			x.StartingHistoryDigest = hd(1)
			p.Histories[1].Opening = mustRaw(t, x)
		}))
	})
	t.Run("CrossOperationDigestConflict", func(t *testing.T) {
		b, _ := begin(t, done, 0, 2, 3)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			x, _ := typed[MigrationOpening](p.Histories[1].Opening)
			x.StartingHistoryDigest = testDigest(999)
			p.Histories[1].Opening = mustRaw(t, x)
		}))
	})
	t.Run("PairedRevisionWrongParent", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, pair, func(p *Payload) {
			x, _ := typed[RestoreOpening](p.Histories[1].Opening)
			x.LinkedMigrationID = str(testID(999))
			p.Histories[1].Opening = mustRaw(t, x)
		}))
	})
	t.Run("ReviewIDCollision", func(t *testing.T) {
		b := accept(t, open, restartReq(t, open, testID(777)))
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) { p.RestartReviews[0].ID = p.Histories[0].ID }))
	})
	t.Run("InvalidPhaseEffectCombination", func(t *testing.T) {
		b := progress(t, open, mid, 1)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			x, _ := typed[MigrationObservation](p.Histories[0].Observations[0])
			x.Phase = "PREFLIGHT"
			p.Histories[0].Observations[0] = mustRaw(t, x)
		}))
	})
	t.Run("DepthBoundary", func(t *testing.T) {
		a := []byte(strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16))
		b := []byte(strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17))
		if !wireGuard(a, MaxEnvelope) || wireGuard(b, MaxEnvelope) {
			t.Fatal("depth guard")
		}
		badDecode(t, a)
		badDecode(t, b)
	})
	t.Run("ReplayedHistoryViolation", func(t *testing.T) {
		b, _ := begin(t, done, 0, 2, 3)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			p.Histories[0].Resolution = json.RawMessage("null")
			p.Histories[1].OpenRevision = 2
			p.Revision = 2
		}))
	})
	t.Run("NullPrefixErasesKnownFact", func(t *testing.T) {
		b := progress(t, open, mid, 1)
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			p.Histories[0].Observations[0] = bytes.Replace(p.Histories[0].Observations[0], []byte(`"verified_prefix":1`), []byte(`"verified_prefix":null`), 1)
		}))
	})
	t.Run("AnomalyTerminalRefusal", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, done, func(p *Payload) {
			x, _ := typed[MigrationResolution](p.Histories[0].Resolution)
			x.Final.OperationEffect = "ANOMALOUS"
			p.Histories[0].Resolution = mustRaw(t, x)
		}))
	})
	t.Run("RestoreAbortAfterKnownMutation", func(t *testing.T) {
		b, r := beginRestore(t, base, nil)
		b = accept(t, b, restoreAppendReq(t, b, r, "RESTORE", restoreObs(t, b, "OBSERVED", "CHANGED", "SETTLED")))
		badDecode(t, mutateSnapshot(t, b, func(p *Payload) {
			x := restoreFinal(t, b, r, true, 0)
			p.Histories[0].Resolution = mustRaw(t, x)
			p.Revision = x.Revision
		}))
	})
	t.Run("RebindAnchorMissing", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, pair, func(p *Payload) {
			x, _ := typed[RestoreResolution](p.Histories[1].Resolution)
			x.NewBindings[0].Anchor = nil
			p.Histories[1].Resolution = mustRaw(t, x)
		}))
	})
	t.Run("HistoricalIncarnationReuse", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, pair, func(p *Payload) {
			x, _ := typed[RestoreResolution](p.Histories[1].Resolution)
			x.NewBindings[0].Binding = p.Slots[0].Initial
			p.Slots[0].Current = p.Slots[0].Initial
			p.Histories[1].Resolution = mustRaw(t, x)
		}))
	})
	t.Run("HistoricalReservationViolation", func(t *testing.T) {
		b := many(t, 31, 1, false, 0)
		p := payload(t, b)
		x := opening(t, b, 0, 0, 0)
		r := p.Revision + 1
		f := MigrationObservation{r + 1, "FINAL", "CLEANUP", nil, "NOT_STARTED", 0, hd(0), "NO_MUTATION", "COMPLETE", "VERIFIED", "SETTLED", testDigest(77), testDigest(78)}
		p.Histories = append(p.Histories, History{testID(99999), r, "MIGRATION", mustRaw(t, x), []json.RawMessage{}, mustRaw(t, MigrationResolution{r + 1, "COMPLETED", f})})
		p.Revision = r + 1
		badDecode(t, mustEnvelope(t, p))
	})
}

func TestSnapshotCapacity(t *testing.T) {
	base := fixture(t, 1)
	b30 := many(t, 30, 1, false, 0)
	b31 := many(t, 31, 1, false, 0)
	b32 := many(t, 32, 1, false, 0)
	max := many(t, 32, 8, true, 8)
	open, mid := begin(t, base, 0, 0, 5)
	p3 := open
	for i := uint32(1); i <= 3; i++ {
		p3 = progress(t, p3, mid, i)
	}
	held := holdUnknown(t, p3, mid, 3, ver(4))
	linked, _, rid := linkedFixture(t)
	t.Run("ExactEnvelopeBoundary", func(t *testing.T) {
		b := bytes.Repeat([]byte{'0'}, MaxEnvelope)
		if !wireGuard(b, MaxEnvelope) {
			t.Fatal("byte ceiling wrong")
		}
		badDecode(t, b)
	})
	t.Run("EightSlots", func(t *testing.T) {
		if len(payload(t, fixture(t, 8)).Slots) != 8 {
			t.Fatal("roster")
		}
	})
	t.Run("ReserveMigrationAndLinkedRestore", func(t *testing.T) {
		if state(t, open).reservation() != 1 {
			t.Fatal("missing reservation")
		}
	})
	t.Run("ConsumeLinkedReservation", func(t *testing.T) {
		m := state(t, linked)
		if m.reservation() != 0 || len(m.p.Histories) != 2 {
			t.Fatal("double charge")
		}
	})
	t.Run("TerminalReservationAtObservationLimit", func(t *testing.T) {
		o := finalObs(t, held, 4, ver(4), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		b := accept(t, held, terminalReq(t, held, mid, "RECONCILED", o))
		if state(t, b).blocked() {
			t.Fatal("terminal reserve lost")
		}
	})
	t.Run("LastSlotStandaloneRestore", func(t *testing.T) {
		b, r := beginRestore(t, b31, nil)
		if len(payload(t, b).Histories) != 32 {
			t.Fatal("final slot")
		}
		accept(t, b, restoreTerminalReq(t, b, r, restoreFinal(t, b, r, true, 0)))
	})
	t.Run("EightRestartReviews", func(t *testing.T) {
		if len(payload(t, max).RestartReviews) != 8 {
			t.Fatal("review limit")
		}
	})
	t.Run("ThirtyClosedMigrationReserve", func(t *testing.T) {
		b, _ := begin(t, b30, 0, 0, 1)
		m := state(t, b)
		if len(m.p.Histories)+m.reservation() != 32 {
			t.Fatal("reservation")
		}
	})
	t.Run("ThirtyOneClosedStandaloneRestore", func(t *testing.T) {
		b, _ := beginRestore(t, b31, nil)
		if len(payload(t, b).Histories) != 32 {
			t.Fatal("last history")
		}
	})
	t.Run("EmergencyObservationReservation", func(t *testing.T) {
		m := state(t, held)
		if m.ops[mid].ordinary != 3 || !m.ops[mid].hold || len(m.p.Histories[0].Observations) != 4 {
			t.Fatal("hold slot")
		}
	})
	t.Run("ResolutionAfterObservationExhaustion", func(t *testing.T) {
		o := finalObs(t, held, 3, ver(4), "ROLLED_BACK", "MUTATED", "CANCELED", true)
		accept(t, held, terminalReq(t, held, mid, "RECONCILED", o))
	})
	t.Run("ReachableRevisionBound", func(t *testing.T) {
		if payload(t, max).Revision != 200 {
			t.Fatal("reachable bound")
		}
		roundTrip(t, max)
	})
	t.Run("RawEnvelopeSizeGuard", func(t *testing.T) {
		if wireGuard(bytes.Repeat([]byte{'0'}, MaxEnvelope+1), MaxEnvelope) {
			t.Fatal("guard accepted excess")
		}
	})
	t.Run("AccountingGuardOperands", func(t *testing.T) {
		if accounting(32, 0, 32*MaxHistory, 0, MaxOverhead, 32*MaxHistory+MaxOverhead) != nil {
			t.Fatal("bounded operand rejected")
		}
		if historySize(MaxHistory) != nil {
			t.Fatal("size boundary")
		}
	})
	t.Run("RevisedStructuralBound", func(t *testing.T) {
		p := payload(t, max)
		if len(max) > 492000 {
			t.Fatal("envelope bound")
		}
		sum := 0
		for _, h := range p.Histories {
			n := len(mustRaw(t, h))
			if n > 15000 {
				t.Fatal("record bound")
			}
			sum += n
		}
		if len(max)-sum > 12000 {
			t.Fatal("overhead bound")
		}
	})
	t.Run("LinkedAbortConsumesReservation", func(t *testing.T) {
		b := accept(t, linked, restoreTerminalReq(t, linked, rid, restoreFinal(t, linked, rid, true, 0)))
		m := state(t, b)
		if !m.blocked() || m.reservation() != 0 {
			t.Fatal("parent or reservation reset")
		}
	})
	t.Run("NinthSlot", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, fixture(t, 8), func(p *Payload) {
			s := p.Slots[7]
			s.Key.DatabaseID = testID(999)
			s.Initial.IncarnationID = testID(999)
			s.Current = s.Initial
			p.Slots = append(p.Slots, s)
		}))
	})
	t.Run("HistoryOver16KiB", func(t *testing.T) {
		if historySize(MaxHistory+1) == nil {
			t.Fatal("size guard")
		}
	})
	t.Run("OverheadOver64KiB", func(t *testing.T) {
		if accounting(0, 0, 0, 0, MaxOverhead+1, MaxOverhead+1) == nil {
			t.Fatal("overhead guard")
		}
	})
	t.Run("EnvelopeOver1MiB", func(t *testing.T) { badDecode(t, bytes.Repeat([]byte{'0'}, MaxEnvelope+1)) })
	t.Run("MigrationWithOneSlotLeft", func(t *testing.T) {
		refuse(t, b31, openReq(t, b31, testID(999), opening(t, b31, 0, 0, 1)), ErrCapacity)
	})
	t.Run("ThirtyThirdHistory", func(t *testing.T) {
		refuse(t, b32, restoreOpenReq(t, b32, testID(999), restoreOpening(t, b32, nil)), ErrCapacity)
	})
	t.Run("FifthObservation", func(t *testing.T) {
		o := observation(t, held, "HOLD", "VERSION", 3, ver(4), "UNKNOWN", "MUTATED", "UNCERTAIN", "UNVERIFIED", "UNVERIFIED")
		refuse(t, held, appendReq(t, held, mid, "REVIEW", o), ErrCapacity)
	})
	t.Run("NinthRestartReview", func(t *testing.T) { refuse(t, max, restartReq(t, max, testID(999)), ErrCapacity) })
	t.Run("ReservationOverflowOrUndercount", func(t *testing.T) {
		for _, v := range [][]int{{32, 1, 0, 1, 0, 0}, {1, 1, 0, 0, 0, 0}, {1, 0, MaxHistory + 1, 0, 0, 0}, {1, 0, 0, 1, 0, MaxHistory + 1}, {1, -1, 0, 0, 0, 0}, {int(^uint(0) >> 1), 1, 0, 0, 0, 0}} {
			if accounting(v[0], v[1], v[2], v[3], v[4], v[5]) == nil {
				t.Fatal("invalid accounting accepted")
			}
		}
	})
	t.Run("ThirtyOneClosedMigrationRefusal", func(t *testing.T) {
		r := openReq(t, b31, testID(991), opening(t, b31, 0, 0, 1))
		refuse(t, b31, r, ErrCapacity)
	})
	t.Run("ThirtyTwoClosedRefusal", func(t *testing.T) {
		refuse(t, b32, openReq(t, b32, testID(991), opening(t, b32, 0, 0, 1)), ErrCapacity)
		refuse(t, b32, restoreOpenReq(t, b32, testID(992), restoreOpening(t, b32, nil)), ErrCapacity)
	})
	t.Run("FourthOrdinaryObservation", func(t *testing.T) {
		o := observation(t, p3, "PROGRESS", "VERSION", 4, ver(4), "COMMITTED", "MUTATED", "IN_PROGRESS", "VERIFIED", "PENDING")
		refuse(t, p3, appendReq(t, p3, mid, "ACTIVE", o), ErrCapacity)
	})
	t.Run("RevisionAboveReachableBound", func(t *testing.T) { badDecode(t, mutateSnapshot(t, max, func(p *Payload) { p.Revision = 201 })) })
	t.Run("SecondLinkedReservationAfterAbort", func(t *testing.T) {
		b := accept(t, linked, restoreTerminalReq(t, linked, rid, restoreFinal(t, linked, rid, true, 0)))
		parent := state(t, b).migration
		refuse(t, b, restoreOpenReq(t, b, testID(998), restoreOpening(t, b, &parent)), ErrInvalidTransition)
	})
}
