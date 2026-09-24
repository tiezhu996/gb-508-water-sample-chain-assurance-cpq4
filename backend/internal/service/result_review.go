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
	security   SecurityService
}

func NewResultReviewService(repo repository.ResultReviewRepository, samples repository.LabSampleRepository, methods repository.AssayMethodRepository, security SecurityService) ResultReviewService {
	return &resultReviewService{repository: repo, samples: samples, methods: methods, security: security}
}

func (s *resultReviewService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.ResultReview], error) {
	page, err := s.repository.List(ctx, query)
	if err != nil {
		return page, err
	}
	if err := s.attachBasis(ctx, page.Items); err != nil {
		return page, err
	}
	return page, nil
}

func (s *resultReviewService) Get(ctx context.Context, id uint) (model.ResultReview, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return item, err
	}
	items := []model.ResultReview{item}
	if err := s.attachBasis(ctx, items); err != nil {
		return item, err
	}
	return items[0], nil
}

func (s *resultReviewService) Create(ctx context.Context, input dto.CreateResultReview, actor, requestID string) (model.ResultReview, error) {
	if err := validateResultReviewBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	sample, method, err := s.resolveBasis(ctx, input.SampleID, input.MethodID)
	if err != nil {
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
		RelatedCode: method.Code, SampleID: sample.ID, SampleCode: sample.Code,
		SampleBatch: sample.RelatedCode, MethodID: method.ID, MethodCode: method.Code,
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.ResultReview{}, fmt.Errorf("create 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "ResultReview", item.ID, "", item.Status,
		fmt.Sprintf("created 结果复核; 依据样本 %s(%s), 方法 %s(v%d)", sample.Code, constants.StatusLabel(sample.Status), method.Code, method.Version))
	return s.repository.Get(ctx, item.ID)
}

func (s *resultReviewService) Update(ctx context.Context, id uint, input dto.UpdateResultReview, actor, requestID string) (model.ResultReview, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.ResultReview{}, err
	}
	if current.Status != string(constants.ReviewStateDraft) {
		return model.ResultReview{}, fmt.Errorf("%w: 仅草稿复核单可修改；补正后请新建复核单", ErrInvalidInput)
	}
	if err := validateResultReviewBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.ResultReview{}, err
	}
	sample, method, err := s.resolveBasis(ctx, input.SampleID, input.MethodID)
	if err != nil {
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
	current.RelatedCode = method.Code
	current.SampleID = sample.ID
	current.SampleCode = sample.Code
	current.SampleBatch = sample.RelatedCode
	current.MethodID = method.ID
	current.MethodCode = method.Code
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.ResultReview{}, fmt.Errorf("update 结果复核: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "ResultReview", id, current.Status, current.Status,
		fmt.Sprintf("updated business fields; 依据样本 %s, 方法 %s(v%d)", sample.Code, method.Code, method.Version))
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
	actor = strings.TrimSpace(actor)

	switch target {
	case string(constants.ReviewStatePeerReview):
		// 提交复核 (含驳回后重新提交): re-check the live basis and freeze the
		// sample batch/status and method version into the review.
		sample, method, basisErr := s.resolveBasis(ctx, current.SampleID, current.MethodID)
		if basisErr != nil {
			return model.ResultReview{}, basisErr
		}
		before := current.Status
		current.Status = target
		current.ReviewRequestedBy = actor
		current.PeerReviewedBy = ""
		current.SignedBy = ""
		current.SampleCode = sample.Code
		current.SampleBatch = sample.RelatedCode
		current.SampleSnapshot = sample.Status
		current.MethodCode = method.Code
		current.MethodSnapshot = method.Version
		current.MethodStatus = method.Status
		current.BasisBlockedAt = nil
		current.BasisBlockedReason = ""
		// A rejected review can be corrected and re-submitted; the re-freeze
		// above already guarantees the basis is current.
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = time.Now().UTC()
		if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
			return model.ResultReview{}, fmt.Errorf("transition 结果复核: %w", err)
		}
		detail := fmt.Sprintf("%s; 提交人 %s; 样本批次 %s, 样本状态 %s; 方法 %s v%d(%s)",
			input.Reason, actor, current.SampleBatch, constants.StatusLabel(sample.Status),
			method.Code, method.Version, constants.StatusLabel(method.Status))
		if err := s.security.Audit(ctx, actor, requestID, "transition", "ResultReview", id, before, target, detail); err != nil {
			return model.ResultReview{}, fmt.Errorf("persist transition audit: %w", err)
		}
		return s.repository.Get(ctx, id)

	case string(constants.ReviewStateSigned):
		// 签发前再核对: a previously blocked (archived) review can never be
		// signed, and a review whose sample is no longer 在检 or whose method was
		// 换版/退役 is locked on the spot. Either way the attempt is recorded.
		basis, basisErr := s.evaluateBasis(ctx, current)
		if basisErr != nil {
			return model.ResultReview{}, basisErr
		}
		if basis.Locked {
			_ = s.security.Audit(ctx, actor, requestID, "sign_blocked", "ResultReview", id, current.Status, current.Status,
				fmt.Sprintf("签发被拦截(旧单留档): %s", basis.LockedReason))
			return model.ResultReview{}, fmt.Errorf("%w: %s（旧单留档，请补正后新建复核单）", ErrReviewBlocked, basis.LockedReason)
		}
		if !basis.Eligible {
			reason := strings.Join(basis.Reasons, "；")
			blockedAt := time.Now().UTC()
			current.BasisBlockedAt = &blockedAt
			current.BasisBlockedReason = reason
			current.Version = input.ExpectedVersion + 1
			current.UpdatedAt = blockedAt
			if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
				return model.ResultReview{}, fmt.Errorf("persist blocked basis: %w", err)
			}
			auditDetail := fmt.Sprintf("签发被拦截(样本/方法依据过期): %s; 样本 %s 提交时=%s 当前=%s; 方法 %s 提交版本=v%d 当前=v%d/%s",
				reason, current.SampleCode, constants.StatusLabel(current.SampleSnapshot), constants.StatusLabel(basis.SampleStatus),
				current.MethodCode, current.MethodSnapshot, basis.MethodVersion, constants.StatusLabel(basis.MethodStatus))
			if err := s.security.Audit(ctx, actor, requestID, "sign_blocked", "ResultReview", id, current.Status, current.Status, auditDetail); err != nil {
				return model.ResultReview{}, fmt.Errorf("persist block audit: %w", err)
			}
			return model.ResultReview{}, fmt.Errorf("%w: %s（已留存，请补正后新建复核单）", ErrReviewBlocked, reason)
		}
		before := current.Status
		current.Status = target
		current.PeerReviewedBy = actor
		current.SignedBy = actor
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = time.Now().UTC()
		if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
			return model.ResultReview{}, fmt.Errorf("transition 结果复核: %w", err)
		}
		if err := s.security.Audit(ctx, actor, requestID, "transition", "ResultReview", id, before, target,
			fmt.Sprintf("%s; 签发前核对通过: 样本 %s 仍为%s, 方法 %s v%d %s", input.Reason,
				current.SampleCode, constants.StatusLabel(basis.SampleStatus), current.MethodCode, basis.MethodVersion, constants.StatusLabel(basis.MethodStatus))); err != nil {
			return model.ResultReview{}, fmt.Errorf("persist transition audit: %w", err)
		}
		return s.repository.Get(ctx, id)

	default:
		// rejected / back to draft transitions keep the stored workflow.
		if current.BasisBlockedAt != nil {
			action := "驳回或退回"
			_ = s.security.Audit(ctx, actor, requestID, "sign_blocked", "ResultReview", id, current.Status, current.Status,
				fmt.Sprintf("操作被拦截: 依据过期的留档复核单不可%s，请补正后新建复核单", action))
			return model.ResultReview{}, fmt.Errorf("%w: 依据过期的旧单已留档，不可%s，请补正后新建复核单", ErrReviewBlocked, action)
		}
		before := current.Status
		current.Status = target
		if target == string(constants.ReviewStateDraft) {
			current.ReviewRequestedBy = ""
			current.PeerReviewedBy = ""
			current.SignedBy = ""
		}
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
}

// resolveBasis loads the selected sample and method and enforces that the
// sample is 在检 and the method is the 现行 version. Used at draft creation,
// draft edits and peer-review submission.
func (s *resultReviewService) resolveBasis(ctx context.Context, sampleID, methodID uint) (model.LabSample, model.AssayMethod, error) {
	var sample model.LabSample
	var method model.AssayMethod
	if sampleID == 0 || methodID == 0 {
		return sample, method, fmt.Errorf("%w: 请选择在检样本和使用的方法", ErrInvalidInput)
	}
	sample, err := s.samples.Get(ctx, sampleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sample, method, fmt.Errorf("%w: 所选样本不存在", ErrInvalidInput)
		}
		return sample, method, err
	}
	if sample.Status != string(constants.SampleStateTesting) {
		return sample, method, fmt.Errorf("%w: 样本 %s 当前为%s，仅在检样本可建立复核依据", ErrInvalidInput, sample.Code, constants.StatusLabel(sample.Status))
	}
	method, err = s.methods.Get(ctx, methodID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sample, method, fmt.Errorf("%w: 所选方法不存在", ErrInvalidInput)
		}
		return sample, method, err
	}
	if method.Status != string(constants.AssayMethodStateActive) {
		return sample, method, fmt.Errorf("%w: 方法 %s 当前为%s，仅现行方法版本可作为复核依据", ErrInvalidInput, method.Code, constants.StatusLabel(method.Status))
	}
	return sample, method, nil
}

// evaluateBasis compares the review's frozen snapshot against live sample and
// method state. Pure input data makes the signing-gate decision unit-testable.
func (s *resultReviewService) evaluateBasis(ctx context.Context, review model.ResultReview) (*model.ReviewBasis, error) {
	basis := buildReviewBasis(review, nil, nil)
	if review.SampleID == 0 || review.MethodID == 0 {
		return basis, nil
	}
	samples, err := s.samples.ListByIDs(ctx, []uint{review.SampleID})
	if err != nil {
		return basis, err
	}
	methods, err := s.methods.ListByIDs(ctx, []uint{review.MethodID})
	if err != nil {
		return basis, err
	}
	var sample *model.LabSample
	var method *model.AssayMethod
	for i := range samples {
		if samples[i].ID == review.SampleID {
			sample = &samples[i]
		}
	}
	for i := range methods {
		if methods[i].ID == review.MethodID {
			method = &methods[i]
		}
	}
	basis = buildReviewBasis(review, sample, method)
	return basis, nil
}

// buildReviewBasis is the pure decision core of the signing gate: snapshot
// values come from the review, live values from the (possibly nil) sample and
// method pointers. It is shared by reads, signing and unit tests.
func buildReviewBasis(review model.ResultReview, sample *model.LabSample, method *model.AssayMethod) *model.ReviewBasis {
	basis := &model.ReviewBasis{
		SampleID:       review.SampleID,
		SampleCode:     review.SampleCode,
		SampleBatch:    review.SampleBatch,
		SampleSnapshot: review.SampleSnapshot,
		MethodID:       review.MethodID,
		MethodCode:     review.MethodCode,
		MethodSnapshot: review.MethodSnapshot,
		MethodStatus:   review.MethodStatus,
		Reasons:        []string{},
	}
	if sample != nil {
		basis.SampleName = sample.Name
		basis.SampleStatus = sample.Status
	} else if review.SampleID != 0 {
		basis.SampleMissing = true
	}
	if method != nil {
		basis.MethodName = method.Name
		basis.MethodVersion = method.Version
		basis.MethodStatus = method.Status
	} else if review.MethodID != 0 {
		basis.MethodMissing = true
	}
	if review.BasisBlockedAt != nil {
		basis.Locked = true
		if review.BasisBlockedReason != "" {
			basis.LockedReason = review.BasisBlockedReason
		} else {
			basis.LockedReason = "复核依据曾被拦截，旧单留档不可签发"
		}
	}

	// Drafts have no frozen snapshot yet: only evaluate a basis when the review
	// has been submitted to peer review.
	if review.Status != string(constants.ReviewStatePeerReview) {
		basis.Eligible = true
		return basis
	}
	switch {
	case basis.SampleMissing:
		basis.Reasons = append(basis.Reasons, fmt.Sprintf("样本 %s 已不存在，原结论依据失效", review.SampleCode))
	case basis.SampleStatus != string(constants.SampleStateTesting):
		basis.Reasons = append(basis.Reasons, fmt.Sprintf("样本 %s 当前为%s（提交时为%s），不再适用签发",
			review.SampleCode, constants.StatusLabel(basis.SampleStatus), constants.StatusLabel(review.SampleSnapshot)))
	}
	switch {
	case basis.MethodMissing:
		basis.Reasons = append(basis.Reasons, fmt.Sprintf("方法 %s 已不存在，原结论依据失效", review.MethodCode))
	case basis.MethodStatus == string(constants.AssayMethodStateRetired):
		basis.Reasons = append(basis.Reasons, fmt.Sprintf("方法 %s 已退役（提交时 v%d），请使用新版方法补正",
			review.MethodCode, review.MethodSnapshot))
	case basis.MethodVersion != review.MethodSnapshot:
		basis.Reasons = append(basis.Reasons, fmt.Sprintf("方法 %s 已换版（提交时 v%d，当前 v%d）",
			review.MethodCode, review.MethodSnapshot, basis.MethodVersion))
	}
	basis.Eligible = !basis.SampleMissing && !basis.MethodMissing &&
		basis.SampleStatus == string(constants.SampleStateTesting) &&
		basis.MethodStatus == string(constants.AssayMethodStateActive) &&
		basis.MethodVersion == review.MethodSnapshot
	return basis
}

func (s *resultReviewService) attachBasis(ctx context.Context, items []model.ResultReview) error {
	if len(items) == 0 {
		return nil
	}
	sampleIDs := map[uint]struct{}{}
	methodIDs := map[uint]struct{}{}
	for _, item := range items {
		if item.SampleID != 0 {
			sampleIDs[item.SampleID] = struct{}{}
		}
		if item.MethodID != 0 {
			methodIDs[item.MethodID] = struct{}{}
		}
	}
	sampleMap := map[uint]model.LabSample{}
	methodMap := map[uint]model.AssayMethod{}
	if len(sampleIDs) > 0 {
		samples, err := s.samples.ListByIDs(ctx, keysOf(sampleIDs))
		if err != nil {
			return err
		}
		for i := range samples {
			sampleMap[samples[i].ID] = samples[i]
		}
	}
	if len(methodIDs) > 0 {
		methods, err := s.methods.ListByIDs(ctx, keysOf(methodIDs))
		if err != nil {
			return err
		}
		for i := range methods {
			methodMap[methods[i].ID] = methods[i]
		}
	}
	for i := range items {
		var sample *model.LabSample
		var method *model.AssayMethod
		if value, ok := sampleMap[items[i].SampleID]; ok {
			sample = &value
		}
		if value, ok := methodMap[items[i].MethodID]; ok {
			method = &value
		}
		items[i].Basis = buildReviewBasis(items[i], sample, method)
	}
	return nil
}

func keysOf(set map[uint]struct{}) []uint {
	keys := make([]uint, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
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
			return fmt.Errorf("%w: 缺少复核提交人，无法签发", ErrInvalidInput)
		}
		if strings.EqualFold(current.ReviewRequestedBy, actor) {
			return fmt.Errorf("%w: 提交复核者不能签发自己的结果", ErrInvalidInput)
		}
	}
	return nil
}

func validateResultReviewBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
