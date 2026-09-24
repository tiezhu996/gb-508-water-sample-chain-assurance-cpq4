package service

import (
	"errors"
	"testing"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
)

func parseReviewTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed
}

func TestValidateResultReviewTransitionRejectsSameSigner(t *testing.T) {
	current := model.ResultReview{
		BaseModel:         model.BaseModel{Status: "peer_review"},
		ReviewRequestedBy: "reviewer-a",
	}

	err := validateResultReviewTransition(current, "signed", "reviewer-a")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected same-person signing to fail business validation, got %v", err)
	}
	if err := validateResultReviewTransition(current, "signed", "reviewer-b"); err != nil {
		t.Fatalf("expected a different reviewer to sign: %v", err)
	}
}

func TestValidateResultReviewTransitionAllowsArchivedReviewToReachSigningGate(t *testing.T) {
	// The state-machine validator must not short-circuit archived reviews; the
	// signing gate in Transition owns the block so every attempt is audited.
	blockedAt := parseReviewTime(t, "2026-09-20T10:00:00Z")
	current := model.ResultReview{
		BaseModel:          model.BaseModel{Status: "peer_review"},
		ReviewRequestedBy:  "operator",
		BasisBlockedAt:     &blockedAt,
		BasisBlockedReason: "方法已退役",
	}
	if err := validateResultReviewTransition(current, "signed", "reviewer"); err != nil {
		t.Fatalf("archived review should reach the signing gate, got %v", err)
	}
}

func TestBuildReviewBasisAllowsFreshTestingSampleAndSameMethodVersion(t *testing.T) {
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-003", SampleBatch: "SB-003", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "GB-11893", MethodSnapshot: 3, MethodStatus: "active",
	}
	sample := &model.LabSample{BaseModel: model.BaseModel{ID: 1, Status: "testing"}, RelatedCode: "SB-003"}
	method := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 3, Status: "active"}}

	basis := buildReviewBasis(review, sample, method)
	if !basis.Eligible || basis.Locked || len(basis.Reasons) != 0 {
		t.Fatalf("expected fresh basis to be eligible, got %+v", basis)
	}
}

func TestBuildReviewBasisBlocksHeldSample(t *testing.T) {
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-005", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "HJ-535", MethodSnapshot: 2,
	}
	sample := &model.LabSample{BaseModel: model.BaseModel{ID: 1, Status: "hold"}}
	method := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 2, Status: "active"}}

	basis := buildReviewBasis(review, sample, method)
	if basis.Eligible {
		t.Fatal("held sample must block signing")
	}
	if len(basis.Reasons) != 1 || basis.Reasons[0] == "" {
		t.Fatalf("expected one sample block reason, got %v", basis.Reasons)
	}
}

func TestBuildReviewBasisBlocksDisposedSample(t *testing.T) {
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-009", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "HJ-535", MethodSnapshot: 2,
	}
	sample := &model.LabSample{BaseModel: model.BaseModel{ID: 1, Status: "disposed"}}
	method := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 2, Status: "active"}}

	basis := buildReviewBasis(review, sample, method)
	if basis.Eligible || len(basis.Reasons) != 1 {
		t.Fatalf("disposed sample must block signing, got %+v", basis)
	}
}

func TestBuildReviewBasisBlocksRetiredAndReversionedMethod(t *testing.T) {
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-003", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "GB-7479-OLD", MethodSnapshot: 2,
	}
	sample := &model.LabSample{BaseModel: model.BaseModel{ID: 1, Status: "testing"}}

	retired := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 2, Status: "retired"}}
	basis := buildReviewBasis(review, sample, retired)
	if basis.Eligible || len(basis.Reasons) != 1 {
		t.Fatalf("retired method must block signing, got %+v", basis)
	}

	reversioned := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 3, Status: "active"}}
	basis = buildReviewBasis(review, sample, reversioned)
	if basis.Eligible || len(basis.Reasons) != 1 {
		t.Fatalf("method version change must block signing, got %+v", basis)
	}
}

func TestBuildReviewBasisBlocksMissingRecords(t *testing.T) {
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-404", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "AM-404", MethodSnapshot: 1,
	}
	basis := buildReviewBasis(review, nil, nil)
	if basis.Eligible {
		t.Fatal("missing sample and method must block signing")
	}
	if !basis.SampleMissing || !basis.MethodMissing || len(basis.Reasons) != 2 {
		t.Fatalf("expected two missing-record reasons, got %+v", basis)
	}
}

func TestBuildReviewBasisMarksArchivedReviewLocked(t *testing.T) {
	blockedAt := parseReviewTime(t, "2026-09-20T10:00:00Z")
	review := model.ResultReview{
		BaseModel: model.BaseModel{Status: string(constants.ReviewStatePeerReview)},
		SampleID:  1, SampleCode: "LS-003", SampleSnapshot: "testing",
		MethodID: 2, MethodCode: "GB-11893", MethodSnapshot: 1,
		BasisBlockedAt:     &blockedAt,
		BasisBlockedReason: "方法 GB-11893 已换版",
	}
	sample := &model.LabSample{BaseModel: model.BaseModel{ID: 1, Status: "testing"}}
	method := &model.AssayMethod{BaseModel: model.BaseModel{ID: 2, Version: 1, Status: "active"}}

	basis := buildReviewBasis(review, sample, method)
	if !basis.Locked || basis.LockedReason == "" {
		t.Fatalf("previously blocked review must stay locked for archive, got %+v", basis)
	}
}
