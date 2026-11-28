package journal

// This file coordinates a deterministic storage contract. It contains no real
// adapter, evidence issuer, filesystem access, or controller entrypoint.
import (
	"bytes"
	"context"
	"errors"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var (
	errStoreUncertain   = errors.New("SESSION_UNCERTAIN")
	errStoreClosed      = errors.New("SESSION_CLOSED")
	errStoreInvalid     = errors.New("INVALID_SESSION")
	errStoreBusy        = errors.New("BUSY")
	errStoreUnavailable = errors.New("STORAGE_UNAVAILABLE")
	errStoreIdentity    = errors.New("IDENTITY_MISMATCH")
	errStoreUnsupported = errors.New("UNSUPPORTED_STORAGE")
	errStoreReview      = errors.New("REVIEW_REQUIRED")
	errStoreRequest     = errors.New("INVALID_STORAGE_REQUEST")
	errStoreAuthority   = errors.New("AUTHORITY_UNVERIFIED")
	errStoreGateUnknown = errors.New("AUTHORITY_CONTINUITY_UNKNOWN")
	errStoreBudget      = errors.New("BUDGET_EXPIRED")
	errStoreIO          = errors.New("LOCAL_STORAGE_FAILURE")
)

const (
	storeLockName   = "journal.lock"
	storeSourceName = "journal.json"
	storeStageName  = "journal.tmp"
	mountInfoLimit  = 1 << 20
	fdInfoLimit     = 4 << 10
	storePathLimit  = 4096
)

type storeState uint8

const (
	storeInvalid storeState = iota
	storeReviewRequired
	storeReviewed
	storePoisoned
	storeClosed
)

type storeDisposition uint8

const (
	storeUnassessed storeDisposition = iota
	storeNotPublished
	storePublished
	storeUncertain
)

type storePhase string

const (
	phaseValidate      storePhase = "VALIDATE"
	phaseApply         storePhase = "APPLY"
	phaseStageCreate   storePhase = "STAGE_CREATE"
	phaseStageWrite    storePhase = "STAGE_WRITE"
	phaseFileSync      storePhase = "FILE_SYNC"
	phaseStageClose    storePhase = "STAGE_CLOSE"
	phasePreReplace    storePhase = "PRE_REPLACE_CHECK"
	phaseReplace       storePhase = "REPLACE"
	phaseDirectorySync storePhase = "DIRECTORY_SYNC"
	phaseReadback      storePhase = "READBACK"
	phaseFinal         storePhase = "FINAL_CHECK"
	phaseCleanup       storePhase = "CLEANUP"
	phaseDone          storePhase = "DONE"
)

type storeSourceRef struct {
	Revision uint64
	Digest   string
}

func sourceRef(s Snapshot) storeSourceRef { return storeSourceRef{s.Payload.Revision, s.Digest} }
func (s storeSourceRef) valid() bool      { return s.Revision <= MaxRevision && digest(s.Digest) }

type publicationResult struct {
	disposition  storeDisposition
	phase        storePhase
	sourceRef    storeSourceRef // zero if no independently verified source
	publishedRef storeSourceRef // set only on published
}
type storageFailure struct {
	cause     error
	phase     storePhase
	cleanup   error
	uncertain bool
}

func (e *storageFailure) Error() string {
	if e.uncertain {
		return "SESSION_UNCERTAIN"
	}
	return storeErrorCode(e.cause)
}
func storeErrorCode(e error) string {
	if e == nil {
		return ""
	}
	for _, x := range []error{errStoreUncertain, errStoreClosed, errStoreInvalid, errStoreBusy, errStoreUnavailable, errStoreIdentity, errStoreUnsupported, errStoreReview, errStoreRequest, errStoreAuthority, errStoreGateUnknown, errStoreBudget, errStoreIO, ErrInvalidSnapshot, ErrStaleSource, ErrInvalidRequest, ErrAuthorityScope, ErrInvalidTransition, ErrCapacity} {
		if errors.Is(e, x) {
			return x.Error()
		}
	}
	return "STORAGE_UNAVAILABLE"
}
func (e *storageFailure) Unwrap() error {
	if e.uncertain {
		return errStoreUncertain
	}
	return e.cause
}

type storageDevice struct{ Major, Minor uint32 }
type storageObject struct {
	Device storageDevice
	Inode  uint64
}
type storageMount struct {
	ID                uint64
	Device            storageDevice
	Root, Point, Type string
}
type expectedProvision struct {
	EnvironmentID, ReceiptID, ReceiptDigest, JournalID, ControllerID, BindingDigest string
	HostVolumeDigest, BootID                                                        string
	Namespace, Directory, Lock                                                      storageObject
	UID, GID                                                                        uint32
	ControlPath                                                                     string
	Mount                                                                           storageMount
	PolicyDigest, ProvenanceDigest                                                  string
}
type currentnessReference struct {
	SessionID, JournalID, BindingDigest, BudgetID string
	Source                                        storeSourceRef
	HistoryEvidence, ReviewReference              string
	BudgetDeadline                                uint64 // externally observed monotonic deadline; zero is invalid
}
type storageMeta struct {
	Object         storageObject
	MountID        uint64
	Kind           string
	Mode, UID, GID uint32
	Links, Size    uint64
	FDInfo         []byte
	FDComplete     bool
}
type storageObservation struct {
	Custody                           string
	BootID                            string
	Namespace                         storageObject
	UID, GID                          uint32
	MountInfo                         []byte
	MountComplete                     bool
	FSMagic                           uint64
	Directory, Lock, Journal, Stage   storageMeta
	PathDirectory, PathLock           storageObject
	PathDirectoryMount, PathLockMount uint64
	Entries                           []string
	Ref                               observationRef
}
type observationRef struct {
	Session string
	Serial  uint64
	Object  storageObject
}
type storageRead struct {
	Ref      observationRef
	Bytes    []byte
	Complete bool
	After    storageObservation
}

type adapterStatus uint8

const (
	adapterUnknown adapterStatus = iota
	adapterOK
	adapterLocalFailure
	adapterUnavailable
	adapterBusy
)

type custodyResult struct {
	Status  adapterStatus
	Custody string
	MayHold bool
	Settled bool
}
type stageResult struct {
	Status adapterStatus
	Stage  storageMeta
	Owned  bool
	Absent bool
}
type phaseResult struct {
	Status  adapterStatus
	Settled bool
	Object  storageObject
	Count   uint64
}

// Each operation is restricted to one session and one fixed provision. A real
// implementation, descriptor lifetime and OS guarantees are deferred. Unknown
// results are deliberately the zero values. No method supplies a verdict of
// durable publication or unchanged authoritative state.
type storageAdapter interface {
	inspectProvision(context.Context, expectedProvision) (storageObservation, adapterStatus)
	acquire(context.Context, string) custodyResult
	inspectSource(context.Context, string) (storageObservation, adapterStatus)
	readSource(context.Context, string, observationRef, int) (storageRead, adapterStatus)
	createStage(context.Context, string) stageResult
	writeStage(context.Context, string, storageObject, []byte) phaseResult
	syncStage(context.Context, string, storageObject) phaseResult
	closeStage(context.Context, string, storageObject) phaseResult
	replace(context.Context, string, storageObject, storeSourceRef) phaseResult
	syncDirectory(context.Context, string) phaseResult
	inspectPublished(context.Context, string) (storageObservation, adapterStatus)
	readPublished(context.Context, string, observationRef, int) (storageRead, adapterStatus)
	removeOwnedStage(context.Context, string, storageObject) phaseResult
	retire(context.Context, string) phaseResult
}

type storageEvidence struct {
	Current                                                 currentnessReference
	Assertion                                               Assertion
	ContinuityDigest, SettlementDigest, AuthorizationDigest string
	// External evidence reference for this exact invocation, not a local claim of
	// authenticity. Worker outcome facts are supplied by the independent gate.
	EvidenceDigest string
}
type reviewEvidence struct{ storageEvidence }
type actionEvidence struct{ storageEvidence }
type closeEvidence struct {
	Current                               currentnessReference
	SettlementDigest, AuthorizationDigest string
}
type gateClaim struct {
	Purpose     string
	Provision   expectedProvision
	Current     currentnessReference
	Evidence    storageEvidence
	Close       closeEvidence
	Custody     string
	PriorState  storeState
	Uncertainty storageUncertainty
}

// Bounded known facts only, not an independently durable record.
type storageUncertainty struct {
	Phase                storePhase
	Cause, Cleanup       string
	Stage                storageObject // reported identity, not proof of ownership
	StageClaimedOwned    bool
	StageQualified       bool
	AcquisitionUncertain bool
	Candidate            storeSourceRef
	CustodyUnknown       bool // no validated custody binding; external settlement only
	RetirementCause      string
	ReplaceAttempted     bool
}
type gateGrant struct {
	ID              string
	Claim           gateClaim
	WorkerEffect    string
	WorkerOperation string
}
type gateStatus uint8

const (
	gateUnknown gateStatus = iota
	gateAllowed
	gateRefused
	gateExpired
)

type gateFacts struct {
	Phase       storePhase
	Observation storageObservation
	Result      phaseResult
	Stage       storageObject
	Source      storeSourceRef
	Candidate   storeSourceRef
	Cleanup     bool
}
type storageEvidenceGate interface {
	authorize(context.Context, gateClaim) (gateGrant, gateStatus)
	confirm(context.Context, gateGrant, gateFacts) gateStatus
}

// Parsing works only on supplied observation bytes. The caller must supply an
// independent complete-read observation; this is not a procfs reader.
func boundedLines(b []byte, complete bool, limit, count, lineLimit int) ([]string, bool) {
	if !complete || len(b) == 0 || len(b) > limit || bytes.Count(b, []byte{'\n'}) > count || b[len(b)-1] != '\n' || bytes.IndexByte(b, 0) >= 0 {
		return nil, false
	}
	lines := strings.Split(string(b[:len(b)-1]), "\n")
	if len(lines) > count {
		return nil, false
	}
	for _, l := range lines {
		if len(l) == 0 || len(l) > lineLimit {
			return nil, false
		}
	}
	return lines, true
}
func canonicalNumber(s string, bits int) (uint64, bool) {
	if len(s) == 0 || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, e := strconv.ParseUint(s, 10, bits)
	return n, e == nil
}
func mountEscape(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			if s[i] <= 32 || s[i] == 127 {
				return "", false
			}
			b.WriteByte(s[i])
			continue
		}
		if i+3 >= len(s) {
			return "", false
		}
		switch s[i+1 : i+4] {
		case "040":
			b.WriteByte(' ')
		case "011":
			b.WriteByte('\t')
		case "012":
			b.WriteByte('\n')
		case "134":
			b.WriteByte('\\')
		default:
			return "", false
		}
		i += 3
	}
	return b.String(), true
}
func parseDevice(s string) (storageDevice, bool) {
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		return storageDevice{}, false
	}
	x, xok := canonicalNumber(a, 32)
	y, yok := canonicalNumber(b, 32)
	if !xok || !yok {
		return storageDevice{}, false
	}
	if x > 0xffffffff {
		return storageDevice{}, false
	}
	if y > 0xffffffff {
		return storageDevice{}, false
	}
	return storageDevice{uint32(x), uint32(y)}, true
}
func fdIdentity(b []byte, complete bool) (uint64, uint64, bool) {
	lines, ok := boundedLines(b, complete, fdInfoLimit, 64, 1024)
	if !ok {
		return 0, 0, false
	}
	var mnt, ino uint64
	seenMount, seenInode := false, false
	for _, l := range lines {
		k, v, ok := strings.Cut(l, ":")
		if !ok || k == "" {
			return 0, 0, false
		}
		switch k {
		case "mnt_id":
			if seenMount {
				return 0, 0, false
			}
			seenMount = true
			mnt, ok = canonicalNumber(strings.TrimLeft(v, " \t"), 64)
			if !ok || mnt == 0 {
				return 0, 0, false
			}
		case "ino":
			if seenInode {
				return 0, 0, false
			}
			seenInode = true
			ino, ok = canonicalNumber(strings.TrimLeft(v, " \t"), 64)
			if !ok || ino == 0 {
				return 0, 0, false
			}
		}
	}
	return mnt, ino, seenMount
}
func fdMount(b []byte, complete bool) (uint64, bool) {
	mnt, _, ok := fdIdentity(b, complete)
	return mnt, ok
}
func bootID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return s != "00000000-0000-0000-0000-000000000000"
}
func validStorePath(s string) bool {
	return len(s) > 0 && len(s) <= storePathLimit && s[0] == '/' && path.Clean(s) == s && !strings.ContainsRune(s, 0)
}
func validObject(o storageObject) bool { return o.Inode > 0 }
func validProvision(p expectedProvision, c currentnessReference) bool {
	return id(p.EnvironmentID) && id(p.ReceiptID) && id(p.JournalID) && id(p.ControllerID) && digests(p.ReceiptDigest, p.BindingDigest, p.HostVolumeDigest, p.PolicyDigest, p.ProvenanceDigest) && bootID(p.BootID) && validObject(p.Namespace) && validObject(p.Directory) && validObject(p.Lock) && p.Directory != p.Lock && p.Directory.Device == p.Mount.Device && p.Lock.Device == p.Mount.Device && p.Mount.ID > 0 && p.Mount.Root == "/" && p.Mount.Type == "ext4" && validStorePath(p.ControlPath) && validStorePath(p.Mount.Point) && (p.Mount.Point == "/" || strings.HasPrefix(p.ControlPath, p.Mount.Point+"/")) && validCurrent(c, p)
}
func validCurrent(c currentnessReference, p expectedProvision) bool {
	return id(c.SessionID) && id(c.BudgetID) && c.BudgetDeadline > 0 && c.JournalID == p.JournalID && c.BindingDigest == p.BindingDigest && c.Source.valid() && digests(c.HistoryEvidence, c.ReviewReference)
}
func validMountToken(s string) bool {
	if s == "" {
		return false
	}
	_, ok := mountEscape(s)
	return ok && !strings.ContainsAny(s, ",=")
}

type mountOption struct {
	value    string
	assigned bool
}

func options(s string) (map[string]mountOption, bool) {
	m := map[string]mountOption{}
	for _, v := range strings.Split(s, ",") {
		k, x, assigned := strings.Cut(v, "=")
		if !validMountToken(k) {
			return nil, false
		}
		if assigned {
			if _, ok := mountEscape(x); !ok {
				return nil, false
			}
		}
		if _, ok := m[k]; ok {
			return nil, false
		}
		m[k] = mountOption{x, assigned}
	}
	return m, true
}
func qualifiedOptions(s string) bool {
	m, ok := options(s)
	if !ok {
		return false
	}
	rw, yes := m["rw"]
	if !yes || rw.assigned {
		return false
	}
	for _, k := range []string{"ro", "nobarrier", "noload", "norecovery"} {
		if _, yes := m[k]; yes {
			return false
		}
	}
	if v, yes := m["data"]; yes && (!v.assigned || v.value != "ordered") {
		return false
	}
	if v, yes := m["barrier"]; yes && (!v.assigned || v.value != "1") {
		return false
	}
	return true
}
func matchMount(b []byte, complete bool, p storageMount) error {
	lines, ok := boundedLines(b, complete, mountInfoLimit, 4096, 16<<10)
	if !ok {
		return errStoreIdentity
	}
	seen := map[uint64]bool{}
	found := false
	for _, l := range lines {
		f := strings.Split(l, " ")
		if len(f) < 10 {
			return errStoreIdentity
		}
		for _, s := range f {
			if s == "" {
				return errStoreIdentity
			}
		}
		n, ok := canonicalNumber(f[0], 64)
		_, parentOK := canonicalNumber(f[1], 64)
		dev, devOK := parseDevice(f[2])
		root, rootOK := mountEscape(f[3])
		point, pointOK := mountEscape(f[4])
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if !ok || n == 0 || !parentOK || !devOK || !rootOK || !pointOK || !strings.HasPrefix(root, "/") || !strings.HasPrefix(point, "/") || seen[n] || sep < 6 || len(f) != sep+4 {
			return errStoreIdentity
		}
		seen[n] = true
		if _, ok = mountEscape(f[sep+2]); !ok || !validMountToken(f[sep+1]) {
			return errStoreIdentity
		}
		if _, ok = options(f[5]); !ok {
			return errStoreIdentity
		}
		if _, ok = options(f[sep+3]); !ok {
			return errStoreIdentity
		}
		if n != p.ID {
			continue
		}
		found = true
		if dev != p.Device || root != p.Root || point != p.Point {
			return errStoreIdentity
		}
		if f[sep+1] != "ext4" || f[sep+1] != p.Type || sep != 6 || !qualifiedOptions(f[5]) || !qualifiedOptions(f[sep+3]) {
			return errStoreUnsupported
		}
	}
	if !found {
		return errStoreIdentity
	}
	return nil
}
func metaQualified(m storageMeta, p expectedProvision, kind string, mode uint32) bool {
	n, ino, ok := fdIdentity(m.FDInfo, m.FDComplete)
	return ok && (ino == 0 || ino == m.Object.Inode) && n == p.Mount.ID && m.MountID == n && m.Object.Device == p.Mount.Device && validObject(m.Object) && m.Kind == kind && m.Mode == mode && m.UID == p.UID && m.GID == p.GID && (kind == "directory" || m.Links == 1)
}
func validateBase(o storageObservation, p expectedProvision, custody string) error {
	if o.Custody != custody || o.BootID != p.BootID || o.Namespace != p.Namespace || o.UID != p.UID || o.GID != p.GID || o.PathDirectory != p.Directory || o.PathLock != p.Lock || o.PathDirectoryMount != p.Mount.ID || o.PathLockMount != p.Mount.ID || o.Directory.Object != p.Directory || o.Lock.Object != p.Lock {
		return errStoreIdentity
	}
	if o.FSMagic != 0xef53 {
		return errStoreUnsupported
	}
	if e := matchMount(o.MountInfo, o.MountComplete, p.Mount); e != nil {
		return e
	}
	if !metaQualified(o.Directory, p, "directory", 0700) || !metaQualified(o.Lock, p, "regular", 0600) {
		return errStoreIdentity
	}
	return nil
}
func inventory(o storageObservation, stage storageObject) error {
	if len(o.Entries) > 3 {
		return errStoreIdentity
	}
	seen := map[string]bool{}
	for _, n := range o.Entries {
		if seen[n] || !oneOf(n, storeLockName, storeSourceName, storeStageName) {
			return errStoreIdentity
		}
		seen[n] = true
	}
	if !seen[storeLockName] || !seen[storeSourceName] {
		return errStoreUnavailable
	}
	if seen[storeStageName] != validObject(stage) {
		return errStoreIdentity
	}
	if validObject(stage) && o.Stage.Object != stage || !validObject(stage) && !reflect.DeepEqual(o.Stage, storageMeta{}) {
		return errStoreIdentity
	}
	return nil
}
func cloneAssertion(a Assertion) Assertion {
	if a.Scope != nil {
		a.Scope = append(make([]Key, 0, len(a.Scope)), a.Scope...)
	}
	return a
}
func cloneEvidence(e storageEvidence) storageEvidence {
	e.Assertion = cloneAssertion(e.Assertion)
	return e
}
func cloneClaim(c gateClaim) gateClaim { c.Evidence = cloneEvidence(c.Evidence); return c }
func cloneGrant(g gateGrant) gateGrant { g.Claim = cloneClaim(g.Claim); return g }
func boundedClaim(c gateClaim) bool {
	a := c.Evidence.Assertion
	if len(a.Scope) > MaxSlots || len(a.Action) > 32 || len(a.Kind) > 16 || len(a.OperationID) > 32 || len(a.JournalID) > 32 || len(a.SourceDigest) > 64 || len(a.BodyDigest) > 64 || len(a.AuthorizationDigest) > 64 {
		return false
	}
	for _, k := range a.Scope {
		if len(k.EnvironmentID) > 32 || len(k.DatabaseID) > 32 || len(k.Capability) > 16 {
			return false
		}
	}
	for _, v := range []string{c.Evidence.ContinuityDigest, c.Evidence.SettlementDigest, c.Evidence.AuthorizationDigest, c.Evidence.EvidenceDigest, c.Close.SettlementDigest, c.Close.AuthorizationDigest} {
		if len(v) > 64 {
			return false
		}
	}
	return true
}

// Check raw sizes before cloning. Returned observations are used synchronously;
// copies passed to the gate prevent it from modifying the coordinator's inputs.
func boundedObservation(o storageObservation) bool {
	if len(o.MountInfo) > mountInfoLimit || len(o.Entries) > 3 || len(o.BootID) > 64 || len(o.Custody) > 32 || len(o.Ref.Session) > 32 {
		return false
	}
	for _, x := range o.Entries {
		if len(x) > 32 {
			return false
		}
	}
	for _, x := range []storageMeta{o.Directory, o.Lock, o.Journal, o.Stage} {
		if len(x.FDInfo) > fdInfoLimit || len(x.Kind) > 16 {
			return false
		}
	}
	return true
}
func cloneMeta(m storageMeta) storageMeta { m.FDInfo = bytes.Clone(m.FDInfo); return m }
func cloneObservation(o storageObservation) storageObservation {
	o.MountInfo = bytes.Clone(o.MountInfo)
	o.Entries = append([]string(nil), o.Entries...)
	o.Directory = cloneMeta(o.Directory)
	o.Lock = cloneMeta(o.Lock)
	o.Journal = cloneMeta(o.Journal)
	o.Stage = cloneMeta(o.Stage)
	return o
}

type storageSession struct {
	mu                   sync.Mutex
	self                 *storageSession
	journal              storageObject
	uncertainty          storageUncertainty
	pendingStage         storageObject
	stageClaimedOwned    bool
	stageQualified       bool
	acquisitionUncertain bool
	pendingCandidate     storeSourceRef
	replaceAttempted     bool
	state                storeState
	adapter              storageAdapter
	gate                 storageEvidenceGate
	provision            expectedProvision
	session, custody     string
	anchor               storeSourceRef
	sourceBytes          []byte
	serial               uint64
	lastGrant            string
	retireAttempted      bool
	reviewOnly           bool
	workerOperation      string
}

func (s *storageSession) enter() error {
	if s == nil || s.self != s {
		return errStoreInvalid
	}
	if !s.mu.TryLock() {
		return errStoreBusy
	}
	switch s.state {
	case storeReviewRequired, storeReviewed:
		return nil
	case storePoisoned:
		s.mu.Unlock()
		return errStoreUncertain
	case storeClosed:
		s.mu.Unlock()
		return errStoreClosed
	default:
		s.mu.Unlock()
		return errStoreInvalid
	}
}
func (s *storageSession) poison(phase storePhase, cause, cleanup error) (publicationResult, error) {
	s.state = storePoisoned
	// Preserve the first uncertain publication facts through retirement failures.
	if s.uncertainty.Phase == "" {
		s.uncertainty = storageUncertainty{Phase: phase, Cause: storeErrorCode(cause), Cleanup: storeErrorCode(cleanup), Stage: s.pendingStage, StageClaimedOwned: s.stageClaimedOwned, StageQualified: s.stageQualified, AcquisitionUncertain: s.acquisitionUncertain, Candidate: s.pendingCandidate, CustodyUnknown: !id(s.custody), ReplaceAttempted: s.replaceAttempted}
	}
	return publicationResult{disposition: storeUncertain, phase: phase, sourceRef: s.anchor}, &storageFailure{cause: cause, phase: phase, cleanup: cleanup, uncertain: true}
}
func (s *storageSession) refusal(phase storePhase, e error) (publicationResult, error) {
	return publicationResult{disposition: storeNotPublished, phase: phase, sourceRef: s.anchor}, &storageFailure{cause: e, phase: phase}
}
func (s *storageSession) grant(ctx context.Context, c gateClaim) (gateGrant, error) {
	if s.gate == nil || !boundedClaim(c) {
		return gateGrant{}, errStoreAuthority
	}
	if ctx == nil || ctx.Err() != nil {
		return gateGrant{}, errStoreBudget
	}
	g, st := s.gate.authorize(ctx, cloneClaim(c))
	if ctx.Err() != nil && st != gateUnknown {
		return gateGrant{}, errStoreBudget
	}
	if st == gateExpired {
		return gateGrant{}, errStoreBudget
	}
	if st == gateUnknown {
		return gateGrant{}, errStoreGateUnknown
	}
	if st != gateAllowed || !id(g.ID) || g.ID == s.lastGrant || !boundedClaim(g.Claim) || !reflect.DeepEqual(g.Claim, c) || !oneOf(g.WorkerEffect, "SETTLED", "UNKNOWN") || g.WorkerEffect == "UNKNOWN" && !id(g.WorkerOperation) || g.WorkerEffect == "SETTLED" && g.WorkerOperation != "" && !id(g.WorkerOperation) {
		return gateGrant{}, errStoreAuthority
	}
	s.lastGrant = g.ID
	return cloneGrant(g), nil
}
func (s *storageSession) confirm(ctx context.Context, g gateGrant, p storePhase, o storageObservation, r phaseResult, stage storageObject, cand storeSourceRef, cleanup bool) error {
	if ctx == nil || ctx.Err() != nil && !cleanup {
		return errStoreBudget
	}
	if !boundedObservation(o) {
		return errStoreIdentity
	}
	st := s.gate.confirm(ctx, cloneGrant(g), gateFacts{p, cloneObservation(o), r, stage, s.anchor, cand, cleanup})
	switch st {
	case gateAllowed:
		if ctx.Err() != nil && !cleanup {
			return errStoreBudget
		}
		return nil
	case gateExpired:
		return errStoreBudget
	case gateUnknown:
		return errStoreGateUnknown
	default:
		return errStoreAuthority
	}
}

// Metadata and content are separate calls. Only a new qualified observation
// reference may be used for one bounded read; never reopen an unchecked path.
func (s *storageSession) readQualified(ctx context.Context, g gateGrant, p storePhase, stage, expectedJournal storageObject, published, cleanup bool) ([]byte, storageObservation, error) {
	if e := s.confirm(ctx, g, p, storageObservation{}, phaseResult{}, stage, s.pendingCandidate, cleanup); e != nil {
		return nil, storageObservation{}, e
	}
	var o storageObservation
	var st adapterStatus
	if published {
		o, st = s.adapter.inspectPublished(ctx, s.custody)
	} else {
		o, st = s.adapter.inspectSource(ctx, s.custody)
	}
	if st != adapterOK {
		return nil, storageObservation{}, errStoreUnavailable
	}
	if !boundedObservation(o) {
		return nil, storageObservation{}, errStoreIdentity
	}
	o = cloneObservation(o)
	qualify := func(x storageObservation) error {
		if !boundedObservation(x) {
			return errStoreIdentity
		}
		if e := validateBase(x, s.provision, s.custody); e != nil {
			return e
		}
		if e := inventory(x, stage); e != nil {
			return e
		}
		if !metaQualified(x.Journal, s.provision, "regular", 0600) || x.Journal.Object == s.provision.Lock || x.Journal.Object == s.provision.Directory {
			return errStoreIdentity
		}
		if x.Journal.Size > MaxEnvelope {
			return ErrInvalidSnapshot
		}
		if validObject(expectedJournal) && x.Journal.Object != expectedJournal {
			return errStoreIdentity
		}
		if validObject(stage) && (!metaQualified(x.Stage, s.provision, "regular", 0600) || x.Stage.Object == x.Journal.Object) {
			return errStoreIdentity
		}
		return nil
	}
	if e := qualify(o); e != nil {
		return nil, o, e
	}
	if o.Ref.Session != s.session || o.Ref.Serial <= s.serial || o.Ref.Object != o.Journal.Object {
		return nil, o, errStoreIdentity
	}
	s.serial = o.Ref.Serial
	if e := s.confirm(ctx, g, p, o, phaseResult{}, stage, s.pendingCandidate, cleanup); e != nil {
		return nil, o, e
	}
	var rr storageRead
	if published {
		rr, st = s.adapter.readPublished(ctx, s.custody, o.Ref, MaxEnvelope+1)
	} else {
		rr, st = s.adapter.readSource(ctx, s.custody, o.Ref, MaxEnvelope+1)
	}
	if st != adapterOK {
		return nil, o, errStoreUnavailable
	}
	if rr.Ref != o.Ref || rr.After.Ref != o.Ref || !boundedObservation(rr.After) {
		return nil, o, errStoreIdentity
	}
	if e := qualify(rr.After); e != nil {
		return nil, o, e
	}
	if !reflect.DeepEqual(rr.After.Journal, o.Journal) || !reflect.DeepEqual(rr.After.Stage, o.Stage) || !rr.Complete || len(rr.Bytes) > MaxEnvelope || uint64(len(rr.Bytes)) != o.Journal.Size {
		return nil, o, ErrInvalidSnapshot
	}
	return bytes.Clone(rr.Bytes), cloneObservation(rr.After), nil
}
func (s *storageSession) checkedSource(ctx context.Context, g gateGrant, p storePhase, stage storageObject, cleanup bool) ([]byte, storageObservation, error) {
	b, o, e := s.readQualified(ctx, g, p, stage, s.journal, false, cleanup)
	if e != nil {
		return nil, o, e
	}
	snap, e := Decode(b)
	if e != nil {
		return nil, o, ErrInvalidSnapshot
	}
	if snap.Payload.JournalID != s.provision.JournalID || snap.Payload.ControllerID != s.provision.ControllerID || snap.Payload.StorageBindingDigest != s.provision.BindingDigest || sourceRef(snap) != g.Claim.Current.Source || s.anchor.valid() && (sourceRef(snap) != s.anchor || !bytes.Equal(b, s.sourceBytes)) {
		return nil, o, errStoreIdentity
	}
	return b, o, nil
}

// A private constructor only; no non-test caller or concrete adapter exists.
func openStore(ctx context.Context, p expectedProvision, c currentnessReference, a storageAdapter, g storageEvidenceGate) (*storageSession, error) {
	if a == nil || g == nil {
		return nil, errStoreAuthority
	}
	if !validProvision(p, c) {
		return nil, errStoreRequest
	}
	s := &storageSession{state: storeReviewRequired, adapter: a, gate: g, provision: p, session: c.SessionID}
	s.self = s
	grant, e := s.grant(ctx, gateClaim{Purpose: "OPEN", Provision: p, Current: c})
	if e != nil {
		return nil, e
	}
	if grant.WorkerEffect != "SETTLED" {
		return nil, errStoreAuthority
	}
	if e = s.confirm(ctx, grant, phaseValidate, storageObservation{}, phaseResult{}, storageObject{}, storeSourceRef{}, false); e != nil {
		return nil, e
	}
	o, st := a.inspectProvision(ctx, p)
	if st != adapterOK || !boundedObservation(o) {
		return nil, errStoreUnavailable
	}
	if e = validateBase(o, p, ""); e != nil {
		return nil, e
	}
	if e = s.confirm(ctx, grant, phaseValidate, o, phaseResult{}, storageObject{}, storeSourceRef{}, false); e != nil {
		return nil, e
	}
	ac := a.acquire(ctx, c.SessionID)
	if ac.Status != adapterOK || !ac.MayHold || !id(ac.Custody) || ac.Settled {
		if ac.MayHold || !ac.Settled || ac.Custody != "" {
			s.acquisitionUncertain = true
			if id(ac.Custody) {
				s.custody = ac.Custody
			}
			_, failure := s.poison(phaseValidate, errStoreUnavailable, nil)
			return s, failure
		}
		if ac.Status == adapterBusy {
			return nil, errStoreBusy
		}
		return nil, errStoreUnavailable
	}
	s.custody = ac.Custody
	b, obs, e := s.checkedSource(ctx, grant, phaseValidate, storageObject{}, false)
	if e == nil {
		e = s.confirm(ctx, grant, phaseFinal, obs, phaseResult{}, storageObject{}, storeSourceRef{}, false)
	}
	if e != nil {
		_, failure := s.poison(phaseValidate, e, nil)
		return s, failure
	}
	s.anchor = c.Source
	s.journal = obs.Journal.Object
	s.sourceBytes = b
	return s, nil
}
func (s *storageSession) validEvidence(e storageEvidence, r Request) bool {
	return validCurrent(e.Current, s.provision) && e.Current.SessionID == s.session && e.Current.Source == s.anchor && reflect.DeepEqual(e.Assertion, r.Assertion) && digests(e.ContinuityDigest, e.SettlementDigest, e.AuthorizationDigest, e.EvidenceDigest) && e.AuthorizationDigest == r.Assertion.AuthorizationDigest
}

type storeInspection struct {
	Source                   storeSourceRef
	Directory, Lock, Journal storageObject
	State                    storeState
	Blocked, ReviewOnly      bool
}

func (s *storageSession) workerEvidence(m *model, g gateGrant) error {
	if g.WorkerEffect != "UNKNOWN" {
		return nil
	}
	o := m.ops[g.WorkerOperation]
	if o == nil || o.closed || s.reviewOnly && s.workerOperation != g.WorkerOperation {
		return errStoreAuthority
	}
	s.reviewOnly = true
	s.workerOperation = g.WorkerOperation
	return nil
}
func (s *storageSession) inspect(ctx context.Context, e storageEvidence) (storeInspection, error) {
	if x := s.enter(); x != nil {
		return storeInspection{}, x
	}
	defer s.mu.Unlock()
	if !validCurrent(e.Current, s.provision) || e.Current.SessionID != s.session || !digests(e.ContinuityDigest, e.EvidenceDigest) {
		return storeInspection{}, errStoreAuthority
	}
	g, x := s.grant(ctx, gateClaim{Purpose: "INSPECT", Provision: s.provision, Current: e.Current, Evidence: e, Custody: s.custody, PriorState: s.state})
	if x != nil {
		if errors.Is(x, errStoreGateUnknown) {
			_, x = s.poison(phaseValidate, x, nil)
		}
		return storeInspection{}, x
	}
	b, o, x := s.checkedSource(ctx, g, phaseValidate, storageObject{}, false)
	if x == nil {
		x = s.confirm(ctx, g, phaseFinal, o, phaseResult{}, storageObject{}, storeSourceRef{}, false)
	}
	if x != nil {
		_, x = s.poison(phaseValidate, x, nil)
		return storeInspection{}, x
	}
	snap, x := Decode(b)
	if x != nil {
		_, x = s.poison(phaseValidate, ErrInvalidSnapshot, nil)
		return storeInspection{}, x
	}
	m, x := replay(snap.Payload)
	if x == nil {
		x = s.workerEvidence(m, g)
	}
	if x != nil {
		_, x = s.poison(phaseValidate, x, nil)
		return storeInspection{}, x
	}
	return storeInspection{s.anchor, s.provision.Directory, s.provision.Lock, s.journal, s.state, m.blocked(), s.reviewOnly}, nil
}
func (s *storageSession) recordRestart(ctx context.Context, r Request, e reviewEvidence) (publicationResult, error) {
	return s.transition(ctx, r, e.storageEvidence, true)
}
func (s *storageSession) applyTransition(ctx context.Context, r Request, e actionEvidence) (publicationResult, error) {
	return s.transition(ctx, r, e.storageEvidence, false)
}

// Before staging, an expired normal budget is a verified refusal only if
// the separate cleanup grant confirms the observed unchanged source and
// settlement. No cleanup write or replacement is allowed on this path.
func (s *storageSession) refusePreStageBudget(ctx context.Context, g gateGrant, phase storePhase, o storageObservation, cause error) (publicationResult, error) {
	if e := s.confirm(context.WithoutCancel(ctx), g, phaseCleanup, o, phaseResult{}, storageObject{}, storeSourceRef{}, true); e != nil {
		return s.poison(phase, cause, e)
	}
	return s.refusal(phase, cause)
}
func (s *storageSession) transition(ctx context.Context, r Request, e storageEvidence, restart bool) (publicationResult, error) {
	if x := s.enter(); x != nil {
		return publicationResult{}, x
	}
	defer s.mu.Unlock()
	if s.gate == nil {
		return publicationResult{}, errStoreAuthority
	}
	if ctx == nil || ctx.Err() != nil {
		return publicationResult{}, errStoreBudget
	}
	if len(r.Body) > MaxEnvelope || len(r.Assertion.Scope) > MaxSlots || len(e.Assertion.Scope) > MaxSlots {
		return publicationResult{}, errStoreRequest
	}
	r.Body = bytes.Clone(r.Body)
	r.Assertion = cloneAssertion(r.Assertion)
	e = cloneEvidence(e)
	purpose := "ACTION"
	if restart {
		purpose = "REVIEW"
	}
	if !validCurrent(e.Current, s.provision) || e.Current.SessionID != s.session {
		return publicationResult{}, errStoreAuthority
	}
	g, x := s.grant(ctx, gateClaim{Purpose: purpose, Provision: s.provision, Current: e.Current, Evidence: e, Custody: s.custody, PriorState: s.state})
	if x != nil {
		if errors.Is(x, errStoreGateUnknown) {
			return s.poison(phaseValidate, x, nil)
		}
		return publicationResult{}, x
	}
	b, o, x := s.checkedSource(ctx, g, phaseValidate, storageObject{}, false)
	if x != nil {
		return s.poison(phaseValidate, x, nil)
	}
	// An ordinary refusal needs positive post-read continued authority. A
	// budget-expired read alone cannot establish a settled refusal.
	if x = s.confirm(ctx, g, phaseValidate, o, phaseResult{}, storageObject{}, storeSourceRef{}, false); x != nil {
		if errors.Is(x, errStoreBudget) {
			return s.refusePreStageBudget(ctx, g, phaseValidate, o, x)
		}
		return s.poison(phaseValidate, x, nil)
	}
	snap, x := Decode(b)
	if x != nil {
		return s.poison(phaseValidate, ErrInvalidSnapshot, nil)
	}
	m, x := replay(snap.Payload)
	if x != nil {
		return s.poison(phaseValidate, ErrInvalidSnapshot, nil)
	}
	if x = s.workerEvidence(m, g); x != nil {
		return s.poison(phaseValidate, x, nil)
	}
	if s.state == storeReviewRequired && !restart {
		return s.refusal(phaseValidate, errStoreReview)
	}
	if restart != (r.Assertion.Action == "RECORD_RESTART") || restart && s.state != storeReviewRequired {
		return s.refusal(phaseValidate, errStoreRequest)
	}
	if !s.validEvidence(e, r) {
		return s.refusal(phaseValidate, errStoreAuthority)
	}
	if restart {
		v, x := typed[restartBody](r.Body)
		if x == nil && (v.Review.ContinuityDigest != e.ContinuityDigest || v.Review.SettlementDigest != e.SettlementDigest) {
			return s.refusal(phaseValidate, errStoreAuthority)
		}
		if g.WorkerEffect != "SETTLED" {
			return s.poison(phaseValidate, errStoreAuthority, nil)
		}
	}

	if s.reviewOnly && r.Assertion.Kind != "REVIEW" {
		return s.refusal(phaseValidate, errStoreReview)
	}
	if x = s.confirm(ctx, g, phaseApply, o, phaseResult{}, storageObject{}, storeSourceRef{}, false); x != nil {
		if errors.Is(x, errStoreBudget) {
			return s.refusePreStageBudget(ctx, g, phaseApply, o, x)
		}
		return s.poison(phaseApply, x, nil)
	}
	candidate, x := Apply(b, r)
	if x != nil {
		return s.refusal(phaseApply, x)
	}
	next, x := Decode(candidate)
	if x != nil || next.Payload.Revision != snap.Payload.Revision+1 {
		return s.poison(phaseApply, ErrInvalidSnapshot, nil)
	}
	// A reviewed terminal fact must not close a tracked unknown worker
	// operation until the independent grant settles that exact operation.
	// Apply validates logical facts; it cannot authenticate actual settlement.
	var nextModel *model
	if s.reviewOnly {
		nextModel, x = replay(next.Payload)
		if x != nil || m.ops[s.workerOperation] == nil || nextModel.ops[s.workerOperation] == nil {
			return s.poison(phaseApply, ErrInvalidSnapshot, nil)
		}
		if !m.ops[s.workerOperation].closed && nextModel.ops[s.workerOperation].closed &&
			(g.WorkerEffect != "SETTLED" || g.WorkerOperation != s.workerOperation) {
			return s.refusal(phaseApply, errStoreAuthority)
		}
	}
	ref := sourceRef(next)
	out, x := s.publish(ctx, g, b, o, candidate, ref)
	if x == nil {
		if restart {
			s.state = storeReviewed
		}
		// A previously published closure already passed the exact worker check.
		// A linked abort can leave the parent open; retain the restriction until
		// a subsequent authorized, settled review resolves every blocker.
		if s.reviewOnly && g.WorkerEffect == "SETTLED" {
			if !nextModel.blocked() && r.Assertion.Kind == "REVIEW" && nextModel.ops[s.workerOperation].closed {
				s.reviewOnly = false
				s.workerOperation = ""
			}
		}
	}
	return out, x
}

func localSettled(r phaseResult, object storageObject) bool {
	return r.Settled && r.Object == object && (r.Status == adapterOK || r.Status == adapterLocalFailure)
}
func (s *storageSession) publish(ctx context.Context, g gateGrant, old []byte, initial storageObservation, candidate []byte, ref storeSourceRef) (publicationResult, error) {
	var stage storageObject
	s.pendingStage = storageObject{}
	s.stageClaimedOwned = false
	s.stageQualified = false
	s.pendingCandidate = ref
	s.replaceAttempted = false
	stageClosed := false
	// Only a positively localized pre-replacement failure can be cleaned up
	// into NOT_PUBLISHED. Cleanup has its own gate-confirmed allowance; it
	// does not reset the normal budget or dispatch any new transition.
	abort := func(p storePhase, cause error, local bool) (publicationResult, error) {
		if !local || errors.Is(cause, errStoreAuthority) {
			return s.poison(p, cause, nil)
		}
		clean := context.WithoutCancel(ctx)
		check := func(r phaseResult) error {
			return s.confirm(clean, g, phaseCleanup, storageObservation{}, r, stage, ref, true)
		}
		if e := check(phaseResult{}); e != nil {
			return s.poison(p, cause, e)
		}
		if validObject(stage) {
			if !stageClosed {
				r := s.adapter.closeStage(clean, s.custody, stage)
				stageClosed = true
				if !localSettled(r, stage) {
					return s.poison(p, cause, errStoreIO)
				}
				if e := check(r); e != nil {
					return s.poison(p, cause, e)
				}
			}
			b, o, e := s.checkedSource(clean, g, phaseCleanup, stage, true)
			if e != nil || !bytes.Equal(b, old) || o.Journal.Object != initial.Journal.Object {
				return s.poison(p, cause, errStoreIdentity)
			}
			if e = check(phaseResult{}); e != nil {
				return s.poison(p, cause, e)
			}
			r := s.adapter.removeOwnedStage(clean, s.custody, stage)
			if r.Status != adapterOK || !r.Settled || r.Object != stage {
				return s.poison(p, cause, errStoreIO)
			}
			if e = check(r); e != nil {
				return s.poison(p, cause, e)
			}
			r = s.adapter.syncDirectory(clean, s.custody)
			if r.Status != adapterOK || !r.Settled || r.Object != s.provision.Directory {
				return s.poison(p, cause, errStoreIO)
			}
			if e = check(r); e != nil {
				return s.poison(p, cause, e)
			}
		}
		b, o, e := s.checkedSource(clean, g, phaseCleanup, storageObject{}, true)
		if e != nil || !bytes.Equal(b, old) || o.Journal.Object != initial.Journal.Object {
			return s.poison(p, cause, errStoreIdentity)
		}
		if e = s.confirm(clean, g, phaseCleanup, o, phaseResult{}, stage, ref, true); e != nil {
			return s.poison(p, cause, e)
		}
		s.pendingStage = storageObject{}
		s.stageClaimedOwned = false
		s.stageQualified = false
		s.pendingCandidate = storeSourceRef{}
		return s.refusal(p, cause)
	}
	check := func(p storePhase, o storageObservation, r phaseResult) error {
		return s.confirm(ctx, g, p, o, r, stage, ref, false)
	}
	if e := check(phaseStageCreate, initial, phaseResult{}); e != nil {
		return abort(phaseStageCreate, e, errors.Is(e, errStoreBudget))
	}
	created := s.adapter.createStage(ctx, s.custody)
	// A reported object is diagnostic evidence only until fully qualified.
	s.stageClaimedOwned = created.Owned
	if validObject(created.Stage.Object) {
		s.pendingStage = created.Stage.Object
	}
	if created.Status != adapterOK || !created.Owned || created.Absent || !metaQualified(created.Stage, s.provision, "regular", 0600) || created.Stage.Size != 0 || created.Stage.Object == initial.Journal.Object || created.Stage.Object == s.provision.Lock || created.Stage.Object == s.provision.Directory {
		if created.Status == adapterLocalFailure && created.Absent && !created.Owned && reflect.DeepEqual(created.Stage, storageMeta{}) {
			return abort(phaseStageCreate, errStoreIO, true)
		}
		return s.poison(phaseStageCreate, errStoreIdentity, nil)
	}
	stage = created.Stage.Object
	s.pendingStage = stage
	s.stageQualified = true
	if e := check(phaseStageWrite, storageObservation{Stage: cloneMeta(created.Stage)}, phaseResult{}); e != nil {
		return abort(phaseStageWrite, e, errors.Is(e, errStoreBudget))
	}
	r := s.adapter.writeStage(ctx, s.custody, stage, bytes.Clone(candidate))
	if r.Status != adapterOK || !r.Settled || r.Object != stage || r.Count != uint64(len(candidate)) {
		return abort(phaseStageWrite, errStoreIO, localSettled(r, stage) && r.Count <= uint64(len(candidate)))
	}
	if e := check(phaseFileSync, storageObservation{}, r); e != nil {
		return abort(phaseFileSync, e, errors.Is(e, errStoreBudget))
	}
	r = s.adapter.syncStage(ctx, s.custody, stage)
	if r.Status != adapterOK || !r.Settled || r.Object != stage {
		return abort(phaseFileSync, errStoreIO, localSettled(r, stage))
	}
	if e := check(phaseStageClose, storageObservation{}, r); e != nil {
		return abort(phaseStageClose, e, errors.Is(e, errStoreBudget))
	}
	r = s.adapter.closeStage(ctx, s.custody, stage)
	stageClosed = true
	if r.Status != adapterOK || !r.Settled || r.Object != stage {
		return abort(phaseStageClose, errStoreIO, localSettled(r, stage))
	}
	b, o, e := s.checkedSource(ctx, g, phasePreReplace, stage, false)
	if e != nil {
		if errors.Is(e, errStoreBudget) {
			return abort(phasePreReplace, e, true)
		}
		return s.poison(phasePreReplace, e, nil)
	}
	if !bytes.Equal(b, old) || o.Journal.Object != initial.Journal.Object || o.Stage.Size != uint64(len(candidate)) {
		return s.poison(phasePreReplace, errStoreIdentity, nil)
	}
	if e = check(phaseReplace, o, r); e != nil {
		return abort(phaseReplace, e, errors.Is(e, errStoreBudget))
	}
	s.replaceAttempted = true
	r = s.adapter.replace(ctx, s.custody, stage, s.anchor)
	// From here onward every failure is uncertain, irrespective of visibility.
	if r.Status != adapterOK || !r.Settled || r.Object != stage {
		return s.poison(phaseReplace, errStoreIO, nil)
	}
	if e = check(phaseDirectorySync, storageObservation{}, r); e != nil {
		return s.poison(phaseDirectorySync, e, nil)
	}
	r = s.adapter.syncDirectory(ctx, s.custody)
	if r.Status != adapterOK || !r.Settled || r.Object != s.provision.Directory {
		return s.poison(phaseDirectorySync, errStoreIO, nil)
	}
	b, o, e = s.readQualified(ctx, g, phaseReadback, storageObject{}, stage, true, false)
	if e != nil {
		return s.poison(phaseReadback, e, nil)
	}
	snap, e := Decode(b)
	if e != nil || !bytes.Equal(b, candidate) || sourceRef(snap) != ref {
		return s.poison(phaseReadback, ErrInvalidSnapshot, nil)
	}
	if e = check(phaseFinal, o, r); e != nil {
		return s.poison(phaseFinal, e, nil)
	}
	oldRef := s.anchor
	s.anchor = ref
	s.journal = stage
	s.pendingStage = storageObject{}
	s.stageClaimedOwned = false
	s.stageQualified = false
	s.pendingCandidate = storeSourceRef{}
	s.replaceAttempted = false
	s.sourceBytes = bytes.Clone(b)
	return publicationResult{disposition: storePublished, phase: phaseDone, sourceRef: oldRef, publishedRef: ref}, nil
}
func (s *storageSession) close(ctx context.Context, e closeEvidence) error {
	if s == nil || s.self != s {
		return errStoreInvalid
	}
	if !s.mu.TryLock() {
		return errStoreBusy
	}
	defer s.mu.Unlock()
	if s.state == storeClosed {
		return errStoreClosed
	}
	if s.state == storeInvalid {
		return errStoreInvalid
	}
	if s.retireAttempted {
		return errStoreUncertain
	}
	// Without a validated custody binding, only external settlement or
	// abandonment can retire this unusable handle. Never invent a token.
	if s.gate == nil || !id(s.custody) || !id(e.Current.SessionID) || e.Current.SessionID != s.session || e.Current.JournalID != s.provision.JournalID || e.Current.BindingDigest != s.provision.BindingDigest || e.Current.Source != s.anchor || !id(e.Current.BudgetID) || e.Current.BudgetDeadline == 0 || !digests(e.Current.HistoryEvidence, e.Current.ReviewReference, e.SettlementDigest, e.AuthorizationDigest) {
		return errStoreAuthority
	}
	g, x := s.grant(ctx, gateClaim{Purpose: "CLOSE", Provision: s.provision, Current: e.Current, Close: e, Custody: s.custody, PriorState: s.state, Uncertainty: s.uncertainty})
	if x != nil {
		if errors.Is(x, errStoreGateUnknown) {
			_, x = s.poison(phaseCleanup, x, nil)
			s.uncertainty.RetirementCause = storeErrorCode(errStoreGateUnknown)
		}
		return x
	}
	if g.WorkerEffect != "SETTLED" {
		// A denied retirement must not discard newly reported unknown effect.
		// Only use the last verified history to retain a restriction, never
		// to establish a new currentness or settlement claim.
		if s.state != storePoisoned {
			snap, e := Decode(s.sourceBytes)
			if e != nil {
				_, x = s.poison(phaseCleanup, ErrInvalidSnapshot, nil)
				return x
			}
			m, e := replay(snap.Payload)
			if e != nil {
				_, x = s.poison(phaseCleanup, ErrInvalidSnapshot, nil)
				return x
			}
			if e = s.workerEvidence(m, g); e != nil {
				_, x = s.poison(phaseCleanup, e, nil)
				return x
			}
		}
		return errStoreAuthority
	}
	if x = s.confirm(ctx, g, phaseCleanup, storageObservation{}, phaseResult{}, s.pendingStage, s.pendingCandidate, true); x != nil {
		if errors.Is(x, errStoreGateUnknown) {
			_, x = s.poison(phaseCleanup, x, nil)
			s.uncertainty.RetirementCause = storeErrorCode(errStoreGateUnknown)
		}
		return x
	}
	s.retireAttempted = true
	r := s.adapter.retire(ctx, s.custody)
	if r.Status != adapterOK || !r.Settled || r.Object != s.provision.Lock {
		_, failure := s.poison(phaseCleanup, errStoreIO, nil)
		s.uncertainty.RetirementCause = storeErrorCode(errStoreIO)
		return failure
	}
	s.state = storeClosed
	s.sourceBytes = nil
	return nil
}
