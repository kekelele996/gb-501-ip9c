package service

import (
	"context"
	"errors"
	"testing"

	"sterile-packaging-release-control/backend/internal/constants"
	"sterile-packaging-release-control/backend/internal/dto"
	"sterile-packaging-release-control/backend/internal/model"
	"sterile-packaging-release-control/backend/internal/repository"
	"sterile-packaging-release-control/backend/internal/util"
)

// roundAudit records nothing and always succeeds.
type roundAudit struct{}

func (roundAudit) Record(context.Context, Actor, string, string, uint, any, any) error { return nil }
func (roundAudit) List(context.Context, repository.AuditFilter) (dto.PageResult[model.AuditLog], error) {
	return dto.PageResult[model.AuditLog]{}, nil
}

type roundBatchRepo struct {
	batches map[uint]*model.ProductionBatch
}

func (r *roundBatchRepo) List(context.Context, repository.BatchFilter) ([]model.ProductionBatch, int64, error) {
	return nil, 0, nil
}
func (r *roundBatchRepo) Find(ctx context.Context, id uint) (*model.ProductionBatch, error) {
	batch, ok := r.batches[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return batch, nil
}
func (r *roundBatchRepo) FindForUpdate(ctx context.Context, id uint) (*model.ProductionBatch, error) {
	return r.Find(ctx, id)
}
func (r *roundBatchRepo) FindByNumber(context.Context, string) (*model.ProductionBatch, error) {
	return nil, errors.New("not found")
}
func (r *roundBatchRepo) Create(context.Context, *model.ProductionBatch) error { return nil }
func (r *roundBatchRepo) Save(_ context.Context, batch *model.ProductionBatch) error {
	r.batches[batch.ID] = batch
	return nil
}
func (r *roundBatchRepo) Overview(context.Context) (*dto.QualityOverview, error) {
	return nil, nil
}

type roundReleaseRepo struct {
	batches   map[uint]*model.ProductionBatch
	decisions []model.ReleaseDecision
}

func (r *roundReleaseRepo) List(context.Context, dto.PageQuery, string) ([]model.ReleaseDecision, int64, error) {
	return r.decisions, int64(len(r.decisions)), nil
}
func (r *roundReleaseRepo) Find(_ context.Context, id uint) (*model.ReleaseDecision, error) {
	for i := range r.decisions {
		if r.decisions[i].ID == id {
			return &r.decisions[i], nil
		}
	}
	return nil, errors.New("not found")
}
func (r *roundReleaseRepo) LatestForBatch(_ context.Context, batchID uint) (*model.ReleaseDecision, error) {
	for i := len(r.decisions) - 1; i >= 0; i-- {
		if r.decisions[i].ProductionBatchID == batchID {
			return &r.decisions[i], nil
		}
	}
	return nil, errors.New("not found")
}
func (r *roundReleaseRepo) CreateWithBatch(_ context.Context, decision *model.ReleaseDecision, batch *model.ProductionBatch) error {
	decision.ID = uint(len(r.decisions) + 1)
	r.decisions = append(r.decisions, *decision)
	r.batches[batch.ID] = batch
	return nil
}

func newRoundService() (*releaseService, *roundBatchRepo, *roundReleaseRepo) {
	batches := map[uint]*model.ProductionBatch{
		1: {
			Base:             model.Base{ID: 1},
			BatchNo:          "B-ROUND-01",
			Status:           constants.BatchStatusHold,
			Specification:    "无菌屏障袋",
			ResponsibleTeam:  "甲班",
			PackagingLineID:  1,
			PlannedQuantity:  100,
			ProducedQuantity: 100,
			ReworkCount:      0,
			Inspections: []model.InspectionSample{
				{Base: model.Base{ID: 1}, ProductionBatchID: 1, ReworkRound: 0, Result: "fail", RetestStatus: "requested", AcceptanceRange: ">= 1.5", SampleCode: "S-OLD-FAIL"},
				{Base: model.Base{ID: 2}, ProductionBatchID: 1, ReworkRound: 0, Result: "pass", RetestStatus: "none", AcceptanceRange: ">= 1.5", SampleCode: "S-OLD-PASS"},
			},
		},
	}
	batchRepo := &roundBatchRepo{batches: batches}
	releaseRepo := &roundReleaseRepo{batches: batches}
	svc := &releaseService{repo: releaseRepo, batchRepo: batchRepo, audit: roundAudit{}, tx: &trackingTransactor{}}
	return svc, batchRepo, releaseRepo
}

const validReason = "依据质量手册第 8 章执行处置"

func TestReleaseRequiresFreshSamplesAfterRework(t *testing.T) {
	svc, batchRepo, releaseRepo := newRoundService()
	actor := Actor{ID: 9, Name: "approver", RequestID: "req-rework"}

	// Initial release is blocked because round 0 samples are not clean.
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionRelease, Reason: validReason,
	}); !isConflict(err) {
		t.Fatalf("expected conflict release with dirty round, got %v", err)
	}

	// Rework decision succeeds despite dirty inspections and opens round 1.
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionRework, Reason: validReason,
	}); err != nil {
		t.Fatalf("rework decision must not be blocked by inspections: %v", err)
	}
	batch := batchRepo.batches[1]
	if batch.Status != constants.BatchStatusRework || batch.ReworkCount != 1 {
		t.Fatalf("after rework: status=%s count=%d, want rework/1", batch.Status, batch.ReworkCount)
	}

	// Old samples must not support the new release: no fresh sample yet.
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionRelease, Reason: validReason,
	}); !isConflict(err) {
		t.Fatalf("expected conflict releasing with no current-round sample, got %v", err)
	}

	// A fresh round-1 sample that is still pending also blocks release.
	batch.Inspections = append(batch.Inspections, model.InspectionSample{
		Base: model.Base{ID: 3}, ProductionBatchID: 1, ReworkRound: 1,
		Result: "pending", RetestStatus: "none", AcceptanceRange: ">= 1.5", SampleCode: "S-NEW-PENDING",
	})
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionRelease, Reason: validReason,
	}); !isConflict(err) {
		t.Fatalf("pending current-round sample must block release, got %v", err)
	}

	// Completed and clean round-1 sample allows release; old failures remain.
	batch.Inspections[2].Result = "pass"
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionRelease, Reason: validReason,
	}); err != nil {
		t.Fatalf("clean current-round samples should release even with old failures: %v", err)
	}
	if batch.Status != constants.BatchStatusReleased {
		t.Fatalf("status=%s, want released", batch.Status)
	}
	if last := releaseRepo.decisions[len(releaseRepo.decisions)-1]; last.ReworkRound != 1 {
		t.Fatalf("release decision round=%d, want 1", last.ReworkRound)
	}
}

func TestQuarantineUnaffectedByInspectionState(t *testing.T) {
	svc, batchRepo, _ := newRoundService()
	actor := Actor{ID: 9, Name: "approver", RequestID: "req-quarantine"}
	if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
		ProductionBatchID: 1, Decision: constants.DecisionQuarantine, Reason: validReason,
	}); err != nil {
		t.Fatalf("quarantine must not be affected by inspection results: %v", err)
	}
	if batch := batchRepo.batches[1]; batch.Status != constants.BatchStatusHold || batch.ReworkCount != 0 {
		t.Fatalf("quarantine changed status=%s count=%d", batch.Status, batch.ReworkCount)
	}
}

func TestReworkDecisionWhileAlreadyInReworkDoesNotAdvanceRound(t *testing.T) {
	svc, batchRepo, _ := newRoundService()
	actor := Actor{ID: 9, Name: "approver", RequestID: "req-rework-again"}
	for i, decision := range []constants.DecisionType{constants.DecisionRework, constants.DecisionRework} {
		if _, err := svc.Decide(context.Background(), actor, dto.CreateReleaseDecisionRequest{
			ProductionBatchID: 1, Decision: decision, Reason: validReason,
		}); err != nil {
			t.Fatalf("rework decision %d failed: %v", i, err)
		}
	}
	if count := batchRepo.batches[1].ReworkCount; count != 1 {
		t.Fatalf("rework count=%d after two rework decisions, want 1", count)
	}
}

func isConflict(err error) bool {
	var apiErr *util.APIError
	return errors.As(err, &apiErr) && apiErr.Status == 409
}
