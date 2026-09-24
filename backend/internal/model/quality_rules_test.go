package model

import (
	"testing"
	"time"

	"sterile-packaging-release-control/backend/internal/constants"
)

func validBatch() ProductionBatch {
	return ProductionBatch{
		BatchNo: "B20260822-TEST", Specification: "无菌屏障袋", Status: constants.BatchStatusRunning,
		ResponsibleTeam: "验证班", PackagingLineID: 1, PlannedQuantity: 1000, ProducedQuantity: 800,
	}
}

func TestBatchReadyForRelease(t *testing.T) {
	batch := validBatch()
	if ready, _ := batch.ReadyForRelease(); ready {
		t.Fatal("batch without inspections cannot release")
	}
	batch.Inspections = []InspectionSample{{Result: "pass", RetestStatus: "none"}}
	if ready, reason := batch.ReadyForRelease(); !ready {
		t.Fatalf("passed batch should release: %s", reason)
	}
	batch.Inspections = append(batch.Inspections, InspectionSample{Result: "fail", RetestStatus: "requested"})
	if ready, _ := batch.ReadyForRelease(); ready {
		t.Fatal("failed batch cannot release")
	}
}

func TestReadyForReleaseOnlyConsidersCurrentReworkRound(t *testing.T) {
	batch := validBatch()
	// 首轮的不合格结果在进入返工后只用于追溯，不应再阻止放行。
	batch.EnterRework("密封强度不达标")
	if batch.Status != constants.BatchStatusRework || batch.ReworkRound != 1 {
		t.Fatalf("enter rework failed: status=%s round=%d", batch.Status, batch.ReworkRound)
	}
	batch.Inspections = []InspectionSample{
		{ReworkRound: 0, Result: "fail", RetestStatus: "completed"},
	}
	if ready, reason := batch.ReadyForRelease(); ready || reason != "at least one inspection is required" {
		t.Fatalf("new round without samples must block release: ready=%v reason=%s", ready, reason)
	}
	batch.Inspections = append(batch.Inspections,
		InspectionSample{ReworkRound: 1, Result: "pass", RetestStatus: "none"},
		InspectionSample{ReworkRound: 1, Result: "pending", RetestStatus: "none"},
	)
	if ready, reason := batch.ReadyForRelease(); ready || reason != "pending inspections remain" {
		t.Fatalf("pending sample of current round must block: ready=%v reason=%s", ready, reason)
	}
	batch.Inspections[2].Result = "fail"
	batch.Inspections[2].RetestStatus = "requested"
	if ready, reason := batch.ReadyForRelease(); ready || reason != "failed inspections remain" {
		t.Fatalf("failed sample of current round must block: ready=%v reason=%s", ready, reason)
	}
	batch.Inspections[2].Result = "pass"
	batch.Inspections[2].RetestStatus = "completed"
	if ready, reason := batch.ReadyForRelease(); !ready {
		t.Fatalf("all current-round samples passing should release despite old fail: %s", reason)
	}
}

func TestCurrentRoundSummaryCountsCurrentRoundOnly(t *testing.T) {
	batch := validBatch()
	batch.ReworkRound = 2
	batch.Inspections = []InspectionSample{
		{ReworkRound: 0, Result: "fail", RetestStatus: "completed"},
		{ReworkRound: 1, Result: "fail", RetestStatus: "requested"},
		{ReworkRound: 2, Result: "pass", RetestStatus: "none"},
		{ReworkRound: 2, Result: "pending", RetestStatus: "none"},
	}
	total, passed, failed, pending, retest := batch.CurrentRoundSummary()
	if total != 2 || passed != 1 || failed != 0 || pending != 1 || retest != 0 {
		t.Fatalf("unexpected current-round summary: total=%d pass=%d fail=%d pending=%d retest=%d", total, passed, failed, pending, retest)
	}
}

func TestInspectionValidation(t *testing.T) {
	now := time.Now()
	sample := InspectionSample{
		ProductionBatchID: 1, SampleCode: "SAMPLE-001", SamplingPosition: "中段",
		InspectionItem: "热封强度", Result: "pass", MeasuredValue: "1.7 N",
		AcceptanceRange: ">= 1.5 N", RetestStatus: "none", InspectedAt: &now,
	}
	if err := sample.ValidateDefinition(); err != nil {
		t.Fatalf("valid sample rejected: %v", err)
	}
	sample.Result = "pending"
	if err := sample.ValidateDefinition(); err == nil {
		t.Fatal("pending sample with timestamp must fail")
	}
}
