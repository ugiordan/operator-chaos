// Package fuzz provides a Go fuzz testing harness for operator reconcilers.
//
// It integrates with the SDK's ChaosClient to automatically inject faults
// during reconciliation and verify that operators handle failures correctly.
//
// # Quick Start
//
// Operator teams implement a ReconcilerFactory that constructs their reconciler
// with a given client.Client:
//
//	func myFactory(c client.Client) reconcile.Reconciler {
//	    return &MyReconciler{client: c}
//	}
//
// Then write a Go fuzz test:
//
//	func FuzzMyReconciler(f *testing.F) {
//	    f.Add(uint16(0x01FF), uint8(0), uint16(32768))
//	    f.Fuzz(func(t *testing.T, opMask uint16, faultType uint8, intensity uint16) {
//	        h := fuzz.NewHarness(myFactory, myScheme, myRequest, seedObjects...)
//	        h.AddInvariant(fuzz.ObjectExists(key, &myv1.MyResource{}))
//	        fc := fuzz.DecodeFaultConfig(opMask, faultType, intensity)
//	        if err := h.Run(t, fc); err != nil {
//	            t.Fatal(err)
//	        }
//	    })
//	}
//
// # Relationship to Other SDK Components
//
//   - ChaosClient: The harness wraps a fake client with ChaosClient automatically.
//   - WrapReconciler: Use for integration/runtime chaos; use this package for fuzz testing.
//   - TestChaos: Use for manual test setup; use this package for automated fuzz exploration.
package fuzz
