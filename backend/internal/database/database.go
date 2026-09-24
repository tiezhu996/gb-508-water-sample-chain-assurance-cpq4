package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/config"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(ctx context.Context, cfg config.Config, log *slog.Logger) (*gorm.DB, *redis.Client, error) {
	var dialector gorm.Dialector
	switch cfg.DatabaseDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseDSN)
	case "mysql":
		dialector = mysql.Open(cfg.DatabaseDSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabaseDSN)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
	logLevel := logger.Warn
	if cfg.Environment == "development" {
		logLevel = logger.Info
	}
	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 20; attempt++ {
		db, err = gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logLevel)})
		if err == nil {
			sqlDB, dbErr := db.DB()
			if dbErr == nil && sqlDB.PingContext(ctx) == nil {
				break
			}
			if dbErr != nil {
				err = dbErr
			} else {
				err = sqlDB.PingContext(ctx)
			}
		}
		log.Warn("database not ready", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	if err := migrate(db); err != nil {
		return nil, nil, err
	}
	if err := Seed(ctx, db); err != nil {
		return nil, nil, err
	}
	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return nil, nil, fmt.Errorf("connect redis: %w", err)
		}
	}
	return db, redisClient, nil
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{}, &model.AuditLog{},
		&model.SamplingBatch{},
		&model.LabSample{},
		&model.AssayMethod{},
		&model.ResultReview{},
	)
}

func Seed(ctx context.Context, db *gorm.DB) error {
	var users int64
	if err := db.WithContext(ctx).Model(&model.User{}).Count(&users).Error; err != nil {
		return err
	}
	if users == 0 {
		password, err := bcrypt.GenerateFromPassword([]byte("Admin123!"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		seedUsers := []model.User{
			{Username: "admin", DisplayName: "系统管理员", PasswordHash: string(password), Role: model.RoleAdmin, Active: true},
			{Username: "reviewer", DisplayName: "质量复核员", PasswordHash: string(password), Role: model.RoleReviewer, Active: true},
			{Username: "operator", DisplayName: "现场操作员", PasswordHash: string(password), Role: model.RoleOperator, Active: true},
		}
		if err := db.WithContext(ctx).Create(&seedUsers).Error; err != nil {
			return err
		}
	}

	if err := seedSamplingBatch(ctx, db); err != nil {
		return err
	}

	samples, err := seedLabSample(ctx, db)
	if err != nil {
		return err
	}

	methods, err := seedAssayMethod(ctx, db)
	if err != nil {
		return err
	}

	if err := seedResultReview(ctx, db, samples, methods); err != nil {
		return err
	}

	return nil
}

func seedSamplingBatch(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.SamplingBatch{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	items := []model.SamplingBatch{

		{BaseModel: model.BaseModel{Code: "SB-001", Name: "采样批次示例一", Status: "planned", Version: 1,
			Description: "用于启动验证和主要流程演示的采样批次记录"}, Facility: "水质检测样本链路审核区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-508-01"},

		{BaseModel: model.BaseModel{Code: "SB-002", Name: "采样批次示例二", Status: "collecting", Version: 1,
			Description: "用于启动验证和主要流程演示的采样批次记录"}, Facility: "水质检测样本链路审核区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-508-02"},

		{BaseModel: model.BaseModel{Code: "SB-003", Name: "采样批次示例三", Status: "received", Version: 1,
			Description: "用于启动验证和主要流程演示的采样批次记录"}, Facility: "水质检测样本链路审核区域3", Owner: "安全主管组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-508-03"},
	}
	return db.WithContext(ctx).Create(&items).Error
}

func seedLabSample(ctx context.Context, db *gorm.DB) ([]model.LabSample, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.LabSample{}).Count(&count).Error; err != nil || count > 0 {
		return nil, err
	}
	now := time.Now().UTC()
	items := []model.LabSample{

		{BaseModel: model.BaseModel{Code: "LS-001", Name: "实验室样本示例一", Status: "received", Version: 1,
			Description: "用于启动验证和主要流程演示的实验室样本记录"}, Facility: "水质检测样本链路审核区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "SB-001"},

		{BaseModel: model.BaseModel{Code: "LS-002", Name: "实验室样本示例二", Status: "accepted", Version: 1,
			Description: "用于启动验证和主要流程演示的实验室样本记录"}, Facility: "水质检测样本链路审核区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "SB-002"},

		{BaseModel: model.BaseModel{Code: "LS-003", Name: "饮用水总磷在检样本", Status: "testing", Version: 2,
			Description: "已完成检测、等待结果复核的在检样本"}, Facility: "水质检测样本链路审核区域3", Owner: "分析一组",
			Category: "复核", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "SB-003"},

		{BaseModel: model.BaseModel{Code: "LS-004", Name: "地表水氨氮在检样本", Status: "testing", Version: 1,
			Description: "可用于新建复核单的在检样本"}, Facility: "水质检测样本链路审核区域2", Owner: "分析二组",
			Category: "常规", RiskLevel: "medium", MetricValue: 8.2, MetricUnit: "mg/L",
			EffectiveAt: now.Add(2 * time.Hour), Evidence: "冷链与保存条件均合格", RelatedCode: "SB-002"},

		{BaseModel: model.BaseModel{Code: "LS-005", Name: "实验室样本暂停示例", Status: "hold", Version: 1,
			Description: "提交复核后被暂停的样本，用于演示过期依据拦截"}, Facility: "水质检测样本链路审核区域1", Owner: "质量复核组",
			Category: "重点", RiskLevel: "high", MetricValue: 19.4, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-5 * time.Hour), Evidence: "复检等待中", RelatedCode: "SB-001"},
	}
	if err := db.WithContext(ctx).Create(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func seedAssayMethod(ctx context.Context, db *gorm.DB) ([]model.AssayMethod, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.AssayMethod{}).Count(&count).Error; err != nil || count > 0 {
		return nil, err
	}
	now := time.Now().UTC()
	items := []model.AssayMethod{

		{BaseModel: model.BaseModel{Code: "AM-001", Name: "检测方法示例一", Status: "draft", Version: 1,
			Description: "用于启动验证和主要流程演示的检测方法记录"}, Facility: "水质检测样本链路审核区域1", Owner: "运行一组",
			Category: "常规", RiskLevel: "low", MetricValue: 12.5, MetricUnit: "unit",
			EffectiveAt: now.Add(0 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-508-01"},

		{BaseModel: model.BaseModel{Code: "AM-002", Name: "检测方法示例二", Status: "validated", Version: 1,
			Description: "用于启动验证和主要流程演示的检测方法记录"}, Facility: "水质检测样本链路审核区域2", Owner: "质量复核组",
			Category: "重点", RiskLevel: "medium", MetricValue: 25.0, MetricUnit: "%",
			EffectiveAt: now.Add(3 * time.Hour), Evidence: "已完成基础证据核对", RelatedCode: "REL-508-02"},

		{BaseModel: model.BaseModel{Code: "GB-11893", Name: "水质 总磷的测定 钼酸铵分光光度法", Status: "active", Version: 1,
			Description: "现行有效版本，可作为在检样本的复核依据"}, Facility: "水质检测样本链路审核区域3", Owner: "技术负责人组",
			Category: "国标", RiskLevel: "high", MetricValue: 37.5, MetricUnit: "score",
			EffectiveAt: now.Add(6 * time.Hour), Evidence: "已完成方法验证与批准", RelatedCode: "GB/T 11893"},

		{BaseModel: model.BaseModel{Code: "HJ-535", Name: "水质 氨氮的测定 纳氏试剂分光光度法", Status: "active", Version: 2,
			Description: "现行 v2，适用于地表水氨氮检测"}, Facility: "水质检测样本链路审核区域2", Owner: "技术负责人组",
			Category: "行标", RiskLevel: "medium", MetricValue: 8.2, MetricUnit: "mg/L",
			EffectiveAt: now.Add(2 * time.Hour), Evidence: "换版后重新验证通过", RelatedCode: "HJ 535"},

		{BaseModel: model.BaseModel{Code: "GB-7479-OLD", Name: "水质 阴离子表面活性剂测定（旧版）", Status: "retired", Version: 2,
			Description: "已退役旧版本，仅用于演示方法退役拦截"}, Facility: "水质检测样本链路审核区域1", Owner: "技术负责人组",
			Category: "国标", RiskLevel: "high", MetricValue: 1.4, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-24 * time.Hour), Evidence: "被新版标准替代后退役", RelatedCode: "GB 7479"},
	}
	if err := db.WithContext(ctx).Create(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func seedResultReview(ctx context.Context, db *gorm.DB, samples []model.LabSample, methods []model.AssayMethod) error {
	var count int64
	if err := db.WithContext(ctx).Model(&model.ResultReview{}).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	now := time.Now().UTC()
	sampleByCode := map[string]model.LabSample{}
	methodByCode := map[string]model.AssayMethod{}
	for _, item := range samples {
		sampleByCode[item.Code] = item
	}
	for _, item := range methods {
		methodByCode[item.Code] = item
	}
	testing1 := sampleByCode["LS-003"]
	testing2 := sampleByCode["LS-004"]
	hold := sampleByCode["LS-005"]
	active1 := methodByCode["GB-11893"]
	active2 := methodByCode["HJ-535"]
	retired := methodByCode["GB-7479-OLD"]
	blockedAt := now.Add(-2 * time.Hour)

	lockedReason := "方法 GB-7479-OLD 已退役（提交时 v2），请使用新版方法补正"
	items := []model.ResultReview{

		// 有效待复核: 在检样本 + 现行方法，签发前核对可通过。
		{BaseModel: model.BaseModel{Code: "RR-001", Name: "总磷结果复核（依据有效）", Status: "peer_review", Version: 1,
			Description: "提交时冻结的样本与方法依据仍然有效，可由复核员签发"}, Facility: "水质检测样本链路审核区域3", Owner: "分析一组",
			Category: "国标", RiskLevel: "high", MetricValue: 0.42, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-1 * time.Hour), Evidence: "平行样与加标回收均合格", RelatedCode: active1.Code,
			ReviewRequestedBy: "operator",
			SampleID:          testing1.ID, SampleCode: testing1.Code, SampleBatch: testing1.RelatedCode, SampleSnapshot: "testing",
			MethodID: active1.ID, MethodCode: active1.Code, MethodSnapshot: active1.Version, MethodStatus: "active"},

		// 实时过期: 样本提交后被暂停，复核员点签发时当场拦截并留存。
		{BaseModel: model.BaseModel{Code: "RR-002", Name: "表面活性剂结果复核（样本已暂停）", Status: "peer_review", Version: 1,
			Description: "提交复核后样本被暂停，旧结论不再适用，签发将被拦截"}, Facility: "水质检测样本链路审核区域1", Owner: "质量复核组",
			Category: "重点", RiskLevel: "high", MetricValue: 1.4, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-6 * time.Hour), Evidence: "等待复核期间样本转入暂停", RelatedCode: active2.Code,
			ReviewRequestedBy: "operator",
			SampleID:          hold.ID, SampleCode: hold.Code, SampleBatch: hold.RelatedCode, SampleSnapshot: "testing",
			MethodID: active2.ID, MethodCode: active2.Code, MethodSnapshot: active2.Version, MethodStatus: "active"},

		// 已签发历史单，用于展示双人复核签发链。
		{BaseModel: model.BaseModel{Code: "RR-003", Name: "氨氮结果复核（已签发）", Status: "signed", Version: 2,
			Description: "已完成双人复核签发的历史记录"}, Facility: "水质检测样本链路审核区域2", Owner: "分析二组",
			Category: "行标", RiskLevel: "medium", MetricValue: 0.31, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-48 * time.Hour), Evidence: "质控数据齐全", RelatedCode: active2.Code,
			ReviewRequestedBy: "operator", PeerReviewedBy: "reviewer", SignedBy: "reviewer",
			SampleID: testing2.ID, SampleCode: testing2.Code, SampleBatch: testing2.RelatedCode, SampleSnapshot: "testing",
			MethodID: active2.ID, MethodCode: active2.Code, MethodSnapshot: active2.Version, MethodStatus: "active"},

		// 已留存锁定: 此前签发被拦截，旧单留档、不可再签，补正后需新建。
		{BaseModel: model.BaseModel{Code: "RR-004", Name: "旧版方法结果复核（已拦截留档）", Status: "peer_review", Version: 2,
			Description: "方法退役导致签发被拦截，单据留存作为审核证据，不允许再签发"}, Facility: "水质检测样本链路审核区域1", Owner: "质量复核组",
			Category: "国标", RiskLevel: "high", MetricValue: 0.88, MetricUnit: "mg/L",
			EffectiveAt: now.Add(-3 * time.Hour), Evidence: "旧方法结论，等待新方法补正", RelatedCode: retired.Code,
			ReviewRequestedBy: "operator",
			SampleID:          testing1.ID, SampleCode: testing1.Code, SampleBatch: testing1.RelatedCode, SampleSnapshot: "testing",
			MethodID: retired.ID, MethodCode: retired.Code, MethodSnapshot: retired.Version, MethodStatus: "retired",
			BasisBlockedAt: &blockedAt, BasisBlockedReason: lockedReason},
	}
	return db.WithContext(ctx).Create(&items).Error
}
