package constants

// Shared status values are mirrored in frontend/src/types/status.ts. Keeping
// the lists explicit makes state-machine drift visible during code review.

type SampleState string

const (
	SampleStateReceived SampleState = "received"
	SampleStateAccepted SampleState = "accepted"
	SampleStateTesting  SampleState = "testing"
	SampleStateHold     SampleState = "hold"
	SampleStateDisposed SampleState = "disposed"
)

var AllSampleState = []string{"received", "accepted", "testing", "hold", "disposed"}

type AssayMethodState string

const (
	AssayMethodStateDraft     AssayMethodState = "draft"
	AssayMethodStateValidated AssayMethodState = "validated"
	AssayMethodStateActive    AssayMethodState = "active"
	AssayMethodStateRetired   AssayMethodState = "retired"
)

var AllAssayMethodState = []string{"draft", "validated", "active", "retired"}

type ReviewState string

const (
	ReviewStateDraft      ReviewState = "draft"
	ReviewStatePeerReview ReviewState = "peer_review"
	ReviewStateSigned     ReviewState = "signed"
	ReviewStateRejected   ReviewState = "rejected"
)

var AllReviewState = []string{"draft", "peer_review", "signed", "rejected"}

var SamplingBatchTransitions = map[string]map[string]bool{
	"planned":    {"collecting": true, "received": true},
	"collecting": {"received": true, "closed": true, "planned": true},
	"received":   {"closed": true, "collecting": true},
	"closed":     {"received": true},
}

var LabSampleTransitions = map[string]map[string]bool{
	"received": {"accepted": true, "testing": true},
	"accepted": {"testing": true, "hold": true, "received": true},
	"testing":  {"hold": true, "disposed": true, "accepted": true},
	"hold":     {"disposed": true, "testing": true},
	"disposed": {"hold": true},
}

var AssayMethodTransitions = map[string]map[string]bool{
	"draft":     {"validated": true, "active": true},
	"validated": {"active": true, "retired": true, "draft": true},
	"active":    {"retired": true, "validated": true},
	"retired":   {"active": true},
}

var ResultReviewTransitions = map[string]map[string]bool{
	"draft":       {"peer_review": true},
	"peer_review": {"signed": true, "rejected": true, "draft": true},
	"signed":      {"rejected": true},
	"rejected":    {"draft": true, "peer_review": true},
}

func CanTransition(graph map[string]map[string]bool, from, to string) bool {
	targets, exists := graph[from]
	return exists && targets[to]
}

// Chinese status labels back review-basis prompts so the workbench and audit
// records always show the same human-readable state wording.
var statusLabels = map[string]string{
	"received": "已接收", "accepted": "已受理", "testing": "在检", "hold": "暂停", "disposed": "已处置",
	"draft": "草稿", "validated": "已验证", "active": "现行", "retired": "已退役",
	"peer_review": "待复核", "signed": "已签发", "rejected": "已驳回",
	"planned": "计划中", "collecting": "采样中", "closed": "已关闭",
}

// StatusLabel returns the Chinese wording for a domain status, falling back
// to the raw value for unknown states.
func StatusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}
