package journal

import (
	"encoding/json"
)

// Assertion is a claimed external approval bound to one source and one action.
// The model checks binding, not authenticity, controller ownership or settlement.
type Assertion struct {
	Kind                string `json:"kind"`
	JournalID           string `json:"journal_id"`
	SourceRevision      uint64 `json:"source_revision"`
	SourceDigest        string `json:"source_digest"`
	Action              string `json:"action"`
	OperationID         string `json:"operation_id"`
	Scope               []Key  `json:"scope"`
	BodyDigest          string `json:"body_digest"`
	AuthorizationDigest string `json:"authorization_digest"`
}
type Request struct {
	Assertion Assertion
	Body      json.RawMessage
}
type migrationOpenBody struct {
	ID      string           `json:"id"`
	Opening MigrationOpening `json:"opening"`
}
type restoreOpenBody struct {
	ID      string         `json:"id"`
	Opening RestoreOpening `json:"opening"`
}
type migrationAppendBody struct {
	ID          string               `json:"id"`
	Observation MigrationObservation `json:"observation"`
}
type restoreAppendBody struct {
	ID          string             `json:"id"`
	Observation RestoreObservation `json:"observation"`
}
type migrationTerminalBody struct {
	ID         string              `json:"id"`
	Resolution MigrationResolution `json:"resolution"`
}
type restoreTerminalBody struct {
	ID         string            `json:"id"`
	Resolution RestoreResolution `json:"resolution"`
}
type pairBody struct {
	RestoreID           string              `json:"restore_id"`
	RestoreResolution   RestoreResolution   `json:"restore_resolution"`
	MigrationID         string              `json:"migration_id"`
	MigrationResolution MigrationResolution `json:"migration_resolution"`
}
type restartBody struct {
	Review RestartReview `json:"review"`
}
type bodyEnvelope struct {
	Action string          `json:"action"`
	Body   json.RawMessage `json:"body"`
}

const transitionDomain = "mamony-migration-journal-transition-v1"

func bodyDigest(action string, body json.RawMessage) (string, error) {
	b, e := json.Marshal(bodyEnvelope{action, body})
	if e != nil {
		return "", e
	}
	return sumDigest(transitionDomain, b), nil
}

// Apply returns a new canonical candidate, not permission to dispatch work or
// acknowledgment of durable publication. It does not mutate source or request.
func Apply(source []byte, request Request) ([]byte, error) {
	s, e := Decode(source)
	if e != nil {
		return nil, ErrInvalidSnapshot
	}
	a := request.Assertion
	if a.JournalID != s.Payload.JournalID || a.SourceRevision != s.Payload.Revision || a.SourceDigest != s.Digest {
		return nil, ErrStaleSource
	}
	if !id(a.OperationID) || !digest(a.BodyDigest) || !digest(a.AuthorizationDigest) || !validKeys(a.Scope) || !oneOf(a.Kind, "ADMISSION", "ACTIVE", "REVIEW", "RESTORE", "RESTART") {
		return nil, ErrInvalidRequest
	}
	es, scope, kinds, auth, e := requestEvents(a.Action, request.Body, s.Payload.Revision+1)
	if e != nil {
		return nil, e
	}
	d, e := bodyDigest(a.Action, request.Body)
	if e != nil || d != a.BodyDigest {
		return nil, ErrInvalidRequest
	}
	m, e := replay(s.Payload)
	if e != nil {
		return nil, ErrInvalidSnapshot
	}
	if len(scope) == 0 {
		if a.Action == "RECORD_RESTART" {
			scope = m.roster()
		} else if o := m.ops[es[0].id]; o != nil {
			if o.migration != nil {
				scope = []Key{o.migration.Slot}
			} else {
				scope = o.restore.AffectedSlots
			}
		}
	}
	if es[0].id != a.OperationID || !oneOf(a.Kind, kinds...) || !sameKeys(a.Scope, scope) || a.AuthorizationDigest != auth {
		return nil, ErrAuthorityScope
	}
	if a.Kind == "ACTIVE" {
		if o := m.ops[es[0].id]; o != nil && o.latch {
			return nil, ErrAuthorityScope
		}
	}
	if e = m.apply(es); e != nil {
		return nil, e
	}
	b, e := envelope(m.p)
	if e != nil {
		return nil, ErrInvalidTransition
	}
	return b, nil
}
func requestEvents(action string, b []byte, revision uint64) ([]event, []Key, []string, string, error) {
	fail := func() ([]event, []Key, []string, string, error) { return nil, nil, nil, "", ErrInvalidRequest }
	makeEvent := func(id, kind, action string, body any) (event, error) {
		r, e := raw(body)
		return event{revision, action, id, kind, r}, e
	}
	switch action {
	case "OPEN_MIGRATION":
		v, e := typed[migrationOpenBody](b)
		if e != nil || !id(v.ID) {
			return fail()
		}
		ev, e := makeEvent(v.ID, "MIGRATION", action, v.Opening)
		if e != nil {
			return fail()
		}
		return []event{ev}, []Key{v.Opening.Slot}, []string{"ADMISSION"}, v.Opening.AuthorizationDigest, nil
	case "OPEN_RESTORE":
		v, e := typed[restoreOpenBody](b)
		if e != nil || !id(v.ID) {
			return fail()
		}
		ev, e := makeEvent(v.ID, "RESTORE", action, v.Opening)
		if e != nil {
			return fail()
		}
		kind := "RESTORE"
		if v.Opening.LinkedMigrationID != nil {
			kind = "REVIEW"
		}
		return []event{ev}, v.Opening.AffectedSlots, []string{kind}, v.Opening.AuthorizationDigest, nil
	case "APPEND_MIGRATION":
		v, e := typed[migrationAppendBody](b)
		if e != nil || !id(v.ID) || v.Observation.Revision != revision || !oneOf(v.Observation.Kind, "PROGRESS", "HOLD") {
			return fail()
		}
		ev, e := makeEvent(v.ID, "MIGRATION", action, v.Observation)
		if e != nil {
			return fail()
		}
		k := []string{"ACTIVE"}
		if v.Observation.Kind == "HOLD" {
			k = append(k, "REVIEW")
		}
		return []event{ev}, nil, k, v.Observation.AuthorizationDigest, nil
	case "APPEND_RESTORE":
		v, e := typed[restoreAppendBody](b)
		if e != nil || !id(v.ID) || v.Observation.Revision != revision {
			return fail()
		}
		ev, e := makeEvent(v.ID, "RESTORE", action, v.Observation)
		if e != nil {
			return fail()
		}
		k := []string{"RESTORE"}
		if oneOf(v.Observation.Phase, "INTERRUPTED", "UNCERTAIN") {
			k = append(k, "REVIEW")
		}
		return []event{ev}, nil, k, v.Observation.AuthorizationDigest, nil
	case "COMPLETE_MIGRATION", "STOP_MIGRATION", "RECONCILE_MIGRATION":
		v, e := typed[migrationTerminalBody](b)
		if e != nil || !id(v.ID) || v.Resolution.Revision != revision || v.Resolution.Final.Revision != revision {
			return fail()
		}
		want := "COMPLETED"
		k := "ACTIVE"
		if action == "STOP_MIGRATION" {
			want = "STOPPED"
		}
		if action == "RECONCILE_MIGRATION" {
			want = "RECONCILED"
			k = "REVIEW"
		}
		if v.Resolution.Disposition != want {
			return fail()
		}
		ev, e := makeEvent(v.ID, "MIGRATION", action, v.Resolution)
		if e != nil {
			return fail()
		}
		return []event{ev}, nil, []string{k}, v.Resolution.Final.AuthorizationDigest, nil
	case "REBIND_RESTORE", "ABORT_RESTORE":
		v, e := typed[restoreTerminalBody](b)
		if e != nil || !id(v.ID) || v.Resolution.Revision != revision {
			return fail()
		}
		want := "REBOUND"
		if action == "ABORT_RESTORE" {
			want = "ABORTED_NO_MUTATION"
		}
		if v.Resolution.Disposition != want {
			return fail()
		}
		ev, e := makeEvent(v.ID, "RESTORE", action, v.Resolution)
		if e != nil {
			return fail()
		}
		return []event{ev}, nil, []string{"REVIEW"}, v.Resolution.AuthorizationDigest, nil
	case "PAIR_REBIND":
		v, e := typed[pairBody](b)
		if e != nil || !id(v.RestoreID) || !id(v.MigrationID) || v.RestoreResolution.Revision != revision || v.MigrationResolution.Revision != revision || v.MigrationResolution.Final.Revision != revision || v.RestoreResolution.Disposition != "REBOUND" || v.MigrationResolution.Disposition != "RECONCILED" || v.RestoreResolution.AuthorizationDigest != v.MigrationResolution.Final.AuthorizationDigest {
			return fail()
		}
		x, e := makeEvent(v.RestoreID, "RESTORE", "REBIND_RESTORE", v.RestoreResolution)
		if e != nil {
			return fail()
		}
		y, e := makeEvent(v.MigrationID, "MIGRATION", "RECONCILE_MIGRATION", v.MigrationResolution)
		if e != nil {
			return fail()
		}
		return []event{x, y}, nil, []string{"REVIEW"}, v.RestoreResolution.AuthorizationDigest, nil
	case "RECORD_RESTART":
		v, e := typed[restartBody](b)
		if e != nil || !id(v.Review.ID) || v.Review.Revision != revision {
			return fail()
		}
		ev, e := makeEvent(v.Review.ID, "", action, v.Review)
		if e != nil {
			return fail()
		}
		return []event{ev}, nil, []string{"RESTART"}, v.Review.AuthorizationDigest, nil
	default:
		return fail()
	}
}

// apply is the shared structural transition used by both replay and new requests.
// Its receiver is a private reconstructed value, discarded on any refusal.
func (m *model) apply(es []event) error {
	if len(es) < 1 || len(es) > 2 || es[0].revision != m.p.Revision+1 {
		return invalid("revision")
	}
	var e error
	if len(es) == 2 {
		a, b := es[0], es[1]
		if a.action == "RECONCILE_MIGRATION" {
			a, b = b, a
		}
		if a.action != "REBIND_RESTORE" || b.action != "RECONCILE_MIGRATION" || a.revision != b.revision {
			return invalid("paired event")
		}
		e = m.pair(a, b)
	} else {
		v := es[0]
		switch v.action {
		case "OPEN_MIGRATION":
			e = m.openMigration(v)
		case "OPEN_RESTORE":
			e = m.openRestore(v)
		case "APPEND_MIGRATION":
			e = m.appendMigration(v)
		case "APPEND_RESTORE":
			e = m.appendRestore(v)
		case "COMPLETE_MIGRATION", "STOP_MIGRATION", "RECONCILE_MIGRATION":
			e = m.closeMigration(v, false)
		case "REBIND_RESTORE", "ABORT_RESTORE":
			e = m.closeRestore(v, false)
		case "RECORD_RESTART":
			e = m.restart(v)
		default:
			e = invalid("action")
		}
	}
	if e != nil {
		return e
	}
	m.p.Revision = es[0].revision
	return m.capacity()
}
func (m *model) appendOpening(v event, o *op) error {
	if !id(v.id) || m.records[v.id] {
		return invalid("duplicate or invalid operation identity")
	}
	o.idx = len(m.p.Histories)
	m.records[v.id] = true
	m.ops[v.id] = o
	m.p.Histories = append(m.p.Histories, History{v.id, v.revision, v.kind, append(json.RawMessage(nil), v.body...), []json.RawMessage{}, json.RawMessage("null")})
	return nil
}
func (m *model) openMigration(v event) error {
	x, e := typed[MigrationOpening](v.body)
	if e != nil || m.blocked() {
		return invalid("migration admission")
	}
	i, ok := m.byKey[x.Slot]
	if !ok || !validBinding(x.Binding) || m.p.Slots[i].Current != x.Binding || x.RequestedVersion > MaxVersion || x.StartingPrefix > x.RequestedVersion || !digests(x.AuthorizationDigest, x.ManifestDigest, x.StartingHistoryDigest, x.InitialObservationDigest) {
		return invalid("migration opening facts")
	}
	a := Anchor{x.StartingPrefix, x.StartingHistoryDigest}
	if old, ok := m.anchors[x.Slot]; ok && old != a {
		return invalid("history anchor mismatch")
	}
	if e = m.appendOpening(v, &op{migration: &x}); e != nil {
		return e
	}
	m.anchors[x.Slot] = a
	m.migration = v.id
	return nil
}
func (m *model) openRestore(v event) error {
	x, e := typed[RestoreOpening](v.body)
	if e != nil || !id(x.EnvironmentID) || !id(x.ClusterID) || !validKeys(x.AffectedSlots) || !sameKeys(x.AffectedSlots, m.scope(x.EnvironmentID, x.ClusterID)) || len(x.PriorBindings) != len(x.AffectedSlots) || !digests(x.RecoveryDigest, x.AuthorizationDigest, x.SettlementEvidenceDigest) || m.restore != "" {
		return invalid("restore opening facts")
	}
	for i, b := range x.PriorBindings {
		k := x.AffectedSlots[i]
		if b.Key != k || b.Binding != m.p.Slots[m.byKey[k]].Current || !equalAnchor(b.Anchor, m.anchor(k)) {
			return invalid("restore prior binding or anchor")
		}
	}
	if x.LinkedMigrationID == nil {
		if m.blocked() {
			return invalid("unrelated restore")
		}
	} else {
		if m.migration == "" || *x.LinkedMigrationID != m.migration {
			return invalid("wrong linked parent")
		}
		p := m.ops[m.migration]
		if p.linked || p.migration.AuthorizationDigest == x.AuthorizationDigest {
			return invalid("used reservation or same authorization")
		}
		inScope := false
		for _, k := range x.AffectedSlots {
			inScope = inScope || k == p.migration.Slot
		}
		if !inScope {
			return invalid("parent outside restore scope")
		}
	}
	if e = m.appendOpening(v, &op{restore: &x}); e != nil {
		return e
	}
	m.restore = v.id
	if x.LinkedMigrationID != nil {
		p := m.ops[*x.LinkedMigrationID]
		p.linked = true
		p.latch = true
	}
	return nil
}
func sameVersion(a, b *uint32) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func validateMigrationObservation(o *op, x MigrationObservation) error {
	m := o.migration
	s, t, p := m.StartingPrefix, m.RequestedVersion, x.VerifiedPrefix
	a := x.AttemptedVersion
	if x.Revision == 0 || x.Revision > MaxRevision || p < s || p > t || !digests(x.HistoryDigest, x.EvidenceDigest, x.AuthorizationDigest) || !oneOf(x.Kind, "PROGRESS", "HOLD", "FINAL") || !oneOf(x.Phase, "PREFLIGHT", "VERSION", "POSTFLIGHT", "CLEANUP", "UNKNOWN_PHASE", "REVIEW") || !oneOf(x.Cause, "IN_PROGRESS", "COMPLETE", "REFUSED", "CANCELED", "TIMEOUT", "INTERRUPTED", "ERROR", "UNCERTAIN", "AUTHORITY_LOST", "CAPACITY") || !oneOf(x.Verification, "PENDING", "VERIFIED", "MISMATCH", "UNVERIFIED") || !oneOf(x.Cleanup, "PENDING", "SETTLED", "UNVERIFIED") || !oneOf(x.OperationEffect, "NO_MUTATION", "MUTATED", "UNKNOWN", "ANOMALOUS") {
		return invalid("observation fields")
	}
	if p == s && x.HistoryDigest != m.StartingHistoryDigest {
		return invalid("starting digest conflict")
	}
	if o.last != nil && (p < o.last.VerifiedPrefix || p == o.last.VerifiedPrefix && x.HistoryDigest != o.last.HistoryDigest) {
		return invalid("verified prefix conflict")
	}
	if a != nil && (*a <= s || *a > t) {
		return invalid("attempt bounds")
	}
	switch x.AttemptedEffect {
	case "NOT_STARTED":
		if a == nil {
			if p != s {
				return invalid("missing selected version")
			}
		} else if *a != p+1 {
			return invalid("not-started version")
		}
	case "ROLLED_BACK":
		if a == nil || *a != p+1 {
			return invalid("rollback version")
		}
	case "COMMITTED":
		if a == nil || *a != p || p <= s {
			return invalid("committed version")
		}
	case "UNKNOWN":
		if a == nil {
			if x.Kind != "HOLD" || x.Phase != "UNKNOWN_PHASE" {
				return invalid("unknown attempted version")
			}
		} else if *a != p+1 {
			return invalid("unknown version bounds")
		}
	default:
		return invalid("attempt effect")
	}
	if x.OperationEffect == "ANOMALOUS" {
		if x.Kind != "HOLD" {
			return invalid("anomalous terminal")
		}
	} else if p > s {
		if x.OperationEffect != "MUTATED" {
			return invalid("known committed prefix erased")
		}
	} else {
		if x.OperationEffect == "MUTATED" || x.AttemptedEffect == "UNKNOWN" && x.OperationEffect != "UNKNOWN" {
			return invalid("operation effect")
		}
	}
	if x.Kind == "PROGRESS" {
		prior := s
		if o.last != nil {
			prior = o.last.VerifiedPrefix
		}
		if x.Phase != "VERSION" || x.Cause != "IN_PROGRESS" || x.Verification != "VERIFIED" || x.Cleanup != "PENDING" || x.AttemptedEffect != "COMMITTED" || x.OperationEffect != "MUTATED" || p <= prior {
			return invalid("progress combination")
		}
	}
	if x.Kind == "HOLD" {
		if x.Phase == "REVIEW" || oneOf(x.Cause, "IN_PROGRESS", "COMPLETE") || x.Phase == "PREFLIGHT" && (x.AttemptedEffect != "NOT_STARTED" || p != s) {
			return invalid("hold phase/cause")
		}
		if !(x.AttemptedEffect == "UNKNOWN" || oneOf(x.OperationEffect, "UNKNOWN", "ANOMALOUS") || oneOf(x.Verification, "MISMATCH", "UNVERIFIED") || x.Cleanup == "UNVERIFIED" || oneOf(x.Cause, "INTERRUPTED", "UNCERTAIN", "AUTHORITY_LOST", "CAPACITY")) {
			return invalid("missing hold trigger")
		}
	}
	if x.Kind == "FINAL" {
		if x.AttemptedEffect == "UNKNOWN" || x.Verification != "VERIFIED" || x.Cleanup != "SETTLED" || p == s && x.OperationEffect != "NO_MUTATION" || p > s && x.OperationEffect != "MUTATED" {
			return invalid("unverified terminal")
		}
		if o.held != nil {
			h := o.held
			if h.OperationEffect == "ANOMALOUS" {
				return invalid("anomaly requires separate disposition")
			}
			if h.AttemptedEffect != "UNKNOWN" {
				if !sameVersion(a, h.AttemptedVersion) || x.AttemptedEffect != h.AttemptedEffect || p != h.VerifiedPrefix || x.HistoryDigest != h.HistoryDigest {
					return invalid("known hold facts changed")
				}
			}
			if h.AttemptedEffect == "UNKNOWN" && h.AttemptedVersion != nil {
				if !sameVersion(a, h.AttemptedVersion) {
					return invalid("uncertain selected version changed")
				}
			}
			if h.OperationEffect == "NO_MUTATION" && x.OperationEffect != "NO_MUTATION" {
				return invalid("known no mutation changed")
			}
		}
	}
	return nil
}
func (m *model) appendMigration(v event) error {
	o := m.ops[v.id]
	if m.migration != v.id || m.restore != "" || o == nil || o.closed {
		return invalid("migration is not open")
	}
	x, e := typed[MigrationObservation](v.body)
	if e != nil || x.Revision != v.revision || !oneOf(x.Kind, "PROGRESS", "HOLD") {
		return invalid("observation shape")
	}
	if e = validateMigrationObservation(o, x); e != nil {
		return e
	}
	if o.hold {
		return ErrCapacity
	}
	if x.Kind == "PROGRESS" {
		if o.latch {
			return invalid("progress after review requirement")
		}
		if o.ordinary >= 3 {
			return ErrCapacity
		}
		o.ordinary++
	} else {
		o.hold = true
		o.latch = true
		o.held = &x
	}
	o.last = &x
	m.p.Histories[o.idx].Observations = append(m.p.Histories[o.idx].Observations, append(json.RawMessage(nil), v.body...))
	return nil
}
func (m *model) checkMigrationTerminal(v event, paired bool) (*op, MigrationResolution, error) {
	var z MigrationResolution
	o := m.ops[v.id]
	if m.migration != v.id || o == nil || o.closed || !paired && m.restore != "" {
		return nil, z, invalid("migration not eligible for resolution")
	}
	x, e := typed[MigrationResolution](v.body)
	if e != nil || x.Revision != v.revision || x.Final.Revision != v.revision || x.Final.Kind != "FINAL" {
		return nil, z, invalid("terminal shape")
	}
	f := x.Final
	if e = validateMigrationObservation(o, f); e != nil {
		return nil, z, e
	}
	switch x.Disposition {
	case "COMPLETED":
		if v.action != "COMPLETE_MIGRATION" || o.latch || f.Phase != "CLEANUP" || f.Cause != "COMPLETE" || f.VerifiedPrefix != o.migration.RequestedVersion || !(f.AttemptedEffect == "COMMITTED" || f.AttemptedEffect == "NOT_STARTED" && f.AttemptedVersion == nil && f.VerifiedPrefix == o.migration.StartingPrefix && f.OperationEffect == "NO_MUTATION") {
			return nil, z, invalid("completion guards")
		}
	case "STOPPED":
		if v.action != "STOP_MIGRATION" || o.latch || f.Phase != "CLEANUP" || !oneOf(f.Cause, "REFUSED", "CANCELED", "TIMEOUT", "ERROR") {
			return nil, z, invalid("stop guards")
		}
	case "RECONCILED":
		if v.action != "RECONCILE_MIGRATION" || f.Phase != "REVIEW" || !oneOf(f.Cause, "REFUSED", "CANCELED", "TIMEOUT", "INTERRUPTED", "ERROR", "UNCERTAIN", "AUTHORITY_LOST", "CAPACITY") || f.AuthorizationDigest == o.migration.AuthorizationDigest {
			return nil, z, invalid("reconciliation guards")
		}
	default:
		return nil, z, invalid("disposition")
	}
	return o, x, nil
}
func (m *model) finishMigration(v event, o *op, x MigrationResolution) {
	o.closed = true
	m.p.Histories[o.idx].Resolution = append(json.RawMessage(nil), v.body...)
	m.anchors[o.migration.Slot] = Anchor{x.Final.VerifiedPrefix, x.Final.HistoryDigest}
	m.migration = ""
}
func (m *model) closeMigration(v event, paired bool) error {
	o, x, e := m.checkMigrationTerminal(v, paired)
	if e != nil {
		return e
	}
	m.finishMigration(v, o, x)
	return nil
}
func (m *model) appendRestore(v event) error {
	o := m.ops[v.id]
	if m.restore != v.id || o == nil || o.closed {
		return invalid("restore not open")
	}
	x, e := typed[RestoreObservation](v.body)
	if e != nil || x.Revision != v.revision || !digests(x.EvidenceDigest, x.AuthorizationDigest) || !oneOf(x.Mutation, "NONE", "CHANGED", "UNKNOWN") || !oneOf(x.Settlement, "SETTLED", "UNVERIFIED") || o.restoreChanged && x.Mutation == "NONE" {
		return invalid("restore observation facts")
	}
	if o.hold {
		return ErrCapacity
	}
	switch x.Phase {
	case "PREPARED", "OBSERVED":
		if o.latch || x.Settlement != "SETTLED" || x.Mutation == "UNKNOWN" || x.Phase == "PREPARED" && (o.ordinary != 0 || x.Mutation != "NONE") {
			return invalid("restore ordinary observation")
		}
		if o.ordinary >= 3 {
			return ErrCapacity
		}
		o.ordinary++
	case "INTERRUPTED", "UNCERTAIN":
		o.hold = true
		o.latch = true
	default:
		return invalid("restore phase")
	}
	o.restoreChanged = o.restoreChanged || x.Mutation == "CHANGED"
	m.p.Histories[o.idx].Observations = append(m.p.Histories[o.idx].Observations, append(json.RawMessage(nil), v.body...))
	return nil
}
func (m *model) checkRestoreTerminal(v event, paired bool) (*op, RestoreResolution, error) {
	var z RestoreResolution
	o := m.ops[v.id]
	if m.restore != v.id || o == nil || o.closed {
		return nil, z, invalid("restore not open")
	}
	x, e := typed[RestoreResolution](v.body)
	if e != nil || x.Revision != v.revision || !digests(x.EvidenceDigest, x.AuthorizationDigest) || x.AuthorizationDigest == o.restore.AuthorizationDigest || x.Verification != "VERIFIED" || x.Settlement != "SETTLED" {
		return nil, z, invalid("restore terminal facts")
	}
	switch x.Disposition {
	case "ABORTED_NO_MUTATION":
		if v.action != "ABORT_RESTORE" || paired || x.NewBindings != nil || x.Mutation != "NONE" || o.restoreChanged {
			return nil, z, invalid("abort requires no mutation")
		}
	case "REBOUND":
		if v.action != "REBIND_RESTORE" || x.Mutation != "CHANGED" || len(x.NewBindings) != len(o.restore.AffectedSlots) || (o.restore.LinkedMigrationID != nil) != paired {
			return nil, z, invalid("rebind scope or parent")
		}
		dbInc := map[dbKey]string{}
		incDB := map[string]dbKey{}
		for i, b := range x.NewBindings {
			if b.Key != o.restore.AffectedSlots[i] || !validBinding(b.Binding) || !validAnchor(b.Anchor) {
				return nil, z, invalid("new binding or anchor")
			}
			inc := b.Binding.IncarnationID
			d := dbOf(b.Key)
			if _, ok := m.incarnations[inc]; ok {
				return nil, z, invalid("historical incarnation reuse")
			}
			if old, ok := dbInc[d]; ok && old != inc {
				return nil, z, invalid("cross-slot incarnation mismatch")
			}
			if old, ok := incDB[inc]; ok && old != d {
				return nil, z, invalid("cross-database incarnation reuse")
			}
			dbInc[d] = inc
			incDB[inc] = d
		}
	default:
		return nil, z, invalid("restore disposition")
	}
	return o, x, nil
}
func (m *model) finishRestore(v event, o *op, x RestoreResolution) {
	if x.Disposition == "REBOUND" {
		for _, b := range x.NewBindings {
			m.p.Slots[m.byKey[b.Key]].Current = b.Binding
			m.anchors[b.Key] = *b.Anchor
			m.incarnations[b.Binding.IncarnationID] = dbOf(b.Key)
		}
	}
	o.closed = true
	m.p.Histories[o.idx].Resolution = append(json.RawMessage(nil), v.body...)
	m.restore = ""
}
func (m *model) closeRestore(v event, paired bool) error {
	o, x, e := m.checkRestoreTerminal(v, paired)
	if e != nil {
		return e
	}
	m.finishRestore(v, o, x)
	return nil
}
func (m *model) pair(a, b event) error {
	ro, rr, e := m.checkRestoreTerminal(a, true)
	if e != nil {
		return e
	}
	mo, mr, e := m.checkMigrationTerminal(b, true)
	if e != nil {
		return e
	}
	if ro.restore.LinkedMigrationID == nil || *ro.restore.LinkedMigrationID != b.id || rr.AuthorizationDigest != mr.Final.AuthorizationDigest {
		return invalid("paired parent or authorization")
	}
	m.finishMigration(b, mo, mr)
	m.finishRestore(a, ro, rr)
	return nil
}
func (m *model) restart(v event) error {
	x, e := typed[RestartReview](v.body)
	if e != nil || x.Revision != v.revision || x.ID != v.id || !id(x.ID) || m.records[x.ID] || !digests(x.ContinuityDigest, x.SettlementDigest, x.AuthorizationDigest) {
		return invalid("restart review facts")
	}
	if len(m.p.RestartReviews) >= MaxReviews {
		return ErrCapacity
	}
	m.records[x.ID] = true
	m.p.RestartReviews = append(m.p.RestartReviews, x)
	if m.migration != "" {
		m.ops[m.migration].latch = true
	}
	if m.restore != "" {
		m.ops[m.restore].latch = true
	}
	return nil
}
