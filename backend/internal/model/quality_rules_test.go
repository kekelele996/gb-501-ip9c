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

func TestReadyForReleaseOnlyConsidersCurrentRound(t *testing.T) {
	batch := validBatch()
	batch.ReworkCount = 1
	// Old samples from the first round must stay visible for traceability but
	// must no longer participate in the release decision.
	batch.Inspections = []InspectionSample{
		{ReworkRound: 0, Result: "fail", RetestStatus: "requested"},
		{ReworkRound: 0, Result: "pass", RetestStatus: "none"},
	}
	if ready, reason := batch.ReadyForRelease(); ready {
		t.Fatalf("new round without samples cannot release: %s", reason)
	}
	batch.Inspections = append(batch.Inspections,
		InspectionSample{ReworkRound: 1, Result: "pending", RetestStatus: "none"})
	if ready, _ := batch.ReadyForRelease(); ready {
		t.Fatal("pending samples in the current round block release")
	}
	batch.Inspections[2].Result = "fail"
	batch.Inspections[2].RetestStatus = "requested"
	if ready, _ := batch.ReadyForRelease(); ready {
		t.Fatal("failed current-round sample blocks release even if old rounds failed too")
	}
	batch.Inspections[2].Result = "pass"
	batch.Inspections[2].RetestStatus = "none"
	if ready, reason := batch.ReadyForRelease(); !ready {
		t.Fatalf("passed current-round samples should release regardless of old rounds: %s", reason)
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
