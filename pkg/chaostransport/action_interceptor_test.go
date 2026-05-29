package chaostransport

import (
	"context"
	"errors"
	"testing"
)

func TestActionInterceptor_NoFaults(t *testing.T) {
	ai := NewActionInterceptor(nil)
	called := false
	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	if err := fn(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("action should be called with no faults")
	}
}

func TestActionInterceptor_Skip(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {Skip: true},
	})

	called := false
	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	if err := fn(context.Background(), nil); err != nil {
		t.Fatalf("skip should return nil: %v", err)
	}
	if called {
		t.Fatal("action should NOT be called when skipped")
	}
}

func TestActionInterceptor_FailBefore(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {FailBefore: "blocked"},
	})

	called := false
	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	err := fn(context.Background(), nil)
	if err == nil {
		t.Fatal("fail-before should return error")
	}
	if called {
		t.Fatal("action should NOT be called on fail-before")
	}
	var ce *ChaosError
	if !errors.As(err, &ce) {
		t.Fatalf("error should be ChaosError, got %T", err)
	}
}

func TestActionInterceptor_FailAfter(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {FailAfter: "partial failure"},
	})

	called := false
	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	err := fn(context.Background(), nil)
	if err == nil {
		t.Fatal("fail-after should return error")
	}
	if !called {
		t.Fatal("action SHOULD be called on fail-after (runs then fails)")
	}
}

func TestActionInterceptor_CaseInsensitiveMatch(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {Skip: true},
	})

	called := false
	fn := ai.Wrap("github.com/example/Deploy.Action", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	fn(context.Background(), nil)
	if called {
		t.Fatal("should match case-insensitively (deploy in Deploy.Action)")
	}
}

func TestActionInterceptor_SubstringMatch(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {Skip: true},
	})

	called := false
	fn := ai.Wrap("github.com/opendatahub-io/operator/v2/pkg/controller/actions/deploy.(*Action).run-fm", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	fn(context.Background(), nil)
	if called {
		t.Fatal("should match substring 'deploy' in full qualified action name")
	}
}

func TestActionInterceptor_NoMatch(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {Skip: true},
	})

	called := false
	fn := ai.Wrap("gc", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	fn(context.Background(), nil)
	if !called {
		t.Fatal("non-matching action should be called normally")
	}
}

func TestActionInterceptor_ErrorRateZero(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {FailBefore: "should not fire", ErrorRate: 0.001},
	})

	hits := 0
	total := 100
	for i := 0; i < total; i++ {
		fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
			return nil
		})
		if err := fn(context.Background(), nil); err != nil {
			hits++
		}
	}

	if hits == total {
		t.Fatal("0.1% rate should not fire every time in 100 tries")
	}
}

func TestActionInterceptor_DefaultErrorRateIs100Percent(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {FailBefore: "always"},
	})

	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		return nil
	})

	for i := 0; i < 10; i++ {
		if err := fn(context.Background(), nil); err == nil {
			t.Fatal("default error rate (1.0) should always fire")
		}
	}
}

func TestActionInterceptor_FailAfterPreservesActionError(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{
		"deploy": {FailAfter: "chaos after"},
	})

	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		return errors.New("action error")
	})

	err := fn(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *ChaosError
	if !errors.As(err, &ce) {
		t.Fatal("fail-after should return ChaosError, overriding action error")
	}
}

func TestActionInterceptor_EmptyFaultsMap(t *testing.T) {
	ai := NewActionInterceptor(map[string]ActionFaultConfig{})

	called := false
	fn := ai.Wrap("deploy", func(ctx context.Context, rr interface{}) error {
		called = true
		return nil
	})

	fn(context.Background(), nil)
	if !called {
		t.Fatal("empty faults map should pass through")
	}
}
