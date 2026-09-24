package model

import "time"

// ResultReview models 结果复核 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type ResultReview struct {
	BaseModel
	Facility          string    `json:"facility" gorm:"size:120;index"`
	Owner             string    `json:"owner" gorm:"size:120;index"`
	Category          string    `json:"category" gorm:"size:80;index"`
	RiskLevel         string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue       float64   `json:"metricValue"`
	MetricUnit        string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt       time.Time `json:"effectiveAt"`
	Evidence          string    `json:"evidence" gorm:"size:2000"`
	RelatedCode       string    `json:"relatedCode" gorm:"size:64;index"`
	ReviewRequestedBy string    `json:"reviewRequestedBy" gorm:"size:80;index"`
	PeerReviewedBy    string    `json:"peerReviewedBy" gorm:"size:80;index"`
	SignedBy          string    `json:"signedBy" gorm:"size:80;index"`

	// Frozen basis captured when the review is submitted to peer_review. The
	// sample batch/status and method version at that moment are kept forever so
	// later suspensions, disposals or method retirements can be detected against
	// the exact basis the reviewer was asked to sign.
	SampleID       uint   `json:"sampleId" gorm:"index"`
	SampleCode     string `json:"sampleCode" gorm:"size:64;index"`
	SampleBatch    string `json:"sampleBatch" gorm:"size:64;index"`
	SampleSnapshot string `json:"sampleSnapshot" gorm:"size:40"`
	MethodID       uint   `json:"methodId" gorm:"index"`
	MethodCode     string `json:"methodCode" gorm:"size:64;index"`
	MethodSnapshot uint   `json:"methodSnapshot"`
	MethodStatus   string `json:"methodStatus" gorm:"size:40"`

	// BasisBlockedAt/BasisBlockedReason lock a review after a signing attempt
	// found the basis stale. A locked review stays archived as evidence and can
	// never be signed; reviewers must create a fresh review after correction.
	BasisBlockedAt     *time.Time `json:"basisBlockedAt,omitempty" gorm:"index"`
	BasisBlockedReason string     `json:"basisBlockedReason" gorm:"size:500"`

	// Basis is populated on read by the service and never persisted.
	Basis *ReviewBasis `json:"basis,omitempty" gorm:"-"`
}

// ReviewBasis is the live evaluation of the review's signing basis. It carries
// the snapshot recorded at submission, the current sample/method states and the
// blocking reasons (if any) so the workbench can show 依据 and 过期提示.
type ReviewBasis struct {
	SampleID       uint     `json:"sampleId"`
	SampleCode     string   `json:"sampleCode"`
	SampleName     string   `json:"sampleName"`
	SampleBatch    string   `json:"sampleBatch"`
	SampleSnapshot string   `json:"sampleSnapshot"`
	SampleStatus   string   `json:"sampleStatus"`
	SampleMissing  bool     `json:"sampleMissing"`
	MethodID       uint     `json:"methodId"`
	MethodCode     string   `json:"methodCode"`
	MethodName     string   `json:"methodName"`
	MethodSnapshot uint     `json:"methodSnapshot"`
	MethodVersion  uint     `json:"methodVersion"`
	MethodStatus   string   `json:"methodStatus"`
	MethodMissing  bool     `json:"methodMissing"`
	Eligible       bool     `json:"eligible"`
	Reasons        []string `json:"reasons"`
	Locked         bool     `json:"locked"`
	LockedReason   string   `json:"lockedReason,omitempty"`
}

func (item *ResultReview) GetBase() *BaseModel { return &item.BaseModel }

func (item ResultReview) TableName() string { return "result_reviews" }

var ResultReviewInitialStatus = "draft"
