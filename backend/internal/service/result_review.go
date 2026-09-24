package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/constants"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/dto"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/repository"
	"gorm.io/gorm"
)

type ResultReviewService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.ResultReview], error)
	Get(context.Context, uint) (model.ResultReview, error)
	Create(context.Context, dto.CreateResultReview, string, string) (model.ResultReview, error)
	Update(context.Context, uint, dto.UpdateResultReview, string, string) (model.ResultReview, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.ResultReview, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type resultReviewService struct {
	repository repository.ResultReviewRepository
	samples    repository.LabSampleRepository
	methods    repository.AssayMethodRepository
	batches    repository.SamplingBatchRepository
	security   SecurityService
}

func NewResultReviewService(repo repository.ResultReviewRepository, samples repository.LabSampleRepository, methods repository.AssayMethodRepository, batches repository.SamplingBatchRepository, security SecurityService) ResultReviewService {
	return &resultReviewService{repository: repo, samples: samples, methods: methods, batches: batches, security: security}
}

func (s *resultReviewService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ResultReview], error) {
	return s.repository.List(ctx, query)
}

func (s *resultReviewService) Get(ctx context.Context, id uint) (model.ResultReview, error) {
	return s.repository.Get(ctx, id)
}

func (s *resultReviewService) Create(ctx context.Context, input dto.CreateResultReview, actor, requestID string) (model.ResultReview, error) {
	if err := validateResultReviewBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	sampleCode := strings.ToUpper(strings.TrimSpace(input.SampleCode))
	methodCode := strings.ToUpper(strings.TrimSpace(input.MethodCode))
	if _, _, err := s.loadReviewTargets(ctx, sampleCode, methodCode); err != nil {
		return model.ResultReview{}, err
	}
	item := model.ResultReview{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.ResultReviewInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
		SampleCode:  sampleCode, MethodCode: methodCode,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ResultReview{}, fmt.Errorf("create 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ResultReview", item.ID, "", item.Status, "created 结果复核")
	return item, nil
}

func (s *resultReviewService) Update(ctx context.Context, id uint, input dto.UpdateResultReview, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	if reason := archivedBlockReason(current.BlockedReason); reason != "" {
		return model.ResultReview{}, fmt.Errorf("%w: %s", ErrReviewBlocked, reason)
	}
	if err := validateResultReviewBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ResultReview{}, fmt.Errorf("update 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ResultReview", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *resultReviewService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	target := strings.TrimSpace(input.Status)
	if err := validateResultReviewTransition(current, target, actor); err != nil {
		return model.ResultReview{}, err
	}
	if reason := archivedBlockReason(current.BlockedReason); reason != "" {
		_ = s.security.Audit(ctx, actor, requestID, "block", "ResultReview", id, current.Status, current.Status, "拦截留档单禁止再迁移: "+reason)
		return model.ResultReview{}, fmt.Errorf("%w: %s", ErrReviewBlocked, reason)
	}
	before := current.Status
	switch target {
	case string(constants.ReviewStatePeerReview):
		if err := s.refreshReviewSnapshot(ctx, &current); err != nil {
			return model.ResultReview{}, err
		}
		current.ReviewRequestedBy = strings.TrimSpace(actor)
		current.PeerReviewedBy = ""
		current.SignedBy = ""
	case string(constants.ReviewStateSigned):
		if reason := s.signBlockReason(ctx, current); reason != "" {
			return model.ResultReview{}, s.blockIssuance(ctx, &current, input.ExpectedVersion, actor, requestID, reason)
		}
		current.PeerReviewedBy = strings.TrimSpace(actor)
		current.SignedBy = strings.TrimSpace(actor)
	case string(constants.ReviewStateDraft):
		current.ReviewRequestedBy = ""
		current.PeerReviewedBy = ""
		current.SignedBy = ""
	}
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ResultReview{}, fmt.Errorf("transition 结果复核: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "ResultReview", id, before, target, input.Reason); err != nil {
		return model.ResultReview{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, id)
}

// refreshReviewSnapshot re-validates the selected sample and method at submit
// time and records the batch, sample status and method version the review is
// based on. The transition entry point guarantees the review is not archived.
func (s *resultReviewService) refreshReviewSnapshot(ctx context.Context, review *model.ResultReview) error {
	sample, method, err := s.loadReviewTargets(ctx, review.SampleCode, review.MethodCode)
	if err != nil {
		return err
	}
	review.SampleStatus = sample.Status
	review.SampleBatchCode = s.resolveBatchCode(ctx, sample)
	review.MethodVersion = method.Version
	return nil
}

// signBlockReason re-checks the live sample and method right before issuance.
// A previously blocked review stays blocked even if the records recover.
func (s *resultReviewService) signBlockReason(ctx context.Context, review model.ResultReview) string {
	if reason := archivedBlockReason(review.BlockedReason); reason != "" {
		return reason
	}
	sample, sampleErr := s.samples.FindByCode(ctx, review.SampleCode)
	if errors.Is(sampleErr, gorm.ErrRecordNotFound) {
		return fmt.Sprintf("样本 %s 不存在或已删除", review.SampleCode)
	}
	method, methodErr := s.methods.FindByCode(ctx, review.MethodCode)
	if errors.Is(methodErr, gorm.ErrRecordNotFound) {
		return fmt.Sprintf("方法 %s 不存在或已删除", review.MethodCode)
	}
	if sampleErr == nil {
		if reason := sampleSignBlockReason(sample.Status); reason != "" {
			return fmt.Sprintf("样本 %s %s", review.SampleCode, reason)
		}
	}
	if methodErr == nil {
		if reason := methodSignBlockReason(method.Status, method.Version, review.MethodVersion); reason != "" {
			return fmt.Sprintf("方法 %s %s", review.MethodCode, reason)
		}
	}
	return ""
}

// blockIssuance archives the blocked reason on the review and writes an audit
// entry so the failed signing attempt stays traceable.
func (s *resultReviewService) blockIssuance(ctx context.Context, review *model.ResultReview, expectedVersion uint, actor, requestID, reason string) error {
	if strings.TrimSpace(review.BlockedReason) == "" {
		review.BlockedReason = reason
		review.Version = expectedVersion + 1
		review.UpdatedAt = time.Now().UTC()
		if err := s.repository.Update(ctx, review.ID, expectedVersion, review); err != nil {
			if !errors.Is(err, repository.ErrVersionConflict) {
				return fmt.Errorf("persist blocked 结果复核: %w", err)
			}
			// A concurrent request may already have archived the block; re-read it.
			fresh, reloadErr := s.repository.Get(ctx, review.ID)
			if reloadErr != nil {
				return fmt.Errorf("reload blocked 结果复核: %w", reloadErr)
			}
			reason = fresh.BlockedReason
		}
	}
	_ = s.security.Audit(ctx, actor, requestID, "block", "ResultReview", review.ID, review.Status, review.Status, "签发被拦截: "+reason)
	return fmt.Errorf("%w: %s", ErrReviewBlocked, reason)
}

// loadReviewTargets resolves the selected sample and method and requires the
// sample to be in testing and the method to be active.
func (s *resultReviewService) loadReviewTargets(ctx context.Context, sampleCode, methodCode string) (model.LabSample, model.AssayMethod, error) {
	if sampleCode == "" || methodCode == "" {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("%w: 必须选择在检样本和检测方法", ErrInvalidInput)
	}
	sample, err := s.samples.FindByCode(ctx, sampleCode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("%w: 样本 %s 不存在", ErrInvalidInput, sampleCode)
	}
	if err != nil {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("load 实验室样本: %w", err)
	}
	if reason := sampleSignBlockReason(sample.Status); reason != "" {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("%w: 样本 %s %s", ErrInvalidInput, sampleCode, reason)
	}
	method, err := s.methods.FindByCode(ctx, methodCode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("%w: 方法 %s 不存在", ErrInvalidInput, methodCode)
	}
	if err != nil {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("load 检测方法: %w", err)
	}
	if method.Status != string(constants.AssayMethodStateActive) {
		return model.LabSample{}, model.AssayMethod{}, fmt.Errorf("%w: 方法 %s 当前状态为 %s，非在用", ErrInvalidInput, methodCode, method.Status)
	}
	return sample, method, nil
}

// resolveBatchCode maps the sample's related code to the sampling batch code
// so the review records which batch the sample came from.
func (s *resultReviewService) resolveBatchCode(ctx context.Context, sample model.LabSample) string {
	related := strings.TrimSpace(sample.RelatedCode)
	if related == "" {
		return ""
	}
	batch, err := s.batches.FindByRelatedCode(ctx, related)
	if err != nil {
		return related
	}
	return batch.Code
}

func archivedBlockReason(blockedReason string) string {
	if strings.TrimSpace(blockedReason) == "" {
		return ""
	}
	return "复核单已被拦截留档（" + blockedReason + "），请新建复核单"
}

func sampleSignBlockReason(sampleStatus string) string {
	if sampleStatus != string(constants.SampleStateTesting) {
		return "状态为 " + sampleStatus + "，已不在检"
	}
	return ""
}

func methodSignBlockReason(methodStatus string, liveVersion, snapshotVersion uint) string {
	switch {
	case methodStatus == string(constants.AssayMethodStateRetired):
		return "已退役"
	case methodStatus != string(constants.AssayMethodStateActive):
		return "当前状态为 " + methodStatus + "，非在用"
	case liveVersion != snapshotVersion:
		return fmt.Sprintf("已换版 v%d → v%d", snapshotVersion, liveVersion)
	}
	return ""
}

func validateResultReviewTransition(current model.ResultReview, target, actor string) error {
	if !constants.CanTransition(constants.ResultReviewTransitions, current.Status, target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return fmt.Errorf("%w: transition actor is required", ErrInvalidInput)
	}
	if target == string(constants.ReviewStateSigned) {
		if current.ReviewRequestedBy == "" {
			return fmt.Errorf("%w: peer-review submitter is missing", ErrInvalidInput)
		}
		if strings.EqualFold(current.ReviewRequestedBy, actor) {
			return fmt.Errorf("%w: signer must differ from peer-review submitter", ErrInvalidInput)
		}
	}
	return nil
}

func (s *resultReviewService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "ResultReview", id, current.Status, "deleted", "soft deleted 结果复核")
}

func (s *resultReviewService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateResultReviewBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
