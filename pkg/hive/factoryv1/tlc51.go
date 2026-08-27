package factoryv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	TLC51ProtocolVersion  = "factory-tlc51/v1"
	TLC51PlanSchema       = "tlc-gate-plan/v1"
	TLC51EvaluationSchema = "tlc-gate-evaluation/v1"
	TLC51ReceiptSchema    = "tlc-gate-receipt/v1"
	TLC51RecordSchema     = "tlc-gate-record/v1"
)

// TLC51InformationState is the fail-closed classifier result returned by the
// trusted TLC planner.
type TLC51InformationState string

const (
	TLC51Classified           TLC51InformationState = "CLASSIFIED"
	TLC51Unclassified         TLC51InformationState = "UNCLASSIFIED"
	TLC51BlockedContradiction TLC51InformationState = "BLOCKED_CONTRADICTION"
)

// TLC51ExactJSON preserves trusted TLC output byte-for-byte. CanonicalJSON
// always includes exactly one terminal LF and SHA256 covers those exact bytes.
type TLC51ExactJSON struct {
	SchemaVersion string `json:"schema_version"`
	CanonicalJSON string `json:"canonical_json"`
	SHA256        string `json:"sha256"`
}

type TLC51Obligation struct {
	ID                    string          `json:"id"`
	Kind                  string          `json:"kind"`
	Prerequisites         []string        `json:"prerequisites"`
	ExactSubjectDigest    string          `json:"exact_subject_digest"`
	AdmittedActorFamilies []string        `json:"admitted_actor_families"`
	EvidenceContract      json.RawMessage `json:"evidence_contract"`
	RetryPolicy           string          `json:"retry_policy"`
	ParallelSafe          bool            `json:"parallel_safe"`
}

// TLC51GatePlan is the scheduling identity projected from an exact trusted TLC
// response. Raw is the authority-neutral canonical response, not a Hive
// reconstruction of TLC policy.
type TLC51GatePlan struct {
	Raw              json.RawMessage       `json:"-"`
	Repository       string                `json:"repository"`
	ChangeSeriesID   string                `json:"change_series_id"`
	Subject          json.RawMessage       `json:"subject"`
	SubjectDigest    string                `json:"subject_digest"`
	InformationState TLC51InformationState `json:"information_state"`
	Track            *string               `json:"track"`
	RetainedFloor    *string               `json:"retained_floor"`
	Obligations      []TLC51Obligation     `json:"obligations"`
	AuthorLineages   []string              `json:"author_lineages"`
	PlanDigest       string                `json:"plan_digest"`
}

type TLC51GateReceipt struct {
	Raw              json.RawMessage       `json:"-"`
	Repository       string                `json:"repository"`
	ChangeSeriesID   string                `json:"change_series_id"`
	PlanDigest       string                `json:"plan_digest"`
	Subject          json.RawMessage       `json:"subject"`
	SubjectDigest    string                `json:"subject_digest"`
	InformationState TLC51InformationState `json:"information_state"`
	Decision         string                `json:"decision"`
	AuthorityGranted []json.RawMessage     `json:"authority_granted"`
	EffectsInvoked   []json.RawMessage     `json:"mutation_effects_invoked"`
	ReceiptDigest    string                `json:"receipt_digest"`
}

type TLC51GateClient interface {
	Plan(ctx context.Context, facts json.RawMessage) (TLC51GatePlan, error)
	Evaluate(ctx context.Context, evaluation json.RawMessage) (TLC51GateReceipt, error)
}

func validTLC51SHA(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func decodeTLC51CanonicalObject(raw []byte, schema string) (map[string]any, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' || !json.Valid(raw) {
		return nil, fmt.Errorf("%s must be valid canonical JSON with one terminal LF", schema)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, fmt.Errorf("%s must be one JSON object", schema)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s must contain exactly one JSON value", schema)
	}
	canonical, err := canonicalTLC51JSON(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, fmt.Errorf("%s response is not TLC canonical JSON", schema)
	}
	if got, _ := value["schema_version"].(string); got != schema {
		return nil, fmt.Errorf("schema_version %q does not match %q", got, schema)
	}
	return value, nil
}

func canonicalTLC51JSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(normalized); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func tlc51ObjectDigest(value map[string]any, excluded string) (string, error) {
	copyValue := make(map[string]any, len(value))
	for key, item := range value {
		if key != excluded {
			copyValue[key] = item
		}
	}
	raw, err := canonicalTLC51JSON(copyValue)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func tlc51RawField(value map[string]any, field string) (json.RawMessage, error) {
	item, exists := value[field]
	if !exists {
		return nil, fmt.Errorf("%s is required", field)
	}
	raw, err := canonicalTLC51JSON(item)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(raw, []byte{'\n'}), nil
}

func requireExactTLC51Fields(value map[string]any, required ...string) error {
	wanted := make(map[string]struct{}, len(required))
	for _, field := range required {
		wanted[field] = struct{}{}
	}
	for field := range value {
		if _, ok := wanted[field]; !ok {
			return fmt.Errorf("unexpected field %q", field)
		}
	}
	for _, field := range required {
		if _, ok := value[field]; !ok {
			return fmt.Errorf("%s is required", field)
		}
	}
	return nil
}

// ParseTLC51GatePlan validates exact canonical output and the scheduling
// identities Hive consumes. The trusted TLC executable remains the only source
// of obligation derivation.
func ParseTLC51GatePlan(raw []byte) (TLC51GatePlan, error) {
	value, err := decodeTLC51CanonicalObject(raw, TLC51PlanSchema)
	if err != nil {
		return TLC51GatePlan{}, err
	}
	if err := requireExactTLC51Fields(value,
		"schema_version", "release_identity", "adapter_identity", "repository", "change_series_id",
		"subject", "subject_digest", "normalized_facts_digest", "information_state", "track",
		"retained_floor", "impact_floor", "required_tests", "derived_effects", "requested_effects",
		"authority_requirements", "obligations", "evidence_rules", "unresolved_requests", "reasons",
		"admitted_fact_records", "author_lineages", "plan_digest",
	); err != nil {
		return TLC51GatePlan{}, fmt.Errorf("closed plan object: %w", err)
	}
	expectedDigest, err := tlc51ObjectDigest(value, "plan_digest")
	if err != nil {
		return TLC51GatePlan{}, err
	}
	encoded := func(field string) ([]byte, error) {
		item, exists := value[field]
		if !exists {
			return nil, fmt.Errorf("%s is required", field)
		}
		return json.Marshal(item)
	}
	planBytes, err := encoded("obligations")
	if err != nil {
		return TLC51GatePlan{}, err
	}
	var obligations []TLC51Obligation
	if err := json.Unmarshal(planBytes, &obligations); err != nil || len(obligations) == 0 {
		return TLC51GatePlan{}, errors.New("plan obligations must be a non-empty array")
	}
	authorBytes, err := encoded("author_lineages")
	if err != nil {
		return TLC51GatePlan{}, err
	}
	var authors []string
	if err := json.Unmarshal(authorBytes, &authors); err != nil || len(authors) == 0 {
		return TLC51GatePlan{}, errors.New("plan author_lineages must be non-empty")
	}
	subject, err := tlc51RawField(value, "subject")
	if err != nil {
		return TLC51GatePlan{}, err
	}
	plan := TLC51GatePlan{Raw: append(json.RawMessage(nil), raw...), Subject: subject, Obligations: obligations, AuthorLineages: append([]string(nil), authors...)}
	readString := func(field string, target *string) error {
		got, ok := value[field].(string)
		if !ok || strings.TrimSpace(got) == "" {
			return fmt.Errorf("%s must be a non-empty string", field)
		}
		*target = got
		return nil
	}
	if err := readString("repository", &plan.Repository); err != nil {
		return TLC51GatePlan{}, err
	}
	if err := readString("change_series_id", &plan.ChangeSeriesID); err != nil {
		return TLC51GatePlan{}, err
	}
	if err := readString("subject_digest", &plan.SubjectDigest); err != nil {
		return TLC51GatePlan{}, err
	}
	state, ok := value["information_state"].(string)
	if !ok {
		return TLC51GatePlan{}, errors.New("information_state is required")
	}
	plan.InformationState = TLC51InformationState(state)
	if plan.InformationState != TLC51Classified && plan.InformationState != TLC51Unclassified && plan.InformationState != TLC51BlockedContradiction {
		return TLC51GatePlan{}, fmt.Errorf("invalid information_state %q", state)
	}
	for field, target := range map[string]**string{"track": &plan.Track, "retained_floor": &plan.RetainedFloor} {
		if value[field] == nil {
			continue
		}
		got, ok := value[field].(string)
		if !ok || (got != "M" && got != "I" && got != "D" && got != "H") {
			return TLC51GatePlan{}, fmt.Errorf("invalid %s", field)
		}
		copy := got
		*target = &copy
	}
	if err := readString("plan_digest", &plan.PlanDigest); err != nil {
		return TLC51GatePlan{}, err
	}
	if !validTLC51SHA(plan.PlanDigest) || plan.PlanDigest != expectedDigest || !validTLC51SHA(plan.SubjectDigest) {
		return TLC51GatePlan{}, errors.New("plan or subject digest is invalid")
	}
	subjectCanonical, err := canonicalTLC51JSON(value["subject"])
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(subjectCanonical)) != plan.SubjectDigest {
		return TLC51GatePlan{}, errors.New("plan subject_digest does not bind canonical subject")
	}
	seen := make(map[string]struct{}, len(plan.Obligations))
	for _, obligation := range plan.Obligations {
		if !strings.HasPrefix(obligation.ID, "O") || obligation.Kind == "" || obligation.ExactSubjectDigest != plan.SubjectDigest || len(obligation.AdmittedActorFamilies) == 0 || obligation.RetryPolicy != "same-subject-new-attempt-after-observation" {
			return TLC51GatePlan{}, fmt.Errorf("obligation %q has invalid TLC 5.1 identity", obligation.ID)
		}
		if _, duplicate := seen[obligation.ID]; duplicate {
			return TLC51GatePlan{}, fmt.Errorf("duplicate obligation %q", obligation.ID)
		}
		seen[obligation.ID] = struct{}{}
	}
	for _, obligation := range plan.Obligations {
		for _, prerequisite := range obligation.Prerequisites {
			if _, exists := seen[prerequisite]; !exists || prerequisite == obligation.ID {
				return TLC51GatePlan{}, fmt.Errorf("obligation %q has invalid prerequisite %q", obligation.ID, prerequisite)
			}
		}
	}
	return plan, nil
}

// ParseTLC51GateReceipt validates exact evaluator output, including the
// mandatory empty authority/effect arrays. Evaluation is report-only.
func ParseTLC51GateReceipt(raw []byte, plan TLC51GatePlan) (TLC51GateReceipt, error) {
	value, err := decodeTLC51CanonicalObject(raw, TLC51ReceiptSchema)
	if err != nil {
		return TLC51GateReceipt{}, err
	}
	if err := requireExactTLC51Fields(value,
		"schema_version", "release_identity", "adapter_identity", "repository", "change_series_id",
		"plan_digest", "subject", "subject_digest", "information_state", "retained_floor",
		"predicate_results", "admitted_record_digests", "reviewers", "authority_references",
		"evaluation_clock", "decision", "reasons", "enforcer_provenance", "authority_granted",
		"mutation_effects_invoked", "receipt_digest",
	); err != nil {
		return TLC51GateReceipt{}, fmt.Errorf("closed receipt object: %w", err)
	}
	expectedDigest, err := tlc51ObjectDigest(value, "receipt_digest")
	if err != nil {
		return TLC51GateReceipt{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return TLC51GateReceipt{}, err
	}
	var receipt TLC51GateReceipt
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		return TLC51GateReceipt{}, err
	}
	receipt.Raw = append(json.RawMessage(nil), raw...)
	if receipt.Repository != plan.Repository || receipt.ChangeSeriesID != plan.ChangeSeriesID || receipt.PlanDigest != plan.PlanDigest || receipt.SubjectDigest != plan.SubjectDigest || string(receipt.Subject) != string(plan.Subject) {
		return TLC51GateReceipt{}, errors.New("receipt does not bind the exact plan subject")
	}
	if receipt.Decision != "pass" && receipt.Decision != "fail" && receipt.Decision != "unknown" {
		return TLC51GateReceipt{}, fmt.Errorf("invalid receipt decision %q", receipt.Decision)
	}
	if len(receipt.AuthorityGranted) != 0 || len(receipt.EffectsInvoked) != 0 {
		return TLC51GateReceipt{}, errors.New("TLC receipt must grant no authority and invoke no mutation")
	}
	if !validTLC51SHA(receipt.ReceiptDigest) || receipt.ReceiptDigest != expectedDigest {
		return TLC51GateReceipt{}, errors.New("receipt digest is invalid")
	}
	return receipt, nil
}

type TLC51EventType string

const (
	TLC51PlanRecorded        TLC51EventType = "factory.tlc51.plan.recorded"
	TLC51PlanSuperseded      TLC51EventType = "factory.tlc51.plan.superseded"
	TLC51ObligationReady     TLC51EventType = "factory.tlc51.obligation.ready"
	TLC51ObligationClaimed   TLC51EventType = "factory.tlc51.obligation.claimed"
	TLC51ObligationRunning   TLC51EventType = "factory.tlc51.obligation.running"
	TLC51ObligationTerminal  TLC51EventType = "factory.tlc51.obligation.terminal"
	TLC51EvidenceLinked      TLC51EventType = "factory.tlc51.evidence.linked"
	TLC51DecisionRecorded    TLC51EventType = "factory.tlc51.decision.recorded"
	TLC51DecisionInvalidated TLC51EventType = "factory.tlc51.decision.invalidated"
	TLC51EffectProposed      TLC51EventType = "factory.tlc51.effect.proposed"
	TLC51EffectObserved      TLC51EventType = "factory.tlc51.effect.observed"
	TLC51EffectReconciled    TLC51EventType = "factory.tlc51.effect.reconciled"
	TLC51EffectTerminal      TLC51EventType = "factory.tlc51.effect.terminal"
	TLC51HumanRequested      TLC51EventType = "factory.tlc51.human.requested"
	TLC51HumanResolved       TLC51EventType = "factory.tlc51.human.resolved"
	TLC51CutoverRecorded     TLC51EventType = "factory.tlc51.cutover.recorded"
)

func AllTLC51EventTypes() []TLC51EventType {
	return []TLC51EventType{
		TLC51PlanRecorded, TLC51PlanSuperseded, TLC51ObligationReady, TLC51ObligationClaimed,
		TLC51ObligationRunning, TLC51ObligationTerminal, TLC51EvidenceLinked, TLC51DecisionRecorded,
		TLC51DecisionInvalidated, TLC51EffectProposed, TLC51EffectObserved, TLC51EffectReconciled,
		TLC51EffectTerminal, TLC51HumanRequested, TLC51HumanResolved, TLC51CutoverRecorded,
	}
}

func (value TLC51EventType) valid() bool {
	for _, candidate := range AllTLC51EventTypes() {
		if value == candidate {
			return true
		}
	}
	return false
}

func (value TLC51EventType) IsValid() bool { return value.valid() }

func (value TLC51EventType) attemptRequired() bool {
	switch value {
	case TLC51ObligationReady, TLC51ObligationClaimed, TLC51ObligationRunning, TLC51ObligationTerminal,
		TLC51EvidenceLinked, TLC51EffectProposed, TLC51EffectObserved, TLC51EffectReconciled, TLC51EffectTerminal:
		return true
	default:
		return false
	}
}

type TLC51EventIdentity struct {
	ProtocolVersion string `json:"protocol_version"`
	FactoryOrderID  string `json:"factory_order_id"`
	ChangeSeriesID  string `json:"change_series_id"`
	PlanDigest      string `json:"plan_digest"`
	SubjectDigest   string `json:"subject_digest"`
	EventOrdinal    uint64 `json:"event_ordinal"`
	AttemptOrdinal  uint32 `json:"attempt_ordinal"`
}

type TLC51Append struct {
	Type       TLC51EventType
	Identity   TLC51EventIdentity
	Payload    json.RawMessage
	OccurredAt time.Time
	Causes     []string
}

type TLC51HistoryEntry struct {
	EventID       string             `json:"event_id"`
	Type          TLC51EventType     `json:"event_type"`
	Identity      TLC51EventIdentity `json:"identity"`
	Payload       json.RawMessage    `json:"payload"`
	PayloadSHA256 string             `json:"payload_sha256"`
	OccurredAt    time.Time          `json:"occurred_at"`
	Causes        []string           `json:"causes"`
}

type TLC51Journal interface {
	AppendTLC51(ctx context.Context, input TLC51Append) (TLC51HistoryEntry, error)
	TLC51History(ctx context.Context, factoryOrderID, changeSeriesID string) ([]TLC51HistoryEntry, error)
}

// TLC51WorkArtifact is byte-compatible with Work's
// FactoryOrderTLC51EventArtifact. It binds a Work task to an exact EventGraph
// event payload but does not prove that the EventGraph source is present.
type TLC51WorkArtifact struct {
	FactoryOrderID string          `json:"factory_order_id"`
	ChangeSeriesID string          `json:"change_series_id"`
	EventOrdinal   uint64          `json:"event_ordinal"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	PayloadSHA256  string          `json:"payload_sha256"`
}

type TLC51WorkLinker interface {
	LinkTLC51Event(ctx context.Context, entry TLC51HistoryEntry) error
	TLC51WorkArtifacts(ctx context.Context, factoryOrderID, changeSeriesID string) ([]TLC51WorkArtifact, error)
}

func TLC51WorkArtifactFromEntry(entry TLC51HistoryEntry) TLC51WorkArtifact {
	return TLC51WorkArtifact{
		FactoryOrderID: entry.Identity.FactoryOrderID, ChangeSeriesID: entry.Identity.ChangeSeriesID,
		EventOrdinal: entry.Identity.EventOrdinal, EventType: string(entry.Type),
		Payload: append(json.RawMessage(nil), entry.Payload...), PayloadSHA256: entry.PayloadSHA256,
	}
}

var (
	ErrTLC51HistoryConflict = errors.New("factory-tlc51/v1 history conflict")
	ErrTLC51HistoryGap      = errors.New("factory-tlc51/v1 event ordinal gap")
)

func ValidateTLC51Append(input TLC51Append) error {
	if !input.Type.valid() {
		return fmt.Errorf("unknown TLC 5.1 event type %q", input.Type)
	}
	identity := input.Identity
	if identity.ProtocolVersion != TLC51ProtocolVersion || identity.FactoryOrderID == "" || identity.ChangeSeriesID == "" || identity.EventOrdinal == 0 || !validTLC51SHA(identity.PlanDigest) || !validTLC51SHA(identity.SubjectDigest) {
		return errors.New("invalid TLC 5.1 event identity")
	}
	if input.Type.attemptRequired() != (identity.AttemptOrdinal > 0) {
		return errors.New("TLC 5.1 attempt ordinal does not match event kind")
	}
	if input.OccurredAt.IsZero() || input.OccurredAt.Location() != time.UTC {
		return errors.New("TLC 5.1 event time must be explicit UTC")
	}
	if len(input.Payload) == 0 || !json.Valid(input.Payload) {
		return errors.New("TLC 5.1 event payload must be valid JSON")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, input.Payload); err != nil || !bytes.Equal(compact.Bytes(), input.Payload) {
		return errors.New("TLC 5.1 event payload must preserve compact JSON bytes")
	}
	var payloadIdentity TLC51EventIdentity
	if err := json.Unmarshal(input.Payload, &payloadIdentity); err != nil || payloadIdentity != identity {
		return errors.New("TLC 5.1 payload identity mismatch")
	}
	return nil
}

func NewTLC51EventPayload(identity TLC51EventIdentity, fields map[string]any) (json.RawMessage, error) {
	value := make(map[string]any, len(fields)+7)
	value["protocol_version"] = identity.ProtocolVersion
	value["factory_order_id"] = identity.FactoryOrderID
	value["change_series_id"] = identity.ChangeSeriesID
	value["plan_digest"] = identity.PlanDigest
	value["subject_digest"] = identity.SubjectDigest
	value["event_ordinal"] = identity.EventOrdinal
	value["attempt_ordinal"] = identity.AttemptOrdinal
	for key, item := range fields {
		if _, reserved := value[key]; reserved {
			return nil, fmt.Errorf("field %q would replace TLC 5.1 identity", key)
		}
		value[key] = item
	}
	raw, err := json.Marshal(value)
	return json.RawMessage(raw), err
}

// InMemoryTLC51Journal provides deterministic unit-level replay. Production
// wiring uses Hive's signed EventGraph adapter.
type InMemoryTLC51Journal struct {
	mu      sync.RWMutex
	entries map[string][]TLC51HistoryEntry
}

func NewInMemoryTLC51Journal() *InMemoryTLC51Journal {
	return &InMemoryTLC51Journal{entries: map[string][]TLC51HistoryEntry{}}
}

func tlc51HistoryKey(orderID, seriesID string) string { return orderID + "\x00" + seriesID }

func (journal *InMemoryTLC51Journal) AppendTLC51(ctx context.Context, input TLC51Append) (TLC51HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return TLC51HistoryEntry{}, err
	}
	if err := ValidateTLC51Append(input); err != nil {
		return TLC51HistoryEntry{}, err
	}
	key := tlc51HistoryKey(input.Identity.FactoryOrderID, input.Identity.ChangeSeriesID)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	rows := journal.entries[key]
	entry := TLC51HistoryEntry{
		Type: input.Type, Identity: input.Identity, Payload: append(json.RawMessage(nil), input.Payload...),
		PayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(input.Payload)), OccurredAt: input.OccurredAt,
		Causes: append([]string(nil), input.Causes...),
	}
	entry.EventID = "tlc51-" + HashText(string(input.Type) + "\x00" + string(input.Payload))[:32]
	if input.Identity.EventOrdinal <= uint64(len(rows)) {
		existing := rows[input.Identity.EventOrdinal-1]
		if existing.Type == entry.Type && existing.PayloadSHA256 == entry.PayloadSHA256 && bytes.Equal(existing.Payload, entry.Payload) {
			return cloneTLC51HistoryEntry(existing), nil
		}
		return TLC51HistoryEntry{}, fmt.Errorf("%w: ordinal %d", ErrTLC51HistoryConflict, input.Identity.EventOrdinal)
	}
	if input.Identity.EventOrdinal != uint64(len(rows)+1) {
		return TLC51HistoryEntry{}, fmt.Errorf("%w: got %d want %d", ErrTLC51HistoryGap, input.Identity.EventOrdinal, len(rows)+1)
	}
	journal.entries[key] = append(rows, entry)
	return cloneTLC51HistoryEntry(entry), nil
}

func (journal *InMemoryTLC51Journal) TLC51History(ctx context.Context, factoryOrderID, changeSeriesID string) ([]TLC51HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	journal.mu.RLock()
	defer journal.mu.RUnlock()
	rows := journal.entries[tlc51HistoryKey(factoryOrderID, changeSeriesID)]
	result := make([]TLC51HistoryEntry, len(rows))
	for index, row := range rows {
		result[index] = cloneTLC51HistoryEntry(row)
	}
	return result, nil
}

func cloneTLC51HistoryEntry(value TLC51HistoryEntry) TLC51HistoryEntry {
	value.Payload = append(json.RawMessage(nil), value.Payload...)
	value.Causes = append([]string(nil), value.Causes...)
	return value
}

func SortTLC51History(values []TLC51HistoryEntry) []TLC51HistoryEntry {
	result := make([]TLC51HistoryEntry, len(values))
	for index, value := range values {
		result[index] = cloneTLC51HistoryEntry(value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Identity.EventOrdinal < result[j].Identity.EventOrdinal })
	return result
}

// TLC51FactoryOrderID is the stable Work/EventGraph protocol identity for a
// canonical FactoryOrder document tuple.
func TLC51FactoryOrderID(orderID, version string) string {
	return "fo_" + HashText(orderID + "\x00" + version)[:24]
}
