package hive

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
)

// TestApplyPerCallBudgetFloor verifies the per-call budget floor: the default
// model catalog leaves MaxBudgetUSD=0, which makes claude-cli fall back to its
// $1/call default — too low for an opus implementer Operate. The floor fills the
// unset case; an explicit catalog value (even below the floor) always wins.
func TestApplyPerCallBudgetFloor(t *testing.T) {
	const floor = 10.0
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "unset (0) gets the floor", in: 0, want: floor},
		{name: "negative gets the floor", in: -1, want: floor},
		{name: "catalog value below floor is preserved", in: 2.5, want: 2.5},
		{name: "catalog value above floor is preserved", in: 50, want: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyPerCallBudgetFloor(intelligence.Config{MaxBudgetUSD: tt.in}, floor)
			if got.MaxBudgetUSD != tt.want {
				t.Errorf("applyPerCallBudgetFloor(MaxBudgetUSD=%v) = %v, want %v", tt.in, got.MaxBudgetUSD, tt.want)
			}
		})
	}
}

func TestSpawnAgent_WarnsWhenCanOperateButProviderLacksIOperator(t *testing.T) {
	tests := []struct {
		name          string
		canOperate    bool
		providerOps   bool
		expectWarning bool
	}{
		{name: "CanOperate=true + non-IOperator → warn", canOperate: true, providerOps: false, expectWarning: true},
		{name: "CanOperate=false + non-IOperator → no warn", canOperate: false, providerOps: false, expectWarning: false},
		{name: "CanOperate=true + IOperator → no warn", canOperate: true, providerOps: true, expectWarning: false},
		{name: "CanOperate=false + IOperator → no warn", canOperate: false, providerOps: true, expectWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emitWarning := canOperateMismatch(tt.canOperate, tt.providerOps)
			if emitWarning != tt.expectWarning {
				t.Errorf("canOperateMismatch returned %v; want %v", emitWarning, tt.expectWarning)
			}
		})
	}
}
