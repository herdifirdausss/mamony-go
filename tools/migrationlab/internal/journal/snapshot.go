// Package journal models bounded synthetic administrative journal values.
// It provides neither durable storage nor authorization or database evidence.
package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const (
	Format       = "mamony-migration-journal-v1"
	MaxEnvelope  = 1 << 20
	MaxSlots     = 8
	MaxHistories = 32
	MaxHistory   = 16 << 10
	MaxOverhead  = 64 << 10
	MaxReviews   = 8
	MaxRevision  = 200
	MaxVersion   = 2147483647
)

var (
	ErrInvalidSnapshot   = errors.New("INVALID_SNAPSHOT")
	ErrStaleSource       = errors.New("STALE_SOURCE")
	ErrInvalidRequest    = errors.New("INVALID_REQUEST")
	ErrAuthorityScope    = errors.New("AUTHORITY_SCOPE")
	ErrInvalidTransition = errors.New("INVALID_TRANSITION")
	ErrCapacity          = errors.New("CAPACITY")
)

type Key struct {
	EnvironmentID string `json:"environment_id"`
	DatabaseID    string `json:"database_id"`
	Capability    string `json:"capability"`
}
type Binding struct {
	IncarnationID string `json:"incarnation_id"`
	ReceiptDigest string `json:"receipt_digest"`
}
type Slot struct {
	Key       Key     `json:"key"`
	ClusterID string  `json:"cluster_id"`
	Initial   Binding `json:"initial_binding"`
	Current   Binding `json:"current_binding"`
}
type Anchor struct {
	Prefix        uint32 `json:"prefix"`
	HistoryDigest string `json:"history_digest"`
}
type BoundAnchor struct {
	Key     Key     `json:"key"`
	Binding Binding `json:"binding"`
	Anchor  *Anchor `json:"anchor"`
}
type MigrationOpening struct {
	Slot                     Key     `json:"slot"`
	Binding                  Binding `json:"binding"`
	AuthorizationDigest      string  `json:"authorization_digest"`
	ManifestDigest           string  `json:"manifest_digest"`
	RequestedVersion         uint32  `json:"requested_version"`
	StartingPrefix           uint32  `json:"starting_prefix"`
	StartingHistoryDigest    string  `json:"starting_history_digest"`
	InitialObservationDigest string  `json:"initial_observation_digest"`
}
type MigrationObservation struct {
	Revision            uint64  `json:"revision"`
	Kind                string  `json:"kind"`
	Phase               string  `json:"phase"`
	AttemptedVersion    *uint32 `json:"attempted_version"`
	AttemptedEffect     string  `json:"attempted_effect"`
	VerifiedPrefix      uint32  `json:"verified_prefix"`
	HistoryDigest       string  `json:"history_digest"`
	OperationEffect     string  `json:"operation_effect"`
	Cause               string  `json:"cause"`
	Verification        string  `json:"verification"`
	Cleanup             string  `json:"cleanup"`
	EvidenceDigest      string  `json:"evidence_digest"`
	AuthorizationDigest string  `json:"authorization_digest"`
}
type MigrationResolution struct {
	Revision    uint64               `json:"revision"`
	Disposition string               `json:"disposition"`
	Final       MigrationObservation `json:"final_observation"`
}
type RestoreOpening struct {
	EnvironmentID            string        `json:"environment_id"`
	ClusterID                string        `json:"cluster_id"`
	AffectedSlots            []Key         `json:"affected_slots"`
	PriorBindings            []BoundAnchor `json:"prior_bindings"`
	RecoveryDigest           string        `json:"recovery_digest"`
	AuthorizationDigest      string        `json:"authorization_digest"`
	SettlementEvidenceDigest string        `json:"settlement_evidence_digest"`
	LinkedMigrationID        *string       `json:"linked_migration_id"`
}
type RestoreObservation struct {
	Revision            uint64 `json:"revision"`
	Phase               string `json:"phase"`
	Mutation            string `json:"mutation"`
	Settlement          string `json:"settlement"`
	EvidenceDigest      string `json:"evidence_digest"`
	AuthorizationDigest string `json:"authorization_digest"`
}
type RestoreResolution struct {
	Revision            uint64        `json:"revision"`
	Disposition         string        `json:"disposition"`
	NewBindings         []BoundAnchor `json:"new_bindings"`
	Mutation            string        `json:"mutation"`
	Verification        string        `json:"verification"`
	Settlement          string        `json:"settlement"`
	EvidenceDigest      string        `json:"evidence_digest"`
	AuthorizationDigest string        `json:"authorization_digest"`
}
type RestartReview struct {
	ID                  string `json:"id"`
	Revision            uint64 `json:"revision"`
	ContinuityDigest    string `json:"continuity_digest"`
	SettlementDigest    string `json:"settlement_digest"`
	AuthorizationDigest string `json:"authorization_digest"`
}

// History's tagged union uses raw JSON solely as a wire representation. Every
// member is decoded to a fixed typed record and checked during structural replay.
type History struct {
	ID           string            `json:"id"`
	OpenRevision uint64            `json:"open_revision"`
	Kind         string            `json:"kind"`
	Opening      json.RawMessage   `json:"opening"`
	Observations []json.RawMessage `json:"observations"`
	Resolution   json.RawMessage   `json:"resolution"`
}
type Payload struct {
	JournalID            string          `json:"journal_id"`
	ControllerID         string          `json:"controller_id"`
	StorageBindingDigest string          `json:"storage_binding_digest"`
	Revision             uint64          `json:"revision"`
	Slots                []Slot          `json:"slots"`
	Histories            []History       `json:"histories"`
	RestartReviews       []RestartReview `json:"restart_reviews"`
}
type Snapshot struct {
	Format  string  `json:"format"`
	Payload Payload `json:"payload"`
	Digest  string  `json:"digest"`
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func hexToken(s string, n int) bool {
	if len(s) != n {
		return false
	}
	nonzero := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
		nonzero = nonzero || c != '0'
	}
	return nonzero
}
func id(s string) bool     { return hexToken(s, 32) }
func digest(s string) bool { return hexToken(s, 64) }
func digests(values ...string) bool {
	for _, v := range values {
		if !digest(v) {
			return false
		}
	}
	return true
}
func validKey(k Key) bool {
	return id(k.EnvironmentID) && id(k.DatabaseID) && oneOf(k.Capability, "PLATFORM", "IDENTITY", "LEDGER")
}
func validBinding(b Binding) bool { return id(b.IncarnationID) && digest(b.ReceiptDigest) }
func validAnchor(a *Anchor) bool {
	return a != nil && a.Prefix <= MaxVersion && digest(a.HistoryDigest)
}
func keyLess(a, b Key) bool {
	if a.EnvironmentID != b.EnvironmentID {
		return a.EnvironmentID < b.EnvironmentID
	}
	if a.DatabaseID != b.DatabaseID {
		return a.DatabaseID < b.DatabaseID
	}
	return a.Capability < b.Capability
}
func validKeys(v []Key) bool {
	if len(v) < 1 || len(v) > MaxSlots {
		return false
	}
	for i, k := range v {
		if !validKey(k) || i > 0 && !keyLess(v[i-1], k) {
			return false
		}
	}
	return true
}
func sameKeys(a, b []Key) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func equalAnchor(a, b *Anchor) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func sumDigest(domain string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{'\n'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
func raw(v any) (json.RawMessage, error) { return json.Marshal(v) }
func isNull(b []byte) bool               { return bytes.Equal(b, []byte("null")) }

// Check size and depth before decoding. All canonical tokens are unescaped ASCII
// and whitespace is forbidden outside and inside the fixed-token strings.
func wireGuard(b []byte, max int) bool {
	if len(b) == 0 || len(b) > max {
		return false
	}
	depth := 0
	quoted := false
	for _, c := range b {
		if c < 0x21 || c > 0x7e || c == '\\' {
			return false
		}
		if c == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch c {
		case '{', '[':
			depth++
			if depth > 16 {
				return false
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && !quoted
}

// Decode into a fixed shape and require identical canonical re-encoding. This
// rejects duplicates, case aliases, missing members and noncanonical tokens even
// when encoding/json would otherwise accept them. Unknown fields are also
// explicitly disallowed. Semantic validation is separate.
func typed[T any](b []byte) (T, error) {
	var v T
	if !wireGuard(b, MaxEnvelope) {
		return v, ErrInvalidSnapshot
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, ErrInvalidSnapshot
	}
	canonical, err := json.Marshal(v)
	if err != nil || !bytes.Equal(canonical, b) {
		return v, ErrInvalidSnapshot
	}
	return v, nil
}
func envelope(p Payload) ([]byte, error) {
	b, e := json.Marshal(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(Snapshot{Format: Format, Payload: p, Digest: sumDigest(Format, b)})
}

// Decode checks the whole wire value and replays all retained transitions. It
// does not establish whether the bytes are the current durable snapshot.
func Decode(b []byte) (Snapshot, error) {
	s, e := typed[Snapshot](b)
	if e != nil || s.Format != Format || !digest(s.Digest) {
		return Snapshot{}, ErrInvalidSnapshot
	}
	p, e := json.Marshal(s.Payload)
	if e != nil || sumDigest(Format, p) != s.Digest {
		return Snapshot{}, ErrInvalidSnapshot
	}
	if _, e = replay(s.Payload); e != nil {
		return Snapshot{}, ErrInvalidSnapshot
	}
	return s, nil
}

// Encode refuses inconsistent values rather than normalizing or repairing them.
func Encode(p Payload) ([]byte, error) {
	if _, e := replay(p); e != nil {
		return nil, ErrInvalidSnapshot
	}
	b, e := envelope(p)
	if e != nil {
		return nil, ErrInvalidSnapshot
	}
	if _, e = Decode(b); e != nil {
		return nil, e
	}
	return b, nil
}

type dbKey struct{ env, db string }

func dbOf(k Key) dbKey { return dbKey{k.EnvironmentID, k.DatabaseID} }

type op struct {
	idx            int
	migration      *MigrationOpening
	restore        *RestoreOpening
	last           *MigrationObservation
	held           *MigrationObservation
	restoreChanged bool
	ordinary       int
	hold           bool
	latch          bool
	linked         bool
	closed         bool
}
type model struct {
	p            Payload
	byKey        map[Key]int
	anchors      map[Key]Anchor
	incarnations map[string]dbKey
	records      map[string]bool
	ops          map[string]*op
	migration    string
	restore      string
}

func genesis(p Payload) (*model, error) {
	if !id(p.JournalID) || !id(p.ControllerID) || !digest(p.StorageBindingDigest) || len(p.Slots) < 1 || len(p.Slots) > MaxSlots || p.Histories == nil || p.RestartReviews == nil {
		return nil, ErrInvalidSnapshot
	}
	m := &model{p: Payload{JournalID: p.JournalID, ControllerID: p.ControllerID, StorageBindingDigest: p.StorageBindingDigest, Slots: append([]Slot(nil), p.Slots...), Histories: []History{}, RestartReviews: []RestartReview{}}, byKey: map[Key]int{}, anchors: map[Key]Anchor{}, incarnations: map[string]dbKey{}, records: map[string]bool{}, ops: map[string]*op{}}
	clusters := map[dbKey]string{}
	incs := map[dbKey]string{}
	for i, s := range p.Slots {
		if !validKey(s.Key) || !id(s.ClusterID) || !validBinding(s.Initial) || !validBinding(s.Current) || i > 0 && !keyLess(p.Slots[i-1].Key, s.Key) {
			return nil, ErrInvalidSnapshot
		}
		d := dbOf(s.Key)
		if c, ok := clusters[d]; ok && (c != s.ClusterID || incs[d] != s.Initial.IncarnationID) {
			return nil, ErrInvalidSnapshot
		}
		clusters[d] = s.ClusterID
		incs[d] = s.Initial.IncarnationID
		if old, ok := m.incarnations[s.Initial.IncarnationID]; ok && old != d {
			return nil, ErrInvalidSnapshot
		}
		m.incarnations[s.Initial.IncarnationID] = d
		m.p.Slots[i].Current = s.Initial
		m.byKey[s.Key] = i
	}
	return m, nil
}
func (m *model) anchor(k Key) *Anchor {
	if a, ok := m.anchors[k]; ok {
		return &a
	}
	return nil
}
func (m *model) scope(env, cluster string) []Key {
	ks := []Key{}
	for _, s := range m.p.Slots {
		if s.Key.EnvironmentID == env && s.ClusterID == cluster {
			ks = append(ks, s.Key)
		}
	}
	return ks
}
func (m *model) roster() []Key {
	ks := make([]Key, 0, len(m.p.Slots))
	for _, s := range m.p.Slots {
		ks = append(ks, s.Key)
	}
	return ks
}
func (m *model) blocked() bool { return m.migration != "" || m.restore != "" }
func (m *model) reservation() int {
	if m.migration != "" && !m.ops[m.migration].linked {
		return 1
	}
	return 0
}
func accounting(count, unused, resolvedBytes, open, overhead, actual int) error {
	if count < 0 || count > MaxHistories || unused < 0 || unused > 1 || count+unused > MaxHistories || open < 0 || open > count || resolvedBytes < 0 || resolvedBytes > MaxHistories*MaxHistory || overhead < 0 || overhead > MaxOverhead || actual < 0 || actual > MaxEnvelope {
		return ErrCapacity
	}
	// All operands bounded before arithmetic, including on 32-bit Go.
	if (unused > 0 && open == 0) || resolvedBytes > (count-open)*MaxHistory || actual < resolvedBytes+overhead || actual > resolvedBytes+open*MaxHistory+overhead || resolvedBytes+MaxHistory*(open+unused)+MaxOverhead > MaxEnvelope {
		return ErrCapacity
	}
	return nil
}
func historySize(n int) error {
	if n < 0 || n > MaxHistory {
		return ErrCapacity
	}
	return nil
}
func (m *model) capacity() error {
	if len(m.p.RestartReviews) > MaxReviews || m.p.Revision > MaxRevision {
		return ErrCapacity
	}
	resolved, open, total := 0, 0, 0
	for _, h := range m.p.Histories {
		b, e := json.Marshal(h)
		if e != nil || historySize(len(b)) != nil {
			return ErrCapacity
		}
		total += len(b)
		if isNull(h.Resolution) {
			open++
		} else {
			resolved += len(b)
		}
	}
	b, e := envelope(m.p)
	if e != nil {
		return ErrCapacity
	}
	return accounting(len(m.p.Histories), m.reservation(), resolved, open, len(b)-total, len(b))
}

type event struct {
	revision         uint64
	action, id, kind string
	body             json.RawMessage
}

func historyEvents(h History) ([]event, error) {
	if !id(h.ID) || h.OpenRevision == 0 || h.OpenRevision > MaxRevision || h.Observations == nil || len(h.Observations) > 4 {
		return nil, ErrInvalidSnapshot
	}
	var ev []event
	switch h.Kind {
	case "MIGRATION":
		if _, e := typed[MigrationOpening](h.Opening); e != nil {
			return nil, e
		}
		ev = append(ev, event{h.OpenRevision, "OPEN_MIGRATION", h.ID, h.Kind, h.Opening})
		last := h.OpenRevision
		for _, b := range h.Observations {
			o, e := typed[MigrationObservation](b)
			if e != nil || o.Revision <= last || o.Revision > MaxRevision {
				return nil, ErrInvalidSnapshot
			}
			last = o.Revision
			ev = append(ev, event{o.Revision, "APPEND_MIGRATION", h.ID, h.Kind, b})
		}
		if !isNull(h.Resolution) {
			r, e := typed[MigrationResolution](h.Resolution)
			if e != nil || r.Revision <= last || r.Revision > MaxRevision {
				return nil, ErrInvalidSnapshot
			}
			action := ""
			switch r.Disposition {
			case "COMPLETED":
				action = "COMPLETE_MIGRATION"
			case "STOPPED":
				action = "STOP_MIGRATION"
			case "RECONCILED":
				action = "RECONCILE_MIGRATION"
			default:
				return nil, ErrInvalidSnapshot
			}
			ev = append(ev, event{r.Revision, action, h.ID, h.Kind, h.Resolution})
		}
	case "RESTORE":
		if _, e := typed[RestoreOpening](h.Opening); e != nil {
			return nil, e
		}
		ev = append(ev, event{h.OpenRevision, "OPEN_RESTORE", h.ID, h.Kind, h.Opening})
		last := h.OpenRevision
		for _, b := range h.Observations {
			o, e := typed[RestoreObservation](b)
			if e != nil || o.Revision <= last || o.Revision > MaxRevision {
				return nil, ErrInvalidSnapshot
			}
			last = o.Revision
			ev = append(ev, event{o.Revision, "APPEND_RESTORE", h.ID, h.Kind, b})
		}
		if !isNull(h.Resolution) {
			r, e := typed[RestoreResolution](h.Resolution)
			if e != nil || r.Revision <= last || r.Revision > MaxRevision {
				return nil, ErrInvalidSnapshot
			}
			action := ""
			switch r.Disposition {
			case "REBOUND":
				action = "REBIND_RESTORE"
			case "ABORTED_NO_MUTATION":
				action = "ABORT_RESTORE"
			default:
				return nil, ErrInvalidSnapshot
			}
			ev = append(ev, event{r.Revision, action, h.ID, h.Kind, h.Resolution})
		}
	default:
		return nil, ErrInvalidSnapshot
	}
	return ev, nil
}
func replay(p Payload) (*model, error) {
	if p.Revision > MaxRevision || len(p.Histories) > MaxHistories || len(p.RestartReviews) > MaxReviews {
		return nil, ErrInvalidSnapshot
	}
	m, e := genesis(p)
	if e != nil {
		return nil, e
	}
	events := make([]event, 0, MaxRevision)
	var last uint64
	for _, h := range p.Histories {
		if h.OpenRevision <= last {
			return nil, ErrInvalidSnapshot
		}
		last = h.OpenRevision
		ev, e := historyEvents(h)
		if e != nil {
			return nil, e
		}
		events = append(events, ev...)
	}
	last = 0
	for _, r := range p.RestartReviews {
		if r.Revision <= last || r.Revision > MaxRevision {
			return nil, ErrInvalidSnapshot
		}
		last = r.Revision
		b, e := raw(r)
		if e != nil {
			return nil, e
		}
		events = append(events, event{r.Revision, "RECORD_RESTART", r.ID, "", b})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].revision < events[j].revision })
	for i := 0; i < len(events); {
		j := i + 1
		for j < len(events) && events[j].revision == events[i].revision {
			j++
		}
		if events[i].revision != m.p.Revision+1 || j-i > 2 {
			return nil, ErrInvalidSnapshot
		}
		if e = m.apply(events[i:j]); e != nil {
			return nil, e
		}
		i = j
	}
	if m.p.Revision != p.Revision {
		return nil, ErrInvalidSnapshot
	}
	for i, s := range p.Slots {
		if m.p.Slots[i].Current != s.Current {
			return nil, ErrInvalidSnapshot
		}
	}
	// Compare the reconstructed complete payload, including immutable history and
	// array order, to the supplied value. No silent repair/normalization.
	a, e := json.Marshal(m.p)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(p)
	if e != nil || !bytes.Equal(a, b) {
		return nil, ErrInvalidSnapshot
	}
	if e = m.capacity(); e != nil {
		return nil, e
	}
	return m, nil
}
func invalid(detail string) error { return fmt.Errorf("%w: %s", ErrInvalidTransition, detail) }
