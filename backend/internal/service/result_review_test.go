package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
)

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

func TestSampleSignBlockReason(t *testing.T) {
	if reason := sampleSignBlockReason("testing"); reason != "" {
		t.Fatalf("expected an in-testing sample to stay signable, got %q", reason)
	}
	for _, status := range []string{"received", "accepted", "hold", "disposed"} {
		reason := sampleSignBlockReason(status)
		if !strings.Contains(reason, status) || !strings.Contains(reason, "已不在检") {
			t.Fatalf("expected sample status %s to block signing, got %q", status, reason)
		}
	}
}

func TestMethodSignBlockReason(t *testing.T) {
	if reason := methodSignBlockReason("active", 3, 3); reason != "" {
		t.Fatalf("expected an unchanged active method to stay signable, got %q", reason)
	}
	if reason := methodSignBlockReason("retired", 3, 3); !strings.Contains(reason, "已退役") {
		t.Fatalf("expected a retired method to block signing, got %q", reason)
	}
	if reason := methodSignBlockReason("validated", 3, 3); !strings.Contains(reason, "非在用") {
		t.Fatalf("expected a non-active method to block signing, got %q", reason)
	}
	reason := methodSignBlockReason("active", 4, 3)
	if !strings.Contains(reason, "v3") || !strings.Contains(reason, "v4") {
		t.Fatalf("expected a method version bump to block signing with both versions, got %q", reason)
	}
}

func TestArchivedBlockReasonKeepsReviewBlocked(t *testing.T) {
	if reason := archivedBlockReason(""); reason != "" {
		t.Fatalf("expected a clean review to have no archived block, got %q", reason)
	}
	reason := archivedBlockReason("样本 LS-002 状态为 hold，已不在检")
	if !strings.Contains(reason, "拦截留档") || !strings.Contains(reason, "新建复核单") {
		t.Fatalf("expected an archived block to demand a fresh review, got %q", reason)
	}
}
