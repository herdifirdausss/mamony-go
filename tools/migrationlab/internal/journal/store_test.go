package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// In-memory scripted ports only. Neither supplies real authority or performs I/O.
type storageScript struct {
	p                expectedProvision
	session, custody string
	data, stageBytes []byte
	journal, stage   storageObject
	serial           uint64
	trace            []string
	faults           map[string]phaseResult
	acquireResult    *custodyResult
	createResult     *stageResult
	mutate           func(string, *storageObservation)
	mutateRead       func(string, *storageRead)
	replaceOnError   bool
	replaceSource    storeSourceRef
}

func (a *storageScript) call(n string) { a.trace = append(a.trace, n) }
func (a *storageScript) meta(o storageObject, k string, mode uint32, size uint64) storageMeta {
	return storageMeta{Object: o, MountID: a.p.Mount.ID, Kind: k, Mode: mode, UID: a.p.UID, GID: a.p.GID, Links: 1, Size: size, FDInfo: []byte(fmt.Sprintf("pos:\t0\nflags:\t0100000\nmnt_id:\t%d\nino:\t%d\n", a.p.Mount.ID, o.Inode)), FDComplete: true}
}
func (a *storageScript) observation(n string, held bool, serial uint64) storageObservation {
	p := a.p
	o := storageObservation{BootID: p.BootID, Namespace: p.Namespace, UID: p.UID, GID: p.GID, MountInfo: []byte("41 1 8:7 / /control rw - ext4 /dev/synthetic rw,data=ordered,barrier=1\n"), MountComplete: true, FSMagic: 0xef53, Directory: a.meta(p.Directory, "directory", 0700, 0), Lock: a.meta(p.Lock, "regular", 0600, 0), Journal: a.meta(a.journal, "regular", 0600, uint64(len(a.data))), PathDirectory: p.Directory, PathLock: p.Lock, PathDirectoryMount: p.Mount.ID, PathLockMount: p.Mount.ID, Entries: []string{storeLockName, storeSourceName}, Ref: observationRef{a.session, serial, a.journal}}
	o.Directory.Links = 2
	if held {
		o.Custody = a.custody
	}
	if validObject(a.stage) {
		o.Entries = append(o.Entries, storeStageName)
		o.Stage = a.meta(a.stage, "regular", 0600, uint64(len(a.stageBytes)))
	}
	if a.mutate != nil {
		a.mutate(n, &o)
	}
	return o
}
func (a *storageScript) inspectProvision(_ context.Context, _ expectedProvision) (storageObservation, adapterStatus) {
	a.call("provision")
	return a.observation("provision", false, 0), adapterOK
}
func (a *storageScript) acquire(_ context.Context, _ string) custodyResult {
	a.call("acquire")
	if a.acquireResult != nil {
		return *a.acquireResult
	}
	return custodyResult{adapterOK, a.custody, true, false}
}
func (a *storageScript) inspect(n string) (storageObservation, adapterStatus) {
	a.call(n)
	a.serial++
	return a.observation(n, true, a.serial), adapterOK
}
func (a *storageScript) inspectSource(_ context.Context, _ string) (storageObservation, adapterStatus) {
	return a.inspect("inspect")
}
func (a *storageScript) inspectPublished(_ context.Context, _ string) (storageObservation, adapterStatus) {
	return a.inspect("inspectPublished")
}
func (a *storageScript) read(n string, ref observationRef, limit int) (storageRead, adapterStatus) {
	a.call(n)
	if limit != MaxEnvelope+1 || ref.Session != a.session || ref.Serial != a.serial || ref.Object != a.journal {
		return storageRead{}, adapterUnknown
	}
	b := a.data
	if len(b) > limit {
		b = b[:limit]
	}
	r := storageRead{ref, bytes.Clone(b), len(a.data) <= limit, a.observation(n, true, a.serial)}
	if a.mutateRead != nil {
		a.mutateRead(n, &r)
	}
	return r, adapterOK
}
func (a *storageScript) readSource(_ context.Context, _ string, r observationRef, l int) (storageRead, adapterStatus) {
	return a.read("read", r, l)
}
func (a *storageScript) readPublished(_ context.Context, _ string, r observationRef, l int) (storageRead, adapterStatus) {
	return a.read("readPublished", r, l)
}
func (a *storageScript) createStage(_ context.Context, _ string) stageResult {
	a.call("create")
	if a.createResult != nil {
		return *a.createResult
	}
	a.stage = storageObject{a.p.Mount.Device, a.journal.Inode + 1}
	a.stageBytes = nil
	return stageResult{Status: adapterOK, Stage: a.meta(a.stage, "regular", 0600, 0), Owned: true}
}
func (a *storageScript) effect(n string, o storageObject, count uint64) phaseResult {
	a.call(n)
	if v, ok := a.faults[n]; ok {
		if v.Object == (storageObject{}) {
			v.Object = o
		}
		return v
	}
	return phaseResult{adapterOK, true, o, count}
}
func (a *storageScript) writeStage(_ context.Context, _ string, o storageObject, b []byte) phaseResult {
	r := a.effect("write", o, uint64(len(b)))
	if r.Count <= uint64(len(b)) {
		a.stageBytes = bytes.Clone(b[:r.Count])
	}
	return r
}
func (a *storageScript) syncStage(_ context.Context, _ string, o storageObject) phaseResult {
	return a.effect("sync", o, 0)
}
func (a *storageScript) closeStage(_ context.Context, _ string, o storageObject) phaseResult {
	return a.effect("closeStage", o, 0)
}
func (a *storageScript) replace(_ context.Context, _ string, o storageObject, ref storeSourceRef) phaseResult {
	a.replaceSource = ref
	r := a.effect("replace", o, 0)
	if r.Status == adapterOK || a.replaceOnError {
		a.journal = o
		a.data = bytes.Clone(a.stageBytes)
		a.stage = storageObject{}
		a.stageBytes = nil
	}
	return r
}
func (a *storageScript) syncDirectory(_ context.Context, _ string) phaseResult {
	return a.effect("dirsync", a.p.Directory, 0)
}
func (a *storageScript) removeOwnedStage(_ context.Context, _ string, o storageObject) phaseResult {
	r := a.effect("remove", o, 0)
	if r.Status == adapterOK {
		a.stage = storageObject{}
		a.stageBytes = nil
	}
	return r
}
func (a *storageScript) retire(_ context.Context, _ string) phaseResult {
	return a.effect("retire", a.p.Lock, 0)
}

type evidenceScript struct {
	expected          *gateClaim
	issued            gateGrant
	count             int
	worker, operation string
	status            gateStatus
	mutate            func(*gateGrant)
	checkpoint        func(gateFacts) gateStatus
	phases            []storePhase
	facts             []gateFacts
	lastFacts         gateFacts
}

func (g *evidenceScript) authorize(_ context.Context, c gateClaim) (gateGrant, gateStatus) {
	g.count++
	if g.expected == nil || !reflect.DeepEqual(*g.expected, c) {
		return gateGrant{}, gateRefused
	}
	want := *g.expected
	g.expected = nil
	if g.status != gateAllowed {
		return gateGrant{}, g.status
	}
	out := gateGrant{testID(1000 + g.count), cloneClaim(want), g.worker, g.operation}
	if g.mutate != nil {
		g.mutate(&out)
	}
	g.issued = cloneGrant(out)
	return out, gateAllowed
}
func (g *evidenceScript) confirm(_ context.Context, grant gateGrant, f gateFacts) gateStatus {
	g.phases = append(g.phases, f.Phase)
	g.lastFacts = f
	g.facts = append(g.facts, f)
	if !reflect.DeepEqual(grant, g.issued) {
		return gateUnknown
	}
	if g.checkpoint != nil {
		return g.checkpoint(f)
	}
	return gateAllowed
}

type storeHarness struct {
	t        *testing.T
	a        *storageScript
	g        *evidenceScript
	s        *storageSession
	expected []byte
	c        currentnessReference
	p        expectedProvision
}

func harness(t *testing.T, b []byte) *storeHarness {
	t.Helper()
	snap := read(t, b)
	d := storageDevice{8, 7}
	p := expectedProvision{EnvironmentID: snap.Payload.Slots[0].Key.EnvironmentID, ReceiptID: testID(81), ReceiptDigest: testDigest(82), JournalID: snap.Payload.JournalID, ControllerID: snap.Payload.ControllerID, BindingDigest: snap.Payload.StorageBindingDigest, HostVolumeDigest: testDigest(83), BootID: "11111111-1111-1111-1111-111111111111", Namespace: storageObject{storageDevice{0, 4}, 4}, Directory: storageObject{d, 10}, Lock: storageObject{d, 11}, UID: 1001, GID: 1001, ControlPath: "/control/journal", Mount: storageMount{41, d, "/", "/control", "ext4"}, PolicyDigest: testDigest(84), ProvenanceDigest: testDigest(85)}
	c := currentnessReference{testID(86), p.JournalID, p.BindingDigest, testID(87), sourceRef(snap), testDigest(88), testDigest(89), 180000}
	return &storeHarness{t: t, p: p, c: c, expected: bytes.Clone(b), a: &storageScript{p: p, session: c.SessionID, custody: testID(90), data: bytes.Clone(b), journal: storageObject{d, 12}, faults: map[string]phaseResult{}}, g: &evidenceScript{worker: "SETTLED", status: gateAllowed}}
}
func (h *storeHarness) open() error {
	h.g.expected = &gateClaim{Purpose: "OPEN", Provision: h.p, Current: h.c}
	s, e := openStore(context.Background(), h.p, h.c, h.a, h.g)
	h.s = s
	return e
}
func (h *storeHarness) mustOpen() {
	h.t.Helper()
	if e := h.open(); e != nil || h.s.state != storeReviewRequired {
		h.t.Fatalf("open %v", e)
	}
	h.a.trace = nil
}
func (h *storeHarness) evidence(r Request) storageEvidence {
	e := storageEvidence{Current: h.c, Assertion: cloneAssertion(r.Assertion), ContinuityDigest: testDigest(91), SettlementDigest: testDigest(92), AuthorizationDigest: r.Assertion.AuthorizationDigest, EvidenceDigest: testDigest(93)}
	if r.Assertion.Action == "RECORD_RESTART" {
		v, err := typed[restartBody](r.Body)
		if err == nil {
			e.ContinuityDigest = v.Review.ContinuityDigest
			e.SettlementDigest = v.Review.SettlementDigest
		}
	}
	return e
}
func (h *storeHarness) prepare(r Request, e storageEvidence, restart bool) {
	purpose := "ACTION"
	if restart {
		purpose = "REVIEW"
	}
	h.g.expected = &gateClaim{Purpose: purpose, Provision: h.p, Current: e.Current, Evidence: cloneEvidence(e), Custody: h.a.custody, PriorState: h.s.state}
}
func (h *storeHarness) transition(r Request, e storageEvidence, restart bool) (publicationResult, error) {
	h.t.Helper()
	h.prepare(r, e, restart)
	h.a.trace = nil
	h.g.phases = nil
	h.g.facts = nil
	old := bytes.Clone(r.Body)
	a := cloneAssertion(r.Assertion)
	ev := cloneEvidence(e)
	var out publicationResult
	var err error
	if restart {
		out, err = h.s.recordRestart(context.Background(), r, reviewEvidence{e})
	} else {
		out, err = h.s.applyTransition(context.Background(), r, actionEvidence{e})
	}
	if !bytes.Equal(r.Body, old) || !reflect.DeepEqual(r.Assertion, a) || !reflect.DeepEqual(e, ev) {
		h.t.Fatal("mutated caller input")
	}
	return out, err
}
func (h *storeHarness) reviewRequest() Request {
	return restartReq(h.t, h.expected, testID(400+h.g.count))
}
func (h *storeHarness) migrationRequest() Request {
	return openReq(h.t, h.expected, testID(500+h.g.count), opening(h.t, h.expected, 0, 0, 1))
}
func (h *storeHarness) verifyPublished(r Request, out publicationResult, err error) {
	h.t.Helper()
	if err != nil || out.disposition != storePublished || out.phase != phaseDone || out.sourceRef != h.c.Source || out.publishedRef.Revision != h.c.Source.Revision+1 {
		h.t.Fatalf("publication %+v %v trace %v", out, err, h.a.trace)
	}
	want, e := Apply(h.expected, r)
	if e != nil || !bytes.Equal(want, h.a.data) || sourceRef(read(h.t, h.a.data)) != out.publishedRef {
		h.t.Fatal("wrong actual candidate bytes")
	}
	// Every successful transition must receive all checkpoints in order.
	phases := []storePhase{phaseValidate, phaseValidate, phaseValidate, phaseApply, phaseStageCreate, phaseStageWrite, phaseFileSync, phaseStageClose, phasePreReplace, phasePreReplace, phaseReplace, phaseDirectorySync, phaseReadback, phaseReadback, phaseFinal}
	if !slices.Equal(h.g.phases, phases) {
		h.t.Fatalf("gate phases %v want %v", h.g.phases, phases)
	}
	for i, f := range h.g.facts {
		if f.Source != h.c.Source || f.Cleanup {
			h.t.Fatalf("checkpoint source/cleanup %d: %+v", i, f)
		}
		if i < 4 {
			if f.Candidate != (storeSourceRef{}) || validObject(f.Stage) {
				h.t.Fatalf("premature candidate %d", i)
			}
		} else if f.Candidate != out.publishedRef {
			h.t.Fatalf("candidate binding %d", i)
		}
		if i >= 5 && i <= 11 || i == 14 {
			if f.Stage != h.a.journal {
				h.t.Fatalf("stage binding %d", i)
			}
		}
	}
	if !slices.Equal(h.a.trace, []string{"inspect", "read", "create", "write", "sync", "closeStage", "inspect", "read", "replace", "dirsync", "inspectPublished", "readPublished"}) {
		h.t.Fatal("publication trace", h.a.trace)
	}
	h.expected = bytes.Clone(want)
	h.c.Source = out.publishedRef
}
func (h *storeHarness) mustReview() {
	h.t.Helper()
	r := h.reviewRequest()
	out, e := h.transition(r, h.evidence(r), true)
	h.verifyPublished(r, out, e)
	if h.s.state != storeReviewed || len(payload(h.t, h.expected).RestartReviews) == 0 {
		h.t.Fatal("missing review")
	}
	h.a.trace = nil
}
func (h *storeHarness) closeEvidence() closeEvidence {
	c := h.c
	c.Source = h.s.anchor
	return closeEvidence{c, testDigest(94), testDigest(95)}
}
func (h *storeHarness) retire(e closeEvidence) error {
	h.g.expected = &gateClaim{Purpose: "CLOSE", Provision: h.p, Current: e.Current, Close: e, Custody: h.a.custody, PriorState: h.s.state, Uncertainty: h.s.uncertainty}
	return h.s.close(context.Background(), e)
}
func (h *storeHarness) noCalls(names ...string) {
	h.t.Helper()
	for _, n := range names {
		if slices.Contains(h.a.trace, n) {
			h.t.Fatalf("forbidden %s in %v", n, h.a.trace)
		}
	}
}
func (h *storeHarness) trace(want ...string) {
	h.t.Helper()
	if !slices.Equal(h.a.trace, want) {
		h.t.Fatalf("trace got %v want %v", h.a.trace, want)
	}
}
func (h *storeHarness) want(out publicationResult, e error, d storeDisposition, cause error, p storePhase) {
	h.t.Helper()
	if out.disposition != d || out.phase != p || !errors.Is(e, cause) || out.publishedRef != (storeSourceRef{}) {
		h.t.Fatalf("got %+v %v want %v %v %v trace=%v", out, e, d, cause, p, h.a.trace)
	}
	if out.sourceRef != h.c.Source {
		h.t.Fatal("last known source mismatch", out.sourceRef, h.c.Source)
	}
	if d == storeUncertain && h.s.state != storePoisoned {
		h.t.Fatal("not poisoned")
	}
	if d == storeNotPublished && (!bytes.Equal(h.expected, h.a.data) || h.s.anchor != h.c.Source || h.s.state == storePoisoned) {
		h.t.Fatal("old bytes changed")
	}
}
func (h *storeHarness) action() (publicationResult, error) {
	r := h.migrationRequest()
	return h.transition(r, h.evidence(r), false)
}
func ready(t *testing.T) *storeHarness {
	h := harness(t, fixture(t, 2))
	h.mustOpen()
	h.mustReview()
	return h
}
func openInvalid(t *testing.T, mut func(string, *storageObservation), cause error) {
	t.Helper()
	h := harness(t, fixture(t, 1))
	h.a.mutate = mut
	e := h.open()
	if e == nil {
		t.Fatal("invalid qualification accepted")
	}
	if h.s != nil {
		var f *storageFailure
		if !errors.As(e, &f) || !errors.Is(f.cause, cause) || h.s.state != storePoisoned {
			t.Fatalf("unexpected %v %+v", e, h.s)
		}
	} else if !errors.Is(e, cause) {
		t.Fatalf("wrong refusal %v", e)
	}
	h.noCalls("read", "readPublished", "create", "replace")
}
func assertFailureCause(t *testing.T, e, errorCause error) {
	t.Helper()
	var f *storageFailure
	if !errors.As(e, &f) || !errors.Is(f.cause, errorCause) {
		t.Fatalf("unexpected cause %v", e)
	}
}

func TestJournalPublication(t *testing.T) {
	t.Run("ValidatedReplacement", func(t *testing.T) {
		h := ready(t)
		r := h.migrationRequest()
		before := payload(t, h.expected)
		x, _ := typed[migrationOpenBody](r.Body)
		out, e := h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, out, e)
		after := payload(t, h.a.data)
		if len(after.Histories) != 1 || after.Histories[0].ID != x.ID || !bytes.Equal(after.Histories[0].Opening, mustRaw(t, x.Opening)) || !reflect.DeepEqual(after.RestartReviews, before.RestartReviews) {
			t.Fatal("independent expected facts mismatch")
		}
		h.trace("inspect", "read", "create", "write", "sync", "closeStage", "inspect", "read", "replace", "dirsync", "inspectPublished", "readPublished")
		if h.a.replaceSource != out.sourceRef || h.g.lastFacts.Phase != phaseFinal || h.g.lastFacts.Candidate != out.publishedRef || h.g.lastFacts.Source != out.sourceRef || h.g.lastFacts.Stage != h.a.journal {
			t.Fatal("phase/source binding mismatch")
		}
		if h.a.journal.Inode != 14 || validObject(h.a.stage) {
			t.Fatal("wrong replacement identity")
		}
		expected := before
		expected.Revision++
		expected.Histories = append(expected.Histories, History{ID: x.ID, OpenRevision: expected.Revision, Kind: "MIGRATION", Opening: mustRaw(t, x.Opening), Observations: []json.RawMessage{}, Resolution: json.RawMessage("null")})
		rawExpected, err := json.Marshal(expected)
		if err != nil || !bytes.Equal(wireWithPayload(rawExpected), h.a.data) {
			t.Fatal("independent candidate oracle mismatch")
		}
		h.a.data[0] = '!'
		if h.s.sourceBytes[0] == '!' {
			t.Fatal("alias")
		}
	})
	t.Run("ExactSourceBinding", func(t *testing.T) {
		h := ready(t)
		r := h.migrationRequest()
		r.Assertion.SourceRevision--
		o, e := h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, ErrStaleSource, phaseApply)
		h.noCalls("create")
		h = ready(t)
		h.a.data = fixture(t, 2)
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("create")
		h = ready(t)
		h.g.mutate = func(g *gateGrant) { g.Claim.Current.SessionID = testID(999) }
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
		h = ready(t)
		h.s.gate = nil
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
		h = ready(t)
		h.g.mutate = func(g *gateGrant) { g.ID = h.s.lastGrant }
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
		h = ready(t)
		h.g.mutate = func(g *gateGrant) { g.Claim.Provision.ReceiptDigest = testDigest(999) }
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
		h = ready(t)
		h.g.mutate = func(g *gateGrant) { g.Claim.Current.BudgetDeadline++ }
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()

		for _, change := range []func(*gateGrant){
			func(g *gateGrant) { g.Claim.Current.HistoryEvidence = testDigest(999) },
			func(g *gateGrant) { g.Claim.Evidence.Assertion.BodyDigest = testDigest(999) },
			func(g *gateGrant) { g.Claim.Evidence.Assertion.OperationID = testID(999) },
			func(g *gateGrant) { g.Claim.Evidence.Assertion.Scope[0].DatabaseID = testID(999) },
			func(g *gateGrant) { g.Claim.Evidence.SettlementDigest = testDigest(999) },
			func(g *gateGrant) { g.Claim.Evidence.AuthorizationDigest = testDigest(999) },
		} {
			h = ready(t)
			h.g.mutate = change
			o, e = h.action()
			if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) || h.s.state != storeReviewed {
				t.Fatal(o, e)
			}
			h.trace()
		}
	})
	t.Run("PureRefusalNoWrite", func(t *testing.T) {
		for _, v := range []struct {
			mut  func(*Request)
			want error
		}{{func(r *Request) { r.Assertion.SourceRevision-- }, ErrStaleSource}, {func(r *Request) { r.Assertion.BodyDigest = testDigest(998) }, ErrInvalidRequest}, {func(r *Request) { r.Assertion.Scope[0].DatabaseID = testID(998) }, ErrAuthorityScope}, {func(r *Request) { r.Assertion.Kind = "RESTORE" }, ErrAuthorityScope}} {
			h := ready(t)
			r := h.migrationRequest()
			v.mut(&r)
			o, e := h.transition(r, h.evidence(r), false)
			h.want(o, e, storeNotPublished, v.want, phaseApply)
			h.noCalls("create")
		}
		h := ready(t)
		x := opening(t, h.expected, 0, 0, 1)
		x.Binding.ReceiptDigest = testDigest(998)
		r := openReq(t, h.expected, testID(997), x)
		o, e := h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, ErrInvalidTransition, phaseApply)
		h = ready(t)
		h.a.data = []byte("bad")
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		assertFailureCause(t, e, ErrInvalidSnapshot)
		h.noCalls("create")
		h = harness(t, many(t, 31, 1, false, 0))
		h.mustOpen()
		h.mustReview()
		o, e = h.action()
		h.want(o, e, storeNotPublished, ErrCapacity, phaseApply)
		h.noCalls("create")
		h = ready(t)
		r = h.migrationRequest()
		r.Assertion.SourceRevision--
		r.Assertion.BodyDigest = testDigest(999)
		o, e = h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, ErrStaleSource, phaseApply)
	})
	t.Run("BoundedReadSentinel", func(t *testing.T) {
		for _, n := range []int{MaxEnvelope, MaxEnvelope + 1} {
			h := ready(t)
			r := h.migrationRequest()
			ev := h.evidence(r)
			h.prepare(r, ev, false)
			g, e := h.s.grant(context.Background(), *h.g.expected)
			if e != nil {
				t.Fatal(e)
			}
			h.a.mutate = func(_ string, o *storageObservation) { o.Journal.Size = MaxEnvelope }
			h.a.mutateRead = func(_ string, r *storageRead) { r.Bytes = make([]byte, n) }
			data, _, e := h.s.readQualified(context.Background(), g, phaseValidate, storageObject{}, h.s.journal, false, false)
			if n == MaxEnvelope {
				if e != nil || len(data) != MaxEnvelope {
					t.Fatal("raw exact boundary not reached", e)
				}
			} else if !errors.Is(e, ErrInvalidSnapshot) || data != nil {
				t.Fatal("sentinel refusal", e)
			}
			h.noCalls("create", "replace")
		}

		for _, n := range []int{MaxEnvelope, MaxEnvelope + 1} {
			h := ready(t)
			h.a.mutate = func(_ string, o *storageObservation) { o.Journal.Size = MaxEnvelope }
			h.a.mutateRead = func(_ string, r *storageRead) { r.Bytes = make([]byte, n) }
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
			h.noCalls("create")
			if !slices.Contains(h.a.trace, "read") {
				t.Fatal("read guard not reached")
			}
		}
		h := ready(t)
		h.a.mutateRead = func(_ string, r *storageRead) { r.Complete = false }
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h = ready(t)
		r := h.migrationRequest()
		r.Body = make([]byte, MaxEnvelope+1)
		o, e = h.transition(r, h.evidence(r), false)
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreRequest) {
			t.Fatal(o, e)
		}
		h.trace()
	})
	t.Run("MissingJournal", func(t *testing.T) {
		openInvalid(t, func(n string, o *storageObservation) {
			if n == "inspect" {
				o.Entries = []string{storeLockName}
				o.Journal = storageMeta{}
			}
		}, errStoreUnavailable)
	})
	t.Run("CorruptJournal", func(t *testing.T) {
		for _, semantic := range []bool{true, false} {
			h := ready(t)
			p := rawPayload(t, h.expected)
			bad := wireWithPayload(replaceOnce(t, p, `"revision":1,"slots":`, `"revision":200,"slots":`))
			if !semantic {
				bad = bytes.Clone(h.expected)
				bad[len(bad)-3] = '0'
				if bytes.Equal(bad, h.expected) {
					bad[len(bad)-3] = '1'
				}
			}
			h.a.data = bad
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
			assertFailureCause(t, e, ErrInvalidSnapshot)
			h.trace("inspect", "read")
		}
	})
	t.Run("OversizeJournal", func(t *testing.T) {
		for _, n := range []uint64{MaxEnvelope + 1, ^uint64(0)} {
			openInvalid(t, func(k string, o *storageObservation) {
				if k == "inspect" {
					o.Journal.Size = n
				}
			}, ErrInvalidSnapshot)
		}
	})
	t.Run("BindingMismatch", func(t *testing.T) {
		for _, change := range []func(*Payload){func(p *Payload) { p.JournalID = testID(999) }, func(p *Payload) { p.ControllerID = testID(999) }, func(p *Payload) { p.StorageBindingDigest = testDigest(999) }} {
			h := ready(t)
			p := payload(t, h.expected)
			change(&p)
			h.a.data = mustEnvelope(t, p)
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
			assertFailureCause(t, e, errStoreIdentity)
			h.noCalls("create")
		}
	})
	t.Run("UnsupportedMount", func(t *testing.T) {
		for _, m := range []string{"xfs", "overlay", "nfs", "tmpfs"} {
			openInvalid(t, func(_ string, o *storageObservation) {
				o.MountInfo = bytes.Replace(o.MountInfo, []byte(" ext4 "), []byte(" "+m+" "), 1)
			}, errStoreUnsupported)
		}
		for _, opt := range []string{"ro,data=ordered", "rw,data=writeback", "rw,barrier=0", "rw,nobarrier", "rw=,data=ordered", "rw=1,data=ordered", "rw,data", "rw,data=", "rw,barrier", "rw,barrier=", "rw,ro", "rw,noload"} {
			openInvalid(t, func(_ string, o *storageObservation) {
				o.MountInfo = bytes.Replace(o.MountInfo, []byte("rw,data=ordered,barrier=1"), []byte(opt), 1)
			}, errStoreUnsupported)
		}
		h := ready(t)
		h.p.ProvenanceDigest = ""
		if e := h.open(); !errors.Is(e, errStoreRequest) {
			t.Fatal(e)
		}
	})
	t.Run("InvalidMountObservation", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		p := h.p.Mount
		valid := []byte("41 1 8:7 / /control rw - ext4 /dev/synthetic rw,data=ordered,barrier=1\n")
		if e := matchMount(valid, true, p); e != nil {
			t.Fatal(e)
		}
		for _, x := range [][]byte{bytes.Repeat([]byte{'x'}, mountInfoLimit+1), bytes.Repeat([]byte{'x'}, 16385), append(bytes.Clone(valid), valid...), bytes.Replace(valid, []byte("41 "), []byte("18446744073709551616 "), 1), bytes.Replace(valid, []byte("/control"), []byte(`/bad\777`), 1), bytes.Replace(valid, []byte(" - "), []byte(" shared:1 - "), 1), bytes.Replace(valid, []byte("8:7"), []byte("8:8"), 1), bytes.TrimSuffix(valid, []byte("\n"))} {
			if e := matchMount(x, true, p); e == nil {
				t.Fatal("invalid mount accepted")
			}
		}
		if _, ok := fdMount([]byte("mnt_id: 41\nmnt_id: 41\n"), true); ok {
			t.Fatal("duplicate")
		}
		if _, ok := fdMount([]byte("mnt_id: 41\n"), false); ok {
			t.Fatal("incomplete")
		}
		if e := matchMount(append(bytes.Clone(valid), []byte("42 1 0:1 / /other rw - tmpfs tmpfs rw\n")...), true, p); e != nil {
			t.Fatal(e)
		}
		openInvalid(t, func(n string, o *storageObservation) {
			if n == "inspect" {
				o.Journal.FDInfo = []byte("mnt_id: 42\n")
			}
		}, errStoreIdentity)
		for _, v := range []struct {
			old, new string
			cause    error
		}{
			{" / /control ", " /wrong /control ", errStoreIdentity},
			{" / /control ", " / relative ", errStoreIdentity},
			{" - ", " future:1 - ", errStoreUnsupported},
			{" rw - ", " rw= - ", errStoreUnsupported},
			{"data=ordered", "=ordered", errStoreIdentity},
			{" ext4 ", " ext4= ", errStoreIdentity},
		} {
			if e := matchMount(bytes.Replace(valid, []byte(v.old), []byte(v.new), 1), true, p); !errors.Is(e, v.cause) {
				t.Fatal("mount mismatch", v, e)
			}
		}
		if e := matchMount(append(bytes.Clone(valid), []byte("42 1 0:1 / /other rw future:1 - tmpfs tmpfs rw,unknown=,other=a=b\n")...), true, p); e != nil {
			t.Fatal("unrelated options disqualified target", e)
		}
		fdExact := []byte("mnt_id: 41\n")
		for len(fdExact) < fdInfoLimit {
			n := min(1024, fdInfoLimit-len(fdExact)-1)
			fdExact = append(fdExact, []byte("x:"+strings.Repeat("a", n-2)+"\n")...)
		}
		if len(fdExact) != fdInfoLimit {
			t.Fatal("fdinfo fixture size")
		}
		if n, ok := fdMount(fdExact, true); !ok || n != 41 {
			t.Fatal("fdinfo exact bound")
		}
		if _, ok := fdMount(append(bytes.Clone(fdExact), '\n'), true); ok {
			t.Fatal("fdinfo bound+1")
		}
		fdLines := append([]byte("mnt_id: 41\n"), bytes.Repeat([]byte("x: a\n"), 63)...)
		if _, ok := fdMount(fdLines, true); !ok {
			t.Fatal("64 fdinfo lines")
		}
		if _, ok := fdMount(append(fdLines, []byte("x: a\n")...), true); ok {
			t.Fatal("65 fdinfo lines")
		}
		if _, ok := fdMount([]byte("pos: 0\n"), true); ok {
			t.Fatal("missing required mount ID")
		}
		if _, ok := fdMount(append([]byte("mnt_id: 41\n"), []byte("x:"+strings.Repeat("a", 1023)+"\n")...), true); ok {
			t.Fatal("fdinfo line over bound")
		}
		full := bytes.Repeat(append(bytes.Repeat([]byte{'x'}, 16383), '\n'), 64)
		if len(full) != mountInfoLimit {
			t.Fatal("fixture bound")
		}
		if _, ok := boundedLines(full, true, mountInfoLimit, 4096, 16384); !ok {
			t.Fatal("exact mountinfo byte bound")
		}
		if _, ok := boundedLines(append(full, '\n'), true, mountInfoLimit, 4096, 16384); ok {
			t.Fatal("mountinfo over bound")
		}
		var records bytes.Buffer
		for i := 0; i < 4095; i++ {
			fmt.Fprintf(&records, "%d 1 0:1 / /other rw - tmpfs tmpfs rw\n", 100+i)
		}
		maxRecords := append(bytes.Clone(valid), records.Bytes()...)
		if e := matchMount(maxRecords, true, p); e != nil {
			t.Fatal("max records", e)
		}
		if e := matchMount(append(maxRecords, []byte("6000 1 0:1 / /other rw - tmpfs tmpfs rw\n")...), true, p); e == nil {
			t.Fatal("record bound")
		}
		if v, ok := mountEscape(`/one\040two\011three\012four\134five`); !ok || v != "/one two\tthree\nfour\\five" {
			t.Fatal("escape parser", v)
		}
		for _, x := range [][]byte{append([]byte("mnt_id: 41\n"), bytes.Repeat([]byte("x: 1\n"), 64)...), append([]byte("mnt_id: 41\n"), bytes.Repeat([]byte{'x'}, fdInfoLimit)...), []byte("mnt_id: 41\nino: 3\nino: 4\n")} {
			if _, ok := fdMount(x, true); ok {
				t.Fatal("invalid fdinfo accepted")
			}
		}

		openInvalid(t, func(n string, o *storageObservation) {
			if n == "inspect" {
				o.Journal.FDInfo = []byte("mnt_id: 41\nino: 99\n")
			}
		}, errStoreIdentity)

	})
	t.Run("WrongOwnerModeOrType", func(t *testing.T) {
		for _, f := range []func(*storageMeta){func(m *storageMeta) { m.Kind = "fifo" }, func(m *storageMeta) { m.Kind = "device" }, func(m *storageMeta) { m.UID++ }, func(m *storageMeta) { m.GID++ }, func(m *storageMeta) { m.Mode = 0644 }, func(m *storageMeta) { m.Links = 2 }} {
			openInvalid(t, func(n string, o *storageObservation) {
				if n == "inspect" {
					f(&o.Journal)
				}
			}, errStoreIdentity)
		}
		h := ready(t)
		if h.s.state != storeReviewed {
			t.Fatal("positive directory multi-link refused")
		}
	})
	t.Run("LinkOrReplacedIdentity", func(t *testing.T) {
		for _, f := range []func(*storageObservation){func(o *storageObservation) { o.PathDirectory.Inode++ }, func(o *storageObservation) { o.Lock.Object.Inode++ }, func(o *storageObservation) { o.Journal.MountID++ }, func(o *storageObservation) { o.Journal.Links = 2 }, func(o *storageObservation) { o.Ref.Session = testID(999) }} {
			openInvalid(t, func(n string, o *storageObservation) {
				if n == "inspect" {
					f(o)
				}
			}, errStoreIdentity)
		}
		h := ready(t)
		r := h.migrationRequest()
		o, e := h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		h = ready(t)
		h.a.journal.Inode++
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("read", "create")
		h = ready(t)
		h.a.mutate = func(n string, o *storageObservation) {
			if n == "inspect" {
				o.PathLockMount++
			}
		}
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("read", "create")
		h = ready(t)
		h.a.mutate = func(n string, o *storageObservation) {
			if n == "inspect" {
				o.Ref.Serial = h.s.serial
			}
		}
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("read")

	})
	t.Run("Contention", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		h.a.acquireResult = &custodyResult{Status: adapterBusy, Settled: true}
		if e := h.open(); !errors.Is(e, errStoreBusy) || h.s != nil {
			t.Fatal(e)
		}
		h.noCalls("read", "create", "retire")
		h = ready(t)
		h.s.mu.Lock()
		o, e := h.action()
		h.s.mu.Unlock()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreBusy) {
			t.Fatal(o, e)
		}
		h.trace()
		h = harness(t, fixture(t, 1))
		h.a.acquireResult = &custodyResult{Status: adapterUnknown, MayHold: true, Custody: h.a.custody}
		if e = h.open(); !errors.Is(e, errStoreUncertain) || h.s == nil || h.s.state != storePoisoned {
			t.Fatal(e)
		}
		if e = h.retire(h.closeEvidence()); e != nil {
			t.Fatal(e)
		}
		h.trace("provision", "acquire", "retire")
		h = harness(t, fixture(t, 1))
		h.a.acquireResult = &custodyResult{Status: adapterUnknown, MayHold: true}
		if e = h.open(); !errors.Is(e, errStoreUncertain) || h.s == nil || h.s.state != storePoisoned || !h.s.uncertainty.CustodyUnknown || !h.s.uncertainty.AcquisitionUncertain {
			t.Fatal("unbound custody not retained", e)
		}
		if e = h.retire(h.closeEvidence()); !errors.Is(e, errStoreAuthority) {
			t.Fatal("unbound custody retirement", e)
		}
		h.trace("provision", "acquire")
	})
	t.Run("ExistingStageRefuses", func(t *testing.T) {
		h := ready(t)
		h.a.stage = storageObject{h.p.Mount.Device, 99}
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("read", "remove", "replace")
		h = ready(t)
		h.a.createResult = &stageResult{Status: adapterLocalFailure, Absent: true}
		o, e = h.action()
		h.want(o, e, storeNotPublished, errStoreIO, phaseStageCreate)
		h.noCalls("remove", "replace")
		h = ready(t)
		h.a.createResult = &stageResult{Status: adapterUnknown}
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseStageCreate)
		h.noCalls("remove", "replace")
		h = ready(t)
		reported := storageObject{h.p.Mount.Device, 99}
		h.a.createResult = &stageResult{Status: adapterUnknown, Owned: true, Stage: h.a.meta(reported, "regular", 0600, 0)}
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseStageCreate)
		if h.s.uncertainty.Stage != reported || !h.s.uncertainty.Candidate.valid() || !h.s.uncertainty.StageClaimedOwned || h.s.uncertainty.StageQualified {
			t.Fatal("known diagnostic facts discarded")
		}
		h.noCalls("remove", "replace", "write")
	})
	for _, v := range []struct {
		name, op string
		phase    storePhase
		short    bool
	}{{"ShortWriteUnchanged", "write", phaseStageWrite, true}, {"WriteFailureUnchanged", "write", phaseStageWrite, false}, {"FileSyncFailureUnchanged", "sync", phaseFileSync, false}, {"CloseFailureUnchanged", "closeStage", phaseStageClose, false}} {
		t.Run(v.name, func(t *testing.T) {
			for _, settled := range []bool{true, false} {
				h := ready(t)
				st := adapterLocalFailure
				if v.short {
					st = adapterOK
				}
				h.a.faults[v.op] = phaseResult{Status: st, Settled: settled, Count: 1}
				o, e := h.action()
				if settled {
					h.want(o, e, storeNotPublished, errStoreIO, v.phase)
					if validObject(h.a.stage) || !slices.Contains(h.a.trace, "remove") || !slices.Contains(h.a.trace, "dirsync") {
						t.Fatal("cleanup absent")
					}
				} else {
					h.want(o, e, storeUncertain, errStoreUncertain, v.phase)
					h.noCalls("remove")
				}
				h.noCalls("replace")
				count := 0
				for _, n := range h.a.trace {
					if n == "closeStage" {
						count++
					}
				}
				if count > 1 || settled && count != 1 {
					t.Fatal("close count", count)
				}
			}
		})
	}
	t.Run("PreReplaceCleanupUncertain", func(t *testing.T) {
		for _, step := range []string{"remove", "dirsync"} {
			h := ready(t)
			h.a.faults["write"] = phaseResult{Status: adapterLocalFailure, Settled: true}
			h.a.faults[step] = phaseResult{Status: adapterUnknown}
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseStageWrite)
			var f *storageFailure
			if !errors.As(e, &f) || f.cleanup == nil || f.cause != errStoreIO {
				t.Fatal("missing causes")
			}
			h.noCalls("replace")
		}
		for _, kind := range []string{"identity", "bytes", "authority"} {
			h := ready(t)
			h.a.faults["write"] = phaseResult{Status: adapterLocalFailure, Settled: true}
			if kind == "identity" {
				h.a.mutate = func(n string, o *storageObservation) {
					if n == "inspect" && validObject(h.a.stage) {
						o.PathLock.Inode++
					}
				}
			}
			if kind == "bytes" {
				h.a.mutateRead = func(n string, r *storageRead) {
					if n == "read" && validObject(h.a.stage) {
						r.Bytes[0] = '!'
					}
				}
			}
			if kind == "authority" {
				h.g.checkpoint = func(f gateFacts) gateStatus {
					if f.Cleanup {
						return gateUnknown
					}
					return gateAllowed
				}
			}
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseStageWrite)
			assertFailureCause(t, e, errStoreIO)
			var f *storageFailure
			wantCleanup := errStoreIdentity
			if kind == "authority" {
				wantCleanup = errStoreGateUnknown
			}
			if !errors.As(e, &f) || !errors.Is(f.cleanup, wantCleanup) {
				t.Fatal("missing cleanup cause", e)
			}
			h.noCalls("replace", "remove")
		}
	})
	t.Run("RenameUncertain", func(t *testing.T) {
		for _, visible := range []bool{true, false} {
			h := ready(t)
			h.a.faults["replace"] = phaseResult{Status: adapterUnknown}
			h.a.replaceOnError = visible
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseReplace)
			h.noCalls("remove", "readPublished", "dirsync")
			if !h.s.uncertainty.ReplaceAttempted || !h.s.uncertainty.StageQualified || !h.s.uncertainty.StageClaimedOwned || !h.s.uncertainty.Candidate.valid() || h.s.uncertainty.Candidate.Revision != h.c.Source.Revision+1 || !validObject(h.s.uncertainty.Stage) {
				t.Fatal("missing known uncertainty facts")
			}
			if h.s.uncertainty.Candidate != h.s.pendingCandidate || o.publishedRef.valid() {
				t.Fatal("diagnostic confused with published ref")
			}

		}
	})
	t.Run("DirectorySyncUncertain", func(t *testing.T) {
		h := ready(t)
		h.a.faults["dirsync"] = phaseResult{Status: adapterUnknown}
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseDirectorySync)
		if bytes.Equal(h.expected, h.a.data) {
			t.Fatal("candidate not visible")
		}
		h.noCalls("remove", "readPublished")
	})
	t.Run("ReadbackMismatch", func(t *testing.T) {
		for _, meta := range []bool{true, false} {
			h := ready(t)
			if meta {
				h.a.mutate = func(n string, o *storageObservation) {
					if n == "inspectPublished" {
						o.Journal.Mode = 0644
					}
				}
			} else {
				h.a.mutateRead = func(n string, r *storageRead) {
					if n == "readPublished" {
						r.Bytes[0] = '!'
					}
				}
			}
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseReadback)
			h.noCalls("remove")
			if meta {
				h.noCalls("readPublished")
			} else if !slices.Contains(h.a.trace, "readPublished") {
				t.Fatal("no readback")
			}
		}
		h := ready(t)
		h.a.mutateRead = func(n string, r *storageRead) {
			if n == "readPublished" {
				r.Ref.Serial++
			}
		}
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseReadback)
		h.noCalls("remove")

		for _, change := range []func(*storageRead){func(r *storageRead) { r.After.Journal.Object.Inode++ }, func(r *storageRead) { r.Complete = false }, func(r *storageRead) { r.After.Journal.Size++ }} {
			h = ready(t)
			h.a.mutateRead = func(n string, r *storageRead) {
				if n == "readPublished" {
					change(r)
				}
			}
			o, e = h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, phaseReadback)
			h.noCalls("remove")
			if !slices.Contains(h.a.trace, "readPublished") {
				t.Fatal("readback test did not reach read")
			}
		}
	})
	t.Run("AuthorityLossOrBudgetExpiry", func(t *testing.T) {
		for _, v := range []struct {
			p      storePhase
			status gateStatus
			d      storeDisposition
			cause  error
		}{{phaseApply, gateExpired, storeNotPublished, errStoreBudget}, {phaseStageWrite, gateExpired, storeNotPublished, errStoreBudget}, {phasePreReplace, gateUnknown, storeUncertain, errStoreUncertain}, {phaseDirectorySync, gateExpired, storeUncertain, errStoreUncertain}, {phaseFinal, gateUnknown, storeUncertain, errStoreUncertain}} {
			h := ready(t)
			h.g.checkpoint = func(f gateFacts) gateStatus {
				if f.Phase == v.p && !f.Cleanup {
					return v.status
				}
				return gateAllowed
			}
			o, e := h.action()
			h.want(o, e, v.d, v.cause, v.p)
			if v.d == storeNotPublished {
				h.noCalls("replace")
			}
		}
		// Independently specify the expected prefix at every confirmation boundary.
		checkpoints := []storePhase{phaseValidate, phaseValidate, phaseValidate, phaseApply, phaseStageCreate, phaseStageWrite, phaseFileSync, phaseStageClose, phasePreReplace, phasePreReplace, phaseReplace, phaseDirectorySync, phaseReadback, phaseReadback, phaseFinal}
		calls := []string{"inspect", "read", "create", "write", "sync", "closeStage", "inspect", "read", "replace", "dirsync", "inspectPublished", "readPublished"}
		prefixes := []int{0, 1, 2, 2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		for i, p := range checkpoints {
			h := ready(t)
			h.g.checkpoint = func(gateFacts) gateStatus {
				if len(h.g.facts) == i+1 {
					return gateUnknown
				}
				return gateAllowed
			}
			o, e := h.action()
			h.want(o, e, storeUncertain, errStoreUncertain, p)
			assertFailureCause(t, e, errStoreGateUnknown)
			h.trace(calls[:prefixes[i]]...)
			if !slices.Equal(h.g.phases, checkpoints[:i+1]) {
				t.Fatal("unexpected confirmation prefix", h.g.phases)
			}
		}
		for _, p := range []storePhase{phaseValidate, phaseApply} {
			for _, cleanup := range []gateStatus{gateAllowed, gateUnknown, gateExpired} {
				h := ready(t)
				// Expire after the source read, not at preliminary metadata confirmation.
				h.g.checkpoint = func(f gateFacts) gateStatus {
					if f.Cleanup {
						return cleanup
					}
					if f.Phase == p && slices.Contains(h.a.trace, "read") {
						return gateExpired
					}
					return gateAllowed
				}
				o, e := h.action()
				if cleanup == gateAllowed {
					h.want(o, e, storeNotPublished, errStoreBudget, p)
				} else {
					h.want(o, e, storeUncertain, errStoreUncertain, p)
					assertFailureCause(t, e, errStoreBudget)
				}
				if !slices.Contains(h.g.phases, phaseCleanup) {
					t.Fatal("no cleanup confirmation")
				}
				h.trace("inspect", "read")
			}
		}
		h := ready(t)
		r := h.migrationRequest()
		o, e := h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		published := o
		h.g.status = gateRefused
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) || published.disposition != storePublished {
			t.Fatal(o, e)
		}
		h.trace()
		h = ready(t)
		h.g.status = gateUnknown
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.trace()
		h = ready(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r = h.migrationRequest()
		ev := h.evidence(r)
		h.prepare(r, ev, false)
		h.g.checkpoint = func(f gateFacts) gateStatus {
			if f.Phase == phaseStageWrite && !f.Cleanup {
				cancel()
			}
			return gateAllowed
		}
		o, e = h.s.applyTransition(ctx, r, actionEvidence{ev})
		h.want(o, e, storeNotPublished, errStoreBudget, phaseStageWrite)
		h.noCalls("write", "replace")

	})
	t.Run("PoisonedSessionNoFurtherWrite", func(t *testing.T) {
		h := ready(t)
		h.a.faults["replace"] = phaseResult{Status: adapterUnknown}
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseReplace)
		h.a.trace = nil
		r := h.migrationRequest()
		if _, e = h.s.inspect(context.Background(), h.evidence(r)); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreUncertain) {
			t.Fatal(o, e)
		}
		if _, e = h.s.recordRestart(context.Background(), r, reviewEvidence{h.evidence(r)}); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		if e = h.s.close(context.Background(), closeEvidence{}); !errors.Is(e, errStoreAuthority) {
			t.Fatal(e)
		}
		h.trace()
		if e = h.retire(h.closeEvidence()); e != nil {
			t.Fatal(e)
		}
		h.trace("retire")
		h = ready(t)
		h.a.faults["replace"] = phaseResult{Status: adapterUnknown}
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseReplace)
		prior := h.s.uncertainty
		h.a.trace = nil
		h.g.checkpoint = func(gateFacts) gateStatus { return gateUnknown }
		if e = h.retire(h.closeEvidence()); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		if h.g.issued.Claim.Uncertainty != prior || h.s.uncertainty.Candidate != prior.Candidate || h.s.uncertainty.Stage != prior.Stage || h.s.uncertainty.Cause != prior.Cause || h.s.uncertainty.RetirementCause != errStoreGateUnknown.Error() {
			t.Fatal("retirement lost original facts")
		}
		h.trace()
		h.g.checkpoint = nil
		h.a.faults["retire"] = phaseResult{Status: adapterUnknown}
		if e = h.retire(h.closeEvidence()); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		if h.s.uncertainty.Candidate != prior.Candidate || h.s.uncertainty.Stage != prior.Stage || h.s.uncertainty.RetirementCause != errStoreIO.Error() {
			t.Fatal("retirement failure lost facts")
		}
		h.trace("retire")
	})
}

func TestJournalRestartReview(t *testing.T) {
	t.Run("NewSessionRequiresReview", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		h.mustOpen()
		o, e := h.action()
		h.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
		h.noCalls("create")
		h.mustReview()
		if e = h.retire(h.closeEvidence()); e != nil {
			t.Fatal(e)
		}
		fresh := harness(t, h.expected)
		fresh.c.SessionID = testID(887)
		fresh.a.session = fresh.c.SessionID
		fresh.a.custody = testID(888)
		fresh.a.journal = h.a.journal
		fresh.mustOpen()
		o, e = fresh.action()
		fresh.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
	})
	t.Run("VisibleTerminalRequiresReview", func(t *testing.T) {
		b, mid := begin(t, fixture(t, 1), 0, 0, 1)
		b = completed(t, b, mid, 1)
		h := harness(t, b)
		h.mustOpen()
		o, e := h.action()
		h.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
		h.noCalls("create")
		h.mustReview()
		if state(t, h.expected).blocked() {
			t.Fatal("terminal history changed")
		}
	})
	t.Run("GenesisRequiresReview", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		h.mustOpen()
		o, e := h.action()
		h.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
		h.mustReview()
		p := payload(t, h.expected)
		if p.Revision != 1 || len(p.RestartReviews) != 1 || len(p.Histories) != 0 {
			t.Fatal("genesis review")
		}
		h = harness(t, fixture(t, 1))
		h.g.status = gateRefused
		if e = h.open(); !errors.Is(e, errStoreAuthority) || h.s != nil {
			t.Fatal(e)
		}
		h.trace()
	})
	t.Run("MissingExternalEvidence", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		s, e := openStore(context.Background(), h.p, h.c, h.a, nil)
		if s != nil || !errors.Is(e, errStoreAuthority) {
			t.Fatal(e)
		}
		h.trace()
		h.mustOpen()
		r := h.reviewRequest()
		v := h.evidence(r)
		v.SettlementDigest = ""
		o, e := h.transition(r, v, true)
		h.want(o, e, storeNotPublished, errStoreAuthority, phaseValidate)
		h.noCalls("create")
	})
	t.Run("StaleReviewAssertion", func(t *testing.T) {
		for _, change := range []func(*storageEvidence){func(e *storageEvidence) { e.Assertion.BodyDigest = testDigest(999) }, func(e *storageEvidence) { e.Assertion.SourceDigest = testDigest(999) }, func(e *storageEvidence) { e.Assertion.SourceRevision++ }, func(e *storageEvidence) { e.Assertion.OperationID = testID(999) }, func(e *storageEvidence) { e.Assertion.Action = "OPEN_MIGRATION" }, func(e *storageEvidence) { e.Assertion.Scope[0].DatabaseID = testID(999) }, func(e *storageEvidence) { e.AuthorizationDigest = testDigest(999) }, func(e *storageEvidence) { e.ContinuityDigest = testDigest(999) }, func(e *storageEvidence) { e.SettlementDigest = testDigest(999) }} {
			h := harness(t, fixture(t, 1))
			h.mustOpen()
			r := h.reviewRequest()
			v := h.evidence(r)
			change(&v)
			o, e := h.transition(r, v, true)
			h.want(o, e, storeNotPublished, errStoreAuthority, phaseValidate)
			h.noCalls("create")
		}
		h := harness(t, fixture(t, 1))
		h.mustOpen()
		r := h.reviewRequest()
		v := h.evidence(r)
		v.Current.SessionID = testID(999)
		o, e := h.transition(r, v, true)
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
	})
	t.Run("DurableReviewAllowsConsideration", func(t *testing.T) {
		h := ready(t)
		r := h.reviewRequest()
		o, e := h.transition(r, h.evidence(r), true)
		h.want(o, e, storeNotPublished, errStoreRequest, phaseValidate)
		h.noCalls("create")
		h.g.status = gateRefused
		o, e = h.action()
		if o.disposition != storeUnassessed || !errors.Is(e, errStoreAuthority) {
			t.Fatal(o, e)
		}
		h.trace()
	})
	t.Run("ReviewKeepsUnresolvedFence", func(t *testing.T) {
		b, mid := begin(t, fixture(t, 2), 0, 0, 1)
		h := harness(t, b)
		h.mustOpen()
		h.mustReview()
		if !state(t, h.expected).blocked() {
			t.Fatal("lost blocker")
		}
		for i := 0; i < 2; i++ {
			r := openReq(t, h.expected, testID(999), opening(t, h.expected, i, 0, 0))
			o, e := h.transition(r, h.evidence(r), false)
			h.want(o, e, storeNotPublished, ErrInvalidTransition, phaseApply)
			h.noCalls("create")
		}
		x := finalObs(t, h.expected, 0, nil, "NOT_STARTED", "NO_MUTATION", "INTERRUPTED", true)
		r := terminalReq(t, h.expected, mid, "RECONCILED", x)
		o, e := h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if state(t, h.expected).blocked() {
			t.Fatal("reviewed resolution missing")
		}
		b, mid, rid := linkedFixture(t)
		h = harness(t, b)
		h.mustOpen()
		h.mustReview()
		h.g.worker, h.g.operation = "UNKNOWN", mid
		rr := restoreFinal(t, h.expected, rid, false, 0)
		x = finalObs(t, h.expected, 1, ver(1), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		x.AuthorizationDigest = rr.AuthorizationDigest
		r = pairReq(t, h.expected, rid, mid, rr, x)
		o, e = h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, errStoreAuthority, phaseApply)
		h.noCalls("create", "replace")
		h.g.worker, h.g.operation = "SETTLED", mid
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if h.s.reviewOnly || state(t, h.expected).blocked() {
			t.Fatal("paired restriction not resolved")
		}
		b, rid = beginRestore(t, fixture(t, 1), nil)
		h = harness(t, b)
		h.mustOpen()
		h.mustReview()
		h.g.worker, h.g.operation = "UNKNOWN", rid
		r = restoreTerminalReq(t, h.expected, rid, restoreFinal(t, h.expected, rid, true, 0))
		o, e = h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, errStoreAuthority, phaseApply)
		h.noCalls("create", "replace")
		h.g.worker, h.g.operation = "SETTLED", rid
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if h.s.reviewOnly || state(t, h.expected).blocked() {
			t.Fatal("standalone restore restriction not resolved")
		}
		b, mid, rid = linkedFixture(t)
		h = harness(t, b)
		h.mustOpen()
		h.mustReview()
		h.g.worker, h.g.operation = "UNKNOWN", rid
		r = restoreTerminalReq(t, h.expected, rid, restoreFinal(t, h.expected, rid, true, 0))
		o, e = h.transition(r, h.evidence(r), false)
		h.want(o, e, storeNotPublished, errStoreAuthority, phaseApply)
		h.g.worker, h.g.operation = "SETTLED", rid
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if !h.s.reviewOnly || !state(t, h.expected).blocked() {
			t.Fatal("linked abort cleared parent restriction")
		}
		h.g.operation = mid
		x = finalObs(t, h.expected, 0, ver(1), "ROLLED_BACK", "NO_MUTATION", "UNCERTAIN", true)
		r = terminalReq(t, h.expected, mid, "RECONCILED", x)
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if h.s.reviewOnly || state(t, h.expected).blocked() {
			t.Fatal("settled final parent review did not release restriction")
		}
	})
	t.Run("ReviewPublicationUncertain", func(t *testing.T) {
		for _, uncertain := range []bool{false, true} {
			h := harness(t, fixture(t, 1))
			h.mustOpen()
			r := h.reviewRequest()
			if uncertain {
				h.a.faults["replace"] = phaseResult{Status: adapterUnknown}
				h.a.replaceOnError = true
			} else {
				h.a.faults["write"] = phaseResult{Status: adapterLocalFailure, Settled: true}
			}
			o, e := h.transition(r, h.evidence(r), true)
			if uncertain {
				h.want(o, e, storeUncertain, errStoreUncertain, phaseReplace)
			} else {
				h.want(o, e, storeNotPublished, errStoreIO, phaseStageWrite)
				if h.s.state != storeReviewRequired {
					t.Fatal("false reviewed")
				}
				delete(h.a.faults, "write")
				h.mustReview()
			}
		}
	})
	t.Run("ExhaustedReviewCapacity", func(t *testing.T) {
		h := harness(t, many(t, 0, 1, false, 8))
		h.mustOpen()
		r := h.reviewRequest()
		o, e := h.transition(r, h.evidence(r), true)
		h.want(o, e, storeNotPublished, ErrCapacity, phaseApply)
		h.noCalls("create")
		if h.s.state != storeReviewRequired {
			t.Fatal("review capacity cleared")
		}
	})
	t.Run("WorkerRestartRetainsVerifiedContinuity", func(t *testing.T) {
		h := ready(t)
		r := h.migrationRequest()
		e := h.evidence(r)
		h.g.expected = &gateClaim{Purpose: "INSPECT", Provision: h.p, Current: e.Current, Evidence: e, Custody: h.a.custody, PriorState: h.s.state}
		if _, x := h.s.inspect(context.Background(), e); x != nil {
			t.Fatal(x)
		}
		if h.s.state != storeReviewed || h.s.anchor != h.c.Source || len(payload(t, h.expected).RestartReviews) != 1 {
			t.Fatal("continuity lost")
		}
		h.noCalls("create", "retire")
	})
	t.Run("UnknownWorkerEffectHolds", func(t *testing.T) {
		h := ready(t)
		r := h.migrationRequest()
		o, e := h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		mid := r.Assertion.OperationID
		h.g.worker = "UNKNOWN"
		h.g.operation = mid
		o, e = h.action()
		h.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
		h.noCalls("create")
		h.g.worker = "SETTLED"
		h.g.operation = ""
		o, e = h.action()
		h.want(o, e, storeNotPublished, errStoreReview, phaseValidate)
		h.noCalls("create")
		x := finalObs(t, h.expected, 0, nil, "NOT_STARTED", "NO_MUTATION", "INTERRUPTED", true)
		r = terminalReq(t, h.expected, mid, "RECONCILED", x)
		for _, v := range []struct{ effect, operation string }{{"UNKNOWN", mid}, {"SETTLED", ""}, {"SETTLED", testID(998)}} {
			h.g.worker, h.g.operation = v.effect, v.operation
			o, e = h.transition(r, h.evidence(r), false)
			h.want(o, e, storeNotPublished, errStoreAuthority, phaseApply)
			h.trace("inspect", "read")
			if !h.s.reviewOnly || !state(t, h.expected).blocked() {
				t.Fatal("unresolved worker lost")
			}
		}
		h.g.worker, h.g.operation = "SETTLED", mid
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if h.s.reviewOnly || state(t, h.expected).blocked() {
			t.Fatal("review restriction not resolved")
		}
		h = ready(t)
		r = h.migrationRequest()
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		mid = r.Assertion.OperationID
		h.g.worker, h.g.operation = "UNKNOWN", mid
		hold := observation(t, h.expected, "HOLD", "UNKNOWN_PHASE", 0, nil, "UNKNOWN", "UNKNOWN", "UNCERTAIN", "UNVERIFIED", "UNVERIFIED")
		r = appendReq(t, h.expected, mid, "REVIEW", hold)
		o, e = h.transition(r, h.evidence(r), false)
		h.verifyPublished(r, o, e)
		if !h.s.reviewOnly || !state(t, h.expected).blocked() {
			t.Fatal("reviewed nonterminal hold cleared restriction")
		}
		h = ready(t)
		h.g.worker = "UNKNOWN"
		h.g.operation = testID(999)
		o, e = h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
	})
	t.Run("LostContinuityRetiresSession", func(t *testing.T) {
		h := ready(t)
		h.a.mutate = func(_ string, o *storageObservation) { o.PathLock.Inode++ }
		o, e := h.action()
		h.want(o, e, storeUncertain, errStoreUncertain, phaseValidate)
		h.noCalls("read", "create", "retire")
		h.a.trace = nil
		if e = h.retire(h.closeEvidence()); e != nil {
			t.Fatal(e)
		}
		h.trace("retire")
		fresh := harness(t, h.expected)
		fresh.c.SessionID = testID(887)
		fresh.a.session = fresh.c.SessionID
		fresh.a.custody = testID(888)
		fresh.a.journal = h.a.journal
		fresh.mustOpen()
		if fresh.s.state != storeReviewRequired {
			t.Fatal("fresh review bypass")
		}
	})
	t.Run("OrphanNeverAdopted", func(t *testing.T) {
		h := harness(t, fixture(t, 1))
		h.a.stage = storageObject{h.p.Mount.Device, 99}
		r := restartReq(t, h.expected, testID(998))
		h.a.stageBytes = accept(t, h.expected, r)
		e := h.open()
		if !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		h.noCalls("read", "remove", "replace")
	})
	t.Run("ClosedSessionRefuses", func(t *testing.T) {
		for _, reviewed := range []bool{true, false} {
			h := harness(t, fixture(t, 1))
			h.mustOpen()
			if reviewed {
				h.mustReview()
			}
			if e := h.retire(h.closeEvidence()); e != nil {
				t.Fatal(e)
			}
			o, e := h.action()
			if o.disposition != storeUnassessed || !errors.Is(e, errStoreClosed) {
				t.Fatal(o, e)
			}
			h.trace()
			if e = h.s.close(context.Background(), h.closeEvidence()); !errors.Is(e, errStoreClosed) {
				t.Fatal(e)
			}
		}
		h0 := ready(t)
		r0 := h0.migrationRequest()
		o0, e0 := h0.transition(r0, h0.evidence(r0), false)
		h0.verifyPublished(r0, o0, e0)
		h0.a.trace = nil
		h0.g.worker, h0.g.operation = "UNKNOWN", r0.Assertion.OperationID
		if e := h0.retire(h0.closeEvidence()); !errors.Is(e, errStoreAuthority) || !h0.s.reviewOnly {
			t.Fatal("unknown retirement effect lost restriction", e)
		}
		h0.trace()
		h0.g.worker, h0.g.operation = "SETTLED", ""
		o0, e0 = h0.action()
		h0.want(o0, e0, storeNotPublished, errStoreReview, phaseValidate)
		h0.noCalls("create", "retire")
		for _, atConfirm := range []bool{false, true} {
			for _, status := range []gateStatus{gateUnknown, gateRefused, gateExpired} {
				h := ready(t)
				if atConfirm {
					h.g.checkpoint = func(gateFacts) gateStatus { return status }
				} else {
					h.g.status = status
				}
				e := h.retire(h.closeEvidence())
				h.trace()
				if status == gateUnknown {
					if !errors.Is(e, errStoreUncertain) || h.s.state != storePoisoned {
						t.Fatal("retirement continuity loss", e)
					}
					assertFailureCause(t, e, errStoreGateUnknown)
					o, e := h.action()
					if o.disposition != storeUnassessed || !errors.Is(e, errStoreUncertain) {
						t.Fatal(o, e)
					}
					h.trace()
				} else {
					want := errStoreAuthority
					if status == gateExpired {
						want = errStoreBudget
					}
					if !errors.Is(e, want) || h.s.state != storeReviewed {
						t.Fatal("ordinary retirement refusal", e)
					}
				}
				h.g.status = gateAllowed
				h.g.checkpoint = nil
				if e = h.retire(h.closeEvidence()); e != nil {
					t.Fatal("authorized settlement", e)
				}
				h.trace("retire")
			}
		}
		h := ready(t)
		h.a.faults["retire"] = phaseResult{Status: adapterUnknown}
		if e := h.retire(h.closeEvidence()); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		if e := h.retire(h.closeEvidence()); !errors.Is(e, errStoreUncertain) {
			t.Fatal(e)
		}
		h.trace("retire")
	})
}
