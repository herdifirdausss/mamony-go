package journal

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJournalTransitions(t *testing.T) {
	base := fixture(t, 2)
	opened, mid := begin(t, base, 0, 0, 5)
	committed := progress(t, opened, mid, 2)
	held := holdUnknown(t, committed, mid, 2, ver(3))
	closed := completed(t, opened, mid, 5)
	linked, lmid, rid := linkedFixture(t)
	paired, _, _ := pairResolved(t)
	assertClosed := func(t *testing.T, b []byte, id, disposition string, p uint32) {
		t.Helper()
		m := state(t, b)
		o := m.ops[id]
		if o == nil || !o.closed {
			t.Fatal("not resolved")
		}
		r, e := typed[MigrationResolution](m.p.Histories[o.idx].Resolution)
		if e != nil || r.Disposition != disposition || r.Final.VerifiedPrefix != p {
			t.Fatalf("wrong resolution: %+v %v", r, e)
		}
	}
	assertOpen := func(t *testing.T, b []byte, id string) {
		t.Helper()
		m := state(t, b)
		if m.ops[id] == nil || m.ops[id].closed || !m.blocked() {
			t.Fatal("blocker cleared")
		}
	}
	t.Run("OpenIntent", func(t *testing.T) {
		m := state(t, opened)
		if m.migration != mid || m.reservation() != 1 || m.anchor(m.roster()[0]).Prefix != 0 || !isNull(m.p.Histories[0].Resolution) {
			t.Fatal("intent missing")
		}
	})
	t.Run("H2NoOp", func(t *testing.T) {
		b, id := begin(t, base, 0, 5, 5)
		x := finalObs(t, b, 5, nil, "NOT_STARTED", "NO_MUTATION", "COMPLETE", false)
		out := accept(t, b, terminalReq(t, b, id, "COMPLETED", x))
		assertClosed(t, out, id, "COMPLETED", 5)
		m := state(t, out)
		if m.anchor(m.roster()[0]).HistoryDigest != hd(5) || m.reservation() != 0 {
			t.Fatal("no-op history changed")
		}
	})
	t.Run("CommitIntermediateThenStop", func(t *testing.T) {
		x := finalObs(t, committed, 2, ver(3), "NOT_STARTED", "MUTATED", "CANCELED", false)
		b := accept(t, committed, terminalReq(t, committed, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 2)
	})
	t.Run("FirstVersionRollback", func(t *testing.T) {
		x := finalObs(t, opened, 0, ver(1), "ROLLED_BACK", "NO_MUTATION", "ERROR", false)
		b := accept(t, opened, terminalReq(t, opened, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 0)
	})
	t.Run("RefusalWithoutMutation", func(t *testing.T) {
		x := finalObs(t, opened, 0, nil, "NOT_STARTED", "NO_MUTATION", "REFUSED", false)
		b := accept(t, opened, terminalReq(t, opened, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 0)
	})
	t.Run("AppendKnownPrefix", func(t *testing.T) {
		m := state(t, committed)
		if m.ops[mid].last.VerifiedPrefix != 2 || m.ops[mid].last.HistoryDigest != hd(2) || m.ops[mid].closed {
			t.Fatal("observation lost")
		}
	})
	t.Run("PreserveUnknownThenReconcile", func(t *testing.T) {
		x := finalObs(t, held, 3, ver(3), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		b := accept(t, held, terminalReq(t, held, mid, "RECONCILED", x))
		assertClosed(t, b, mid, "RECONCILED", 3)
		if !bytes.Equal(payload(t, b).Histories[0].Observations[1], payload(t, held).Histories[0].Observations[1]) {
			t.Fatal("uncertainty erased")
		}
	})
	t.Run("StandaloneRestoreAtomicRebind", func(t *testing.T) {
		b, id := beginRestore(t, closed, nil)
		x := restoreFinal(t, b, id, false, 1)
		out := accept(t, b, restoreTerminalReq(t, b, id, x))
		m := state(t, out)
		if m.blocked() {
			t.Fatal("fence remains")
		}
		for _, a := range x.NewBindings {
			if m.p.Slots[m.byKey[a.Key]].Current != a.Binding || !equalAnchor(m.anchor(a.Key), a.Anchor) {
				t.Fatal("partial rebind")
			}
		}
	})
	t.Run("LinkedRestoreAtomicResolution", func(t *testing.T) {
		m := state(t, paired)
		if m.blocked() || !m.ops[lmid].closed || !m.ops[rid].closed {
			t.Fatal("paired blockers")
		}
		assertClosed(t, paired, lmid, "RECONCILED", 1)
	})
	t.Run("RestartReviewKeepsFence", func(t *testing.T) {
		b := accept(t, linked, restartReq(t, linked, testID(999)))
		m := state(t, b)
		if m.restore != rid || m.migration != lmid || !m.ops[rid].latch || !m.ops[lmid].latch {
			t.Fatal("restart cleared fence")
		}
	})
	t.Run("FreshAdmissionAfterAllResolved", func(t *testing.T) { b, id := begin(t, closed, 1, 0, 1); assertOpen(t, b, id) })
	t.Run("InputDeepCopy", func(t *testing.T) {
		before := append([]byte(nil), base...)
		x := opening(t, base, 0, 0, 1)
		r := openReq(t, base, testID(999), x)
		b := accept(t, base, r)
		b[0] = '!'
		if !bytes.Equal(base, before) {
			t.Fatal("source aliases output")
		}
		if r.Body[0] != '{' {
			t.Fatal("request aliases output")
		}
	})
	t.Run("RevisionAndHistoryPreservation", func(t *testing.T) {
		old := payload(t, committed)
		next := payload(t, held)
		if next.Revision != old.Revision+1 || !bytes.Equal(old.Histories[0].Opening, next.Histories[0].Opening) || !bytes.Equal(old.Histories[0].Observations[0], next.Histories[0].Observations[0]) {
			t.Fatal("prior history changed")
		}
	})
	t.Run("PairedResolutionSingleRevision", func(t *testing.T) {
		m := payload(t, paired)
		a, _ := typed[MigrationResolution](m.Histories[0].Resolution)
		b, _ := typed[RestoreResolution](m.Histories[1].Resolution)
		if a.Revision != b.Revision || a.Revision != m.Revision || m.Revision != payload(t, linked).Revision+1 {
			t.Fatal("not atomic revision")
		}
	})
	t.Run("IntermediateCommitNextNotStarted", func(t *testing.T) {
		x := finalObs(t, committed, 2, ver(3), "NOT_STARTED", "MUTATED", "TIMEOUT", false)
		b := accept(t, committed, terminalReq(t, committed, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 2)
	})
	t.Run("IntermediateCommitNextRollback", func(t *testing.T) {
		x := finalObs(t, committed, 2, ver(3), "ROLLED_BACK", "MUTATED", "ERROR", false)
		b := accept(t, committed, terminalReq(t, committed, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 2)
	})
	t.Run("KnownPrefixSurvivesUnknown", func(t *testing.T) {
		m := state(t, held)
		if m.ops[mid].last.VerifiedPrefix != 2 || m.ops[mid].last.AttemptedEffect != "UNKNOWN" || m.ops[mid].last.OperationEffect != "MUTATED" {
			t.Fatal("lost known prefix")
		}
	})
	t.Run("CrossOperationAnchorMatch", func(t *testing.T) {
		b, id := begin(t, closed, 0, 5, 5)
		m := state(t, b)
		if m.ops[id].migration.StartingHistoryDigest != hd(5) || m.anchor(m.roster()[0]).Prefix != 5 {
			t.Fatal("anchor mismatch")
		}
	})
	t.Run("AbortedStandaloneRestore", func(t *testing.T) {
		b, id := beginRestore(t, closed, nil)
		out := accept(t, b, restoreTerminalReq(t, b, id, restoreFinal(t, b, id, true, 0)))
		m := state(t, out)
		old := state(t, closed)
		if m.blocked() {
			t.Fatal("abort remains fenced")
		}
		for _, k := range m.roster() {
			if m.p.Slots[m.byKey[k]].Current != old.p.Slots[old.byKey[k]].Current || !equalAnchor(m.anchor(k), old.anchor(k)) {
				t.Fatal("abort changed binding")
			}
		}
	})
	t.Run("AbortedLinkedRestoreKeepsParent", func(t *testing.T) {
		b := accept(t, linked, restoreTerminalReq(t, linked, rid, restoreFinal(t, linked, rid, true, 0)))
		m := state(t, b)
		if m.restore != "" || m.migration != lmid || !m.ops[lmid].latch || m.reservation() != 0 {
			t.Fatal("linked abort cleared parent")
		}
	})
	t.Run("OrdinaryObservationCapPreservesEmergency", func(t *testing.T) {
		b := opened
		for i := uint32(1); i <= 3; i++ {
			b = progress(t, b, mid, i)
		}
		b = holdUnknown(t, b, mid, 3, ver(4))
		if len(payload(t, b).Histories[0].Observations) != 4 {
			t.Fatal("emergency slot")
		}
	})
	t.Run("PairedResolutionReplay", func(t *testing.T) {
		roundTrip(t, paired)
		m := state(t, paired)
		if m.anchor(m.roster()[0]).Prefix != 0 {
			t.Fatal("old anchor overrides restored one")
		}
	})
	t.Run("FreshAdmissionAfterApprovedRestore", func(t *testing.T) {
		b, id := begin(t, paired, 0, 0, 1)
		m := state(t, b)
		if m.ops[id].migration.Binding != m.p.Slots[0].Current {
			t.Fatal("old binding")
		}
	})
	t.Run("UnknownPhaseReviewedOutcome", func(t *testing.T) {
		b := holdUnknown(t, committed, mid, 2, nil)
		x := finalObs(t, b, 3, ver(3), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		out := accept(t, b, terminalReq(t, b, mid, "RECONCILED", x))
		assertClosed(t, out, mid, "RECONCILED", 3)
	})
	t.Run("ExactTargetStoppedNotSuccess", func(t *testing.T) {
		x := finalObs(t, opened, 5, ver(5), "COMMITTED", "MUTATED", "ERROR", false)
		b := accept(t, opened, terminalReq(t, opened, mid, "STOPPED", x))
		assertClosed(t, b, mid, "STOPPED", 5)
	})
	t.Run("RestartReviewWithoutHistory", func(t *testing.T) {
		b := accept(t, base, restartReq(t, base, testID(999)))
		p := payload(t, b)
		if len(p.Histories) != 0 || len(p.RestartReviews) != 1 || p.Revision != 1 {
			t.Fatal("review-only snapshot")
		}
	})
	t.Run("ReviewedAbortRefinesUnknownWithPositiveProof", func(t *testing.T) {
		b, id := beginRestore(t, base, nil)
		b = accept(t, b, restoreAppendReq(t, b, id, "RESTORE", restoreObs(t, b, "UNCERTAIN", "UNKNOWN", "UNVERIFIED")))
		out := accept(t, b, restoreTerminalReq(t, b, id, restoreFinal(t, b, id, true, 0)))
		m := state(t, out)
		if m.blocked() || m.p.Slots[0].Current != payload(t, base).Slots[0].Current {
			t.Fatal("verified abort")
		}
	})
	t.Run("MissingAuthorityOrStaleRevision", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.Kind = ""
		refuse(t, base, r, ErrInvalidRequest)
		r = openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.SourceRevision = 1
		refuse(t, base, r, ErrStaleSource)
	})
	t.Run("UntrustedOrWrongBinding", func(t *testing.T) {
		x := opening(t, base, 0, 0, 1)
		x.Binding.ReceiptDigest = testDigest(999)
		refuse(t, base, openReq(t, base, testID(999), x), ErrInvalidTransition)
	})
	t.Run("GlobalRefusalSameAndOtherSlot", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			refuse(t, opened, openReq(t, opened, testID(999), opening(t, opened, i, 0, 1)), ErrInvalidTransition)
			refuse(t, linked, openReq(t, linked, testID(999), opening(t, linked, i, 0, 1)), ErrInvalidTransition)
		}
	})
	t.Run("H2WhileBlocked", func(t *testing.T) {
		refuse(t, held, openReq(t, held, testID(999), opening(t, held, 1, 0, 0)), ErrInvalidTransition)
	})
	t.Run("UnrelatedRestoreWhileBlocked", func(t *testing.T) {
		refuse(t, held, restoreOpenReq(t, held, testID(999), restoreOpening(t, held, nil)), ErrInvalidTransition)
	})
	t.Run("WrongScopeOrSecondLinkedRestore", func(t *testing.T) {
		x := restoreOpening(t, held, &mid)
		x.AffectedSlots = x.AffectedSlots[1:]
		x.PriorBindings = x.PriorBindings[1:]
		refuse(t, held, restoreOpenReq(t, held, testID(999), x), ErrInvalidTransition)
		refuse(t, linked, restoreOpenReq(t, linked, testID(999), restoreOpening(t, linked, &lmid)), ErrInvalidTransition)
	})
	t.Run("LossOfOwnerNormalCompletion", func(t *testing.T) {
		b := accept(t, opened, restartReq(t, opened, testID(999)))
		x := finalObs(t, b, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		refuse(t, b, terminalReq(t, b, mid, "COMPLETED", x), ErrAuthorityScope)
	})
	t.Run("UnknownOrUnsettledTerminal", func(t *testing.T) {
		x := finalObs(t, opened, 0, ver(1), "UNKNOWN", "UNKNOWN", "ERROR", false)
		refuse(t, opened, terminalReq(t, opened, mid, "STOPPED", x), ErrInvalidTransition)
		x = finalObs(t, opened, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		x.Cleanup = "UNVERIFIED"
		refuse(t, opened, terminalReq(t, opened, mid, "COMPLETED", x), ErrInvalidTransition)
	})
	t.Run("ExactTargetFailedPostflightNotSuccess", func(t *testing.T) {
		x := observation(t, opened, "HOLD", "POSTFLIGHT", 5, ver(5), "COMMITTED", "MUTATED", "ERROR", "UNVERIFIED", "SETTLED")
		b := accept(t, opened, appendReq(t, opened, mid, "ACTIVE", x))
		f := finalObs(t, b, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		refuse(t, b, terminalReq(t, b, mid, "COMPLETED", f), ErrAuthorityScope)
		assertOpen(t, b, mid)
	})
	t.Run("PrefixDecreaseOrDigestConflict", func(t *testing.T) {
		x := finalObs(t, committed, 1, ver(1), "COMMITTED", "MUTATED", "ERROR", false)
		refuse(t, committed, terminalReq(t, committed, mid, "STOPPED", x), ErrInvalidTransition)
		x = finalObs(t, committed, 2, ver(2), "COMMITTED", "MUTATED", "ERROR", false)
		x.HistoryDigest = testDigest(999)
		refuse(t, committed, terminalReq(t, committed, mid, "STOPPED", x), ErrInvalidTransition)
	})
	t.Run("NoMutationAfterKnownCommit", func(t *testing.T) {
		x := finalObs(t, committed, 2, ver(3), "NOT_STARTED", "NO_MUTATION", "CANCELED", false)
		refuse(t, committed, terminalReq(t, committed, mid, "STOPPED", x), ErrInvalidTransition)
	})
	t.Run("RewriteOrDeleteHistory", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, committed, func(p *Payload) { p.Histories[0].Observations = []json.RawMessage{} }))
		x := finalObs(t, closed, 5, ver(5), "COMMITTED", "MUTATED", "ERROR", false)
		refuse(t, closed, terminalReq(t, closed, mid, "STOPPED", x), ErrInvalidTransition)
	})
	t.Run("DuplicateIDOrResolution", func(t *testing.T) {
		refuse(t, closed, openReq(t, closed, mid, opening(t, closed, 0, 5, 5)), ErrInvalidTransition)
		x := finalObs(t, closed, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		refuse(t, closed, terminalReq(t, closed, mid, "COMPLETED", x), ErrInvalidTransition)
	})
	t.Run("PartialRebindOrReusedIncarnation", func(t *testing.T) {
		b, id := beginRestore(t, base, nil)
		x := restoreFinal(t, b, id, false, 0)
		x.NewBindings = x.NewBindings[:1]
		refuse(t, b, restoreTerminalReq(t, b, id, x), ErrInvalidTransition)
		x = restoreFinal(t, b, id, false, 0)
		x.NewBindings[0].Binding = payload(t, base).Slots[0].Initial
		refuse(t, b, restoreTerminalReq(t, b, id, x), ErrInvalidTransition)
	})
	t.Run("UnprovedParentEffect", func(t *testing.T) {
		rr := restoreFinal(t, linked, rid, false, 0)
		x := finalObs(t, linked, 0, ver(1), "UNKNOWN", "UNKNOWN", "UNCERTAIN", true)
		x.AuthorizationDigest = rr.AuthorizationDigest
		refuse(t, linked, pairReq(t, linked, rid, lmid, rr, x), ErrInvalidTransition)
	})
	t.Run("RestartReviewNotAdmission", func(t *testing.T) {
		b := accept(t, held, restartReq(t, held, testID(999)))
		refuse(t, b, openReq(t, b, testID(1000), opening(t, b, 1, 0, 1)), ErrInvalidTransition)
	})
	t.Run("CapacityRefusalUnchanged", func(t *testing.T) {
		b := many(t, 31, 1, false, 0)
		refuse(t, b, openReq(t, b, testID(999), opening(t, b, 0, 0, 1)), ErrCapacity)
		// Assert the approved precedence across independent failures.
		// Capacity is already exhausted for a migration (31 closed histories).
		x := opening(t, b, 0, 0, 1)
		x.Binding.ReceiptDigest = testDigest(999) // semantic conflict
		r := openReq(t, b, testID(999), x)
		r.Assertion.Scope = []Key{{testID(3), testID(999), "IDENTITY"}} // wrong scope, valid grammar
		r.Assertion.BodyDigest = testDigest(999)                        // wrong body binding
		r.Assertion.SourceRevision++                                    // stale
		corrupt := bytes.Replace(b, []byte(`"format":"mamony-migration-journal-v1"`), []byte(`"format":"invalid"`), 1)
		refuse(t, corrupt, r, ErrInvalidSnapshot)
		refuse(t, b, r, ErrStaleSource)
		r.Assertion.SourceRevision--
		refuse(t, b, r, ErrInvalidRequest)
		r.Assertion.BodyDigest = openReq(t, b, testID(999), x).Assertion.BodyDigest
		refuse(t, b, r, ErrAuthorityScope)
		r.Assertion.Scope = []Key{x.Slot}
		refuse(t, b, r, ErrInvalidTransition)
		x.Binding = payload(t, b).Slots[0].Current
		refuse(t, b, openReq(t, b, testID(999), x), ErrCapacity)
	})
	t.Run("AliasingMutation", func(t *testing.T) {
		s := read(t, committed)
		before := append([]byte(nil), committed...)
		s.Payload.Histories[0].Observations[0][0] = '!'
		if !bytes.Equal(committed, before) {
			t.Fatal("decoder aliases source")
		}
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		out := accept(t, base, r)
		r.Body[0] = '!'
		read(t, out)
	})
	t.Run("ForgedReadyFlag", func(t *testing.T) {
		badDecode(t, bytes.Replace(held, []byte(`"revision":3`), []byte(`"ready":true,"revision":3`), 1))
		p := rawPayload(t, held)
		badDecode(t, wireWithPayload(replaceOnce(t, p, `"revision":3,"slots":`, `"ready":true,"revision":3,"slots":`)))
	})
	t.Run("StaleCandidateApplication", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		refuse(t, opened, r, ErrStaleSource)
	})
	t.Run("MissingHistoricalFacts", func(t *testing.T) { badDecode(t, mutateSnapshot(t, closed, func(p *Payload) { p.Histories = nil })) })
	t.Run("DisallowedBindingChange", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, committed, func(p *Payload) { p.Slots[0].Current.IncarnationID = testID(999) }))
	})
	t.Run("RevisionGapOrDuplicate", func(t *testing.T) {
		badDecode(t, mutateSnapshot(t, committed, func(p *Payload) { p.Revision++ }))
		badDecode(t, mutateSnapshot(t, committed, func(p *Payload) {
			x, _ := typed[MigrationObservation](p.Histories[0].Observations[0])
			x.Revision = 1
			p.Histories[0].Observations[0] = mustRaw(t, x)
		}))
	})
	t.Run("UnknownRequiresReview", func(t *testing.T) {
		x := finalObs(t, held, 2, ver(3), "ROLLED_BACK", "MUTATED", "ERROR", false)
		refuse(t, held, terminalReq(t, held, mid, "STOPPED", x), ErrAuthorityScope)
	})
	t.Run("FailedPostflightCannotComplete", func(t *testing.T) {
		x := finalObs(t, opened, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		x.Verification = "MISMATCH"
		refuse(t, opened, terminalReq(t, opened, mid, "COMPLETED", x), ErrInvalidTransition)
	})
	t.Run("AuthorityLossCannotComplete", func(t *testing.T) {
		x := observation(t, opened, "HOLD", "PREFLIGHT", 0, nil, "NOT_STARTED", "NO_MUTATION", "AUTHORITY_LOST", "UNVERIFIED", "UNVERIFIED")
		b := accept(t, opened, appendReq(t, opened, mid, "REVIEW", x))
		f := finalObs(t, b, 5, ver(5), "COMMITTED", "MUTATED", "COMPLETE", false)
		refuse(t, b, terminalReq(t, b, mid, "COMPLETED", f), ErrAuthorityScope)
	})
	t.Run("RevalidationConflictKeepsHold", func(t *testing.T) {
		x := observation(t, opened, "HOLD", "PREFLIGHT", 0, nil, "NOT_STARTED", "NO_MUTATION", "REFUSED", "MISMATCH", "SETTLED")
		b := accept(t, opened, appendReq(t, opened, mid, "ACTIVE", x))
		f := finalObs(t, b, 0, nil, "NOT_STARTED", "NO_MUTATION", "REFUSED", false)
		refuse(t, b, terminalReq(t, b, mid, "STOPPED", f), ErrAuthorityScope)
		assertOpen(t, b, mid)
	})
	t.Run("PartialOrUncertainRestoreCannotAbort", func(t *testing.T) {
		b, id := beginRestore(t, base, nil)
		x := restoreObs(t, b, "INTERRUPTED", "CHANGED", "UNVERIFIED")
		b = accept(t, b, restoreAppendReq(t, b, id, "RESTORE", x))
		refuse(t, b, restoreTerminalReq(t, b, id, restoreFinal(t, b, id, true, 0)), ErrInvalidTransition)
		b, id = beginRestore(t, base, nil)
		b = accept(t, b, restoreAppendReq(t, b, id, "RESTORE", restoreObs(t, b, "UNCERTAIN", "UNKNOWN", "UNVERIFIED")))
		r := restoreFinal(t, b, id, true, 0)
		r.Mutation = "UNKNOWN"
		refuse(t, b, restoreTerminalReq(t, b, id, r), ErrInvalidTransition)
	})
	t.Run("DirtyRestoreRemainsFenced", func(t *testing.T) {
		b, id := beginRestore(t, base, nil)
		x := restoreFinal(t, b, id, false, 0)
		x.Verification = "MISMATCH"
		refuse(t, b, restoreTerminalReq(t, b, id, x), ErrInvalidTransition)
		assertOpen(t, b, id)
	})
	t.Run("AssertionWrongActionOrScope", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.Scope = []Key{payload(t, base).Slots[1].Key}
		refuse(t, base, r, ErrAuthorityScope)
		r = openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.Action = "OPEN_RESTORE"
		refuse(t, base, r, ErrInvalidRequest)
	})
	t.Run("AssertionWrongSourceDigest", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.SourceDigest = testDigest(999)
		refuse(t, base, r, ErrStaleSource)
	})
	t.Run("RestartReviewNotReusable", func(t *testing.T) {
		r := restartReq(t, base, testID(999))
		b := accept(t, base, r)
		refuse(t, b, r, ErrStaleSource)
		refuse(t, b, restartReq(t, b, testID(999)), ErrInvalidTransition)
	})
	t.Run("AssertionWrongBodyDigest", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.BodyDigest = testDigest(999)
		refuse(t, base, r, ErrInvalidRequest)
	})
	t.Run("AssertionWrongKindOrJournal", func(t *testing.T) {
		r := openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.Kind = "REVIEW"
		refuse(t, base, r, ErrAuthorityScope)
		r = openReq(t, base, testID(999), opening(t, base, 0, 0, 1))
		r.Assertion.JournalID = testID(999)
		refuse(t, base, r, ErrStaleSource)
	})
	t.Run("ProgressAfterHoldOrRestart", func(t *testing.T) {
		for _, b := range [][]byte{held, accept(t, committed, restartReq(t, committed, testID(999)))} {
			x := observation(t, b, "PROGRESS", "VERSION", 3, ver(3), "COMMITTED", "MUTATED", "IN_PROGRESS", "VERIFIED", "PENDING")
			refuse(t, b, appendReq(t, b, mid, "ACTIVE", x), ErrAuthorityScope)
		}
	})
	t.Run("KnownEffectCannotBeRewritten", func(t *testing.T) {
		x := observation(t, committed, "HOLD", "CLEANUP", 2, ver(3), "ROLLED_BACK", "MUTATED", "ERROR", "UNVERIFIED", "UNVERIFIED")
		b := accept(t, committed, appendReq(t, committed, mid, "ACTIVE", x))
		f := finalObs(t, b, 3, ver(3), "COMMITTED", "MUTATED", "ERROR", true)
		refuse(t, b, terminalReq(t, b, mid, "RECONCILED", f), ErrInvalidTransition)
	})
	t.Run("UnknownSelectedVersionCannotSkip", func(t *testing.T) {
		x := finalObs(t, held, 4, ver(4), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		refuse(t, held, terminalReq(t, held, mid, "RECONCILED", x), ErrInvalidTransition)
	})
	t.Run("AnomalyCannotReconcile", func(t *testing.T) {
		x := observation(t, opened, "HOLD", "VERSION", 0, ver(1), "UNKNOWN", "ANOMALOUS", "ERROR", "MISMATCH", "UNVERIFIED")
		b := accept(t, opened, appendReq(t, opened, mid, "ACTIVE", x))
		f := finalObs(t, b, 0, ver(1), "ROLLED_BACK", "NO_MUTATION", "ERROR", true)
		refuse(t, b, terminalReq(t, b, mid, "RECONCILED", f), ErrInvalidTransition)
	})
	t.Run("ParentCannotResolveWithOpenFence", func(t *testing.T) {
		x := finalObs(t, linked, 1, ver(1), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		refuse(t, linked, terminalReq(t, linked, lmid, "RECONCILED", x), ErrInvalidTransition)
	})
	t.Run("PairedResolutionWrongAuthorization", func(t *testing.T) {
		rr := restoreFinal(t, linked, rid, false, 0)
		x := finalObs(t, linked, 1, ver(1), "COMMITTED", "MUTATED", "UNCERTAIN", true)
		refuse(t, linked, pairReq(t, linked, rid, lmid, rr, x), ErrInvalidRequest)
	})
	t.Run("LinkedAbortNoSecondRestore", func(t *testing.T) {
		b := accept(t, linked, restoreTerminalReq(t, linked, rid, restoreFinal(t, linked, rid, true, 0)))
		refuse(t, b, restoreOpenReq(t, b, testID(999), restoreOpening(t, b, &lmid)), ErrInvalidTransition)
	})
	t.Run("KnownNoSelectionHoldCannotCommit", func(t *testing.T) {
		x := observation(t, opened, "HOLD", "PREFLIGHT", 0, nil, "NOT_STARTED", "NO_MUTATION", "AUTHORITY_LOST", "UNVERIFIED", "UNVERIFIED")
		b := accept(t, opened, appendReq(t, opened, mid, "REVIEW", x))
		f := finalObs(t, b, 1, ver(1), "COMMITTED", "MUTATED", "AUTHORITY_LOST", true)
		refuse(t, b, terminalReq(t, b, mid, "RECONCILED", f), ErrInvalidTransition)
	})
}
