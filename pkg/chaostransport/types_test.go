package chaostransport

import (
	"errors"
	"sync"
	"testing"
)

func TestFaultConfig_NilIsInactive(t *testing.T) {
	var fc *FaultConfig
	if fc.IsActive() {
		t.Fatal("nil FaultConfig should be inactive")
	}
}

func TestFaultConfig_NilMaybeInjectReturnsNil(t *testing.T) {
	var fc *FaultConfig
	if err := fc.MaybeInject(OpGet); err != nil {
		t.Fatalf("nil FaultConfig should not inject: %v", err)
	}
}

func TestFaultConfig_NewFaultConfigIsActive(t *testing.T) {
	fc := NewFaultConfig(nil)
	if !fc.IsActive() {
		t.Fatal("NewFaultConfig should be active by default")
	}
}

func TestFaultConfig_ActivateDeactivate(t *testing.T) {
	fc := NewFaultConfig(nil)
	fc.Deactivate()
	if fc.IsActive() {
		t.Fatal("should be inactive after Deactivate")
	}
	fc.Activate()
	if !fc.IsActive() {
		t.Fatal("should be active after Activate")
	}
}

func TestFaultConfig_MaybeInject_ActiveNoFaults(t *testing.T) {
	fc := NewFaultConfig(nil)
	if err := fc.MaybeInject(OpGet); err != nil {
		t.Fatalf("no faults configured should return nil: %v", err)
	}
}

func TestFaultConfig_MaybeInject_InactiveWithFaults(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "should not fire"},
	})
	fc.Deactivate()
	if err := fc.MaybeInject(OpGet); err != nil {
		t.Fatalf("inactive config should not inject: %v", err)
	}
}

func TestFaultConfig_MaybeInject_100Percent(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "always fail"},
	})
	err := fc.MaybeInject(OpGet)
	if err == nil {
		t.Fatal("100% error rate should always inject")
	}
	var ce *ChaosError
	if !errors.As(err, &ce) {
		t.Fatalf("error should be ChaosError, got %T", err)
	}
	if ce.Operation != OpGet {
		t.Fatalf("expected operation OpGet, got %s", ce.Operation)
	}
}

func TestFaultConfig_MaybeInject_ZeroPercent(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 0.0, Error: "never fail"},
	})
	for i := 0; i < 100; i++ {
		if err := fc.MaybeInject(OpGet); err != nil {
			t.Fatalf("0%% error rate should never inject: %v", err)
		}
	}
}

func TestFaultConfig_MaybeInject_UnmatchedOperation(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpCreate: {ErrorRate: 1.0, Error: "create only"},
	})
	if err := fc.MaybeInject(OpGet); err != nil {
		t.Fatalf("unmatched operation should not inject: %v", err)
	}
}

func TestFaultConfig_MaybeInject_Probabilistic(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 0.5, Error: "half the time"},
	})

	hits := 0
	total := 1000
	for i := 0; i < total; i++ {
		if err := fc.MaybeInject(OpGet); err != nil {
			hits++
		}
	}

	if hits == 0 {
		t.Fatal("50% rate produced 0 hits in 1000 tries")
	}
	if hits == total {
		t.Fatal("50% rate produced 1000/1000 hits")
	}
}

func TestFaultConfig_SetFault(t *testing.T) {
	fc := NewFaultConfig(nil)
	fc.SetFault(OpGet, FaultSpec{ErrorRate: 1.0, Error: "added"})

	err := fc.MaybeInject(OpGet)
	if err == nil {
		t.Fatal("SetFault should add a fault that fires")
	}
}

func TestFaultConfig_RemoveFault(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 1.0, Error: "will remove"},
	})
	fc.RemoveFault(OpGet)

	if err := fc.MaybeInject(OpGet); err != nil {
		t.Fatalf("removed fault should not inject: %v", err)
	}
}

func TestFaultConfig_ConcurrentAccess(t *testing.T) {
	fc := NewFaultConfig(map[Operation]FaultSpec{
		OpGet: {ErrorRate: 0.5, Error: "concurrent"},
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			fc.MaybeInject(OpGet)
		}()
		go func() {
			defer wg.Done()
			fc.SetFault(OpCreate, FaultSpec{ErrorRate: 1.0, Error: "new"})
		}()
		go func() {
			defer wg.Done()
			fc.IsActive()
		}()
	}
	wg.Wait()
}

func TestChaosError_Format(t *testing.T) {
	err := &ChaosError{Operation: OpGet, Message: "test error"}
	expected := "chaos(get): test error"
	if err.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, err.Error())
	}
}

func TestChaosError_ErrorsAs(t *testing.T) {
	err := &ChaosError{Operation: OpGet, Message: "test"}
	var ce *ChaosError
	if !errors.As(err, &ce) {
		t.Fatal("errors.As should match ChaosError")
	}
}
