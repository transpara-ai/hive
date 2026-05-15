package hive

import "testing"

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
