package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"sterile-packaging-release-control/backend/internal/constants"
	"sterile-packaging-release-control/backend/internal/dto"
	"sterile-packaging-release-control/backend/internal/model"
	"sterile-packaging-release-control/backend/internal/repository"
)

type mockBatchRepository struct {
	batch *model.ProductionBatch
}

func (r *mockBatchRepository) List(context.Context, repository.BatchFilter) ([]model.ProductionBatch, int64, error) {
	return nil, 0, nil
}
func (r *mockBatchRepository) Find(context.Context, uint) (*model.ProductionBatch, error) {
	return r.batch, nil
}
func (r *mockBatchRepository) FindForUpdate(context.Context, uint) (*model.ProductionBatch, error) {
	return r.batch, nil
}
func (r *mockBatchRepository) FindByNumber(context.Context, string) (*model.ProductionBatch, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *mockBatchRepository) Create(context.Context, *model.ProductionBatch) error { return nil }
func (r *mockBatchRepository) Save(context.Context, *model.ProductionBatch) error   { return nil }
func (r *mockBatchRepository) Overview(context.Context) (*dto.QualityOverview, error) {
	return nil, nil
}

type mockReleaseRepository struct {
	decision *model.ReleaseDecision
}

func (r *mockReleaseRepository) List(context.Context, dto.PageQuery, string) ([]model.ReleaseDecision, int64, error) {
	return nil, 0, nil
}
func (r *mockReleaseRepository) Find(context.Context, uint) (*model.ReleaseDecision, error) {
	return r.decision, nil
}
func (r *mockReleaseRepository) LatestForBatch(context.Context, uint) (*model.ReleaseDecision, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *mockReleaseRepository) CreateWithBatch(_ context.Context, decision *model.ReleaseDecision, _ *model.ProductionBatch) error {
	r.decision = decision
	return nil
}

type mockInspectionRepository struct {
	sample *model.InspectionSample
}

func (r *mockInspectionRepository) List(context.Context, repository.InspectionFilter) ([]model.InspectionSample, int64, error) {
	return nil, 0, nil
}
func (r *mockInspectionRepository) Find(context.Context, uint) (*model.InspectionSample, error) {
	return r.sample, nil
}
func (r *mockInspectionRepository) FindForUpdate(context.Context, uint) (*model.InspectionSample, error) {
	return r.sample, nil
}
func (r *mockInspectionRepository) FindByCode(context.Context, string) (*model.InspectionSample, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *mockInspectionRepository) Create(_ context.Context, sample *model.InspectionSample) error {
	sample.ID = 1
	r.sample = sample
	return nil
}
func (r *mockInspectionRepository) Save(context.Context, *model.InspectionSample) error { return nil }

func reworkBatch() *model.ProductionBatch {
	batch := &model.ProductionBatch{
		BatchNo: "B-REWORK-001", Specification: "无菌屏障袋", Status: constants.BatchStatusRework,
		ResponsibleTeam: "甲班", PackagingLineID: 1, PlannedQuantity: 100, ProducedQuantity: 100,
		ReworkRound: 1,
	}
	batch.ID = 42
	return batch
}

func newBatch(id uint, batchNo string, status constants.BatchStatus) *model.ProductionBatch {
	batch := &model.ProductionBatch{
		BatchNo: batchNo, Specification: "无菌屏障袋", Status: status,
		ResponsibleTeam: "甲班", PackagingLineID: 1, PlannedQuantity: 100, ProducedQuantity: 100,
	}
	batch.ID = id
	return batch
}

func releaseActor() Actor { return Actor{ID: 9, Name: "approver"} }

func TestReleaseBlocksWithoutCurrentRoundSamples(t *testing.T) {
	tx := &trackingTransactor{}
	batch := reworkBatch()
	batch.Inspections = []model.InspectionSample{
		{ReworkRound: 0, Result: "pass", RetestStatus: "none"},
	}
	batchRepo := &mockBatchRepository{batch: batch}
	svc := NewReleaseService(&mockReleaseRepository{}, batchRepo, failingAuditService{tx: tx}, tx)
	_, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
		ProductionBatchID: batch.ID, Decision: constants.DecisionRelease, Reason: "首轮全部合格申请放行",
	})
	if err == nil {
		t.Fatal("release without samples of the current round must be rejected")
	}
	if batch.Status != constants.BatchStatusRework {
		t.Fatalf("batch status must not change after rejection, got %s", batch.Status)
	}
}

func TestReleaseBlocksWhenCurrentRoundIncompleteOrFailed(t *testing.T) {
	tx := &trackingTransactor{}
	blockingResults := []model.InspectionSample{
		{ReworkRound: 1, Result: "pending", RetestStatus: "none"},
		{ReworkRound: 1, Result: "fail", RetestStatus: "requested"},
		{ReworkRound: 1, Result: "fail", RetestStatus: "none"},
	}
	for index, sample := range blockingResults {
		batch := reworkBatch()
		batch.Inspections = []model.InspectionSample{
			{ReworkRound: 0, Result: "fail", RetestStatus: "completed"},
			sample,
		}
		svc := NewReleaseService(&mockReleaseRepository{}, &mockBatchRepository{batch: batch}, failingAuditService{tx: tx}, tx)
		_, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
			ProductionBatchID: batch.ID, Decision: constants.DecisionRelease, Reason: "本轮检验尚未闭环不能放行",
		})
		if err == nil {
			t.Fatalf("case %d: current-round sample %s/%s must block release", index, sample.Result, sample.RetestStatus)
		}
	}
}

func TestReleaseUsesOnlyCurrentRoundPassingSamples(t *testing.T) {
	tx := &trackingTransactor{}
	batch := reworkBatch()
	batch.Inspections = []model.InspectionSample{
		{ReworkRound: 0, Result: "fail", RetestStatus: "completed"},
		{ReworkRound: 1, Result: "pass", RetestStatus: "completed"},
	}
	batchRepo := &mockBatchRepository{batch: batch}
	releaseRepo := &mockReleaseRepository{}
	svc := NewReleaseService(releaseRepo, batchRepo, failingAuditService{tx: tx}, tx)
	decision, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
		ProductionBatchID: batch.ID, Decision: constants.DecisionRelease, Reason: "返工后重新检验全部合格",
	})
	if err != nil {
		t.Fatalf("release should pass with all current-round samples passing: %v", err)
	}
	if batch.Status != constants.BatchStatusReleased {
		t.Fatalf("batch should be released, got %s", batch.Status)
	}
	if decision.InspectionSummary == "" {
		t.Fatal("decision should carry an inspection summary")
	}
}

func TestQuarantineIgnoresInspectionGate(t *testing.T) {
	tx := &trackingTransactor{}
	batch := reworkBatch()
	svc := NewReleaseService(&mockReleaseRepository{}, &mockBatchRepository{batch: batch}, failingAuditService{tx: tx}, tx)
	if _, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
		ProductionBatchID: batch.ID, Decision: constants.DecisionQuarantine, Reason: "等待进一步质量调查",
	}); err != nil {
		t.Fatalf("quarantine must not depend on inspection gate: %v", err)
	}
	if batch.Status != constants.BatchStatusHold || batch.ReworkRound != 1 {
		t.Fatalf("unexpected state after quarantine: status=%s round=%d", batch.Status, batch.ReworkRound)
	}
}

func TestReworkDecisionOpensNewInspectionRound(t *testing.T) {
	tx := &trackingTransactor{}
	batch := newBatch(43, "B-REWORK-002", constants.BatchStatusRunning)
	svc := NewReleaseService(&mockReleaseRepository{}, &mockBatchRepository{batch: batch}, failingAuditService{tx: tx}, tx)
	if _, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
		ProductionBatchID: batch.ID, Decision: constants.DecisionRework, Reason: "末段密封强度低于内控限",
	}); err != nil {
		t.Fatalf("rework decision failed: %v", err)
	}
	if batch.Status != constants.BatchStatusRework || batch.ReworkRound != 1 {
		t.Fatalf("rework should open round 1: status=%s round=%d", batch.Status, batch.ReworkRound)
	}
	// 已经处于返工时重复提交返工决定，不应再增加轮次。
	if _, err := svc.Decide(context.Background(), releaseActor(), dto.CreateReleaseDecisionRequest{
		ProductionBatchID: batch.ID, Decision: constants.DecisionRework, Reason: "返工方案需要再次确认",
	}); err != nil {
		t.Fatalf("second rework decision failed: %v", err)
	}
	if batch.ReworkRound != 1 {
		t.Fatalf("repeated rework decision must not open another round, got %d", batch.ReworkRound)
	}
}

func TestBatchTransitionToReworkOpensNewRound(t *testing.T) {
	tx := &trackingTransactor{}
	batch := newBatch(44, "B-REWORK-003", constants.BatchStatusHold)
	batch.HoldReason = "等待处置"
	batchRepo := &mockBatchRepository{batch: batch}
	svc := NewBatchService(batchRepo, nil, failingAuditService{tx: tx}, tx)
	if _, err := svc.Transition(context.Background(), Actor{ID: 2, Name: "operator"}, batch.ID, constants.BatchStatusRework, "产线安排返工"); err != nil {
		t.Fatalf("transition to rework failed: %v", err)
	}
	if batch.Status != constants.BatchStatusRework || batch.ReworkRound != 1 {
		t.Fatalf("transition should open round 1: status=%s round=%d", batch.Status, batch.ReworkRound)
	}
}

func TestNewSampleIsRegisteredToCurrentRound(t *testing.T) {
	tx := &trackingTransactor{}
	batch := reworkBatch()
	inspectionRepo := &mockInspectionRepository{}
	svc := NewInspectionService(inspectionRepo, &mockBatchRepository{batch: batch}, failingAuditService{tx: tx}, tx)
	sample, err := svc.Create(context.Background(), Actor{ID: 3, Name: "inspector"}, dto.CreateInspectionRequest{
		ProductionBatchID: batch.ID, SampleCode: "S-R1-MID-01", SamplingPosition: "返工后中段",
		InspectionItem: "热封强度", AcceptanceRange: ">= 1.50 N/15mm",
	})
	if err != nil {
		t.Fatalf("sample registration failed: %v", err)
	}
	if sample.ReworkRound != 1 {
		t.Fatalf("new sample must be filed under current round 1, got %d", sample.ReworkRound)
	}
}

func TestHistoricalRoundSampleCannotBeCompleted(t *testing.T) {
	tx := &trackingTransactor{}
	batch := reworkBatch()
	sample := &model.InspectionSample{
		ProductionBatchID: batch.ID, SampleCode: "S-R0-OLD", SamplingPosition: "首轮末段",
		InspectionItem: "热封强度", AcceptanceRange: ">= 1.50 N/15mm",
		Result: "fail", RetestStatus: "completed", ReworkRound: 0,
	}
	inspectionRepo := &mockInspectionRepository{sample: sample}
	svc := NewInspectionService(inspectionRepo, &mockBatchRepository{batch: batch}, failingAuditService{tx: tx}, tx)
	_, err := svc.Complete(context.Background(), Actor{ID: 3, Name: "inspector"}, sample.ID, dto.CompleteInspectionRequest{
		Result: "pass", MeasuredValue: "1.8 N/15mm",
	})
	if err == nil {
		t.Fatal("completing a historical-round sample must be rejected")
	}
}
