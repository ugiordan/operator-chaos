package fuzz

import (
	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// allOperations is the ordered list of operations (bit position = index).
var allOperations = []sdk.Operation{
	sdk.OpGet,
	sdk.OpList,
	sdk.OpCreate,
	sdk.OpUpdate,
	sdk.OpDelete,
	sdk.OpPatch,
	sdk.OpDeleteAllOf,
	sdk.OpReconcile,
	sdk.OpApply,
}

// faultMessages maps faultType values to realistic K8s error messages.
var faultMessages = []string{
	"the object has been modified; please apply your changes to the latest version and try again", // 0: conflict
	"not found",                          // 1: not-found
	"context deadline exceeded",          // 2: timeout
	"Internal error occurred",            // 3: server-error
	"etcd leader changed",               // 4: etcd
	"too many requests",                  // 5: throttle
	"connection refused",                 // 6: connection
	"object has been deleted",            // 7: gone
	"admission webhook denied request",   // 8: webhook
	"resource quota exceeded",            // 9: quota
	"service unavailable",               // 10: unavailable
}

// DecodeFaultConfig maps Go fuzz engine primitives to a valid FaultConfig.
//   - opMask: bitmask selecting which operations get faults (bit0=Get, bit1=List, ...)
//   - faultType: selects error message (indexes into faultMessages table)
//   - intensity: maps to error rate (0-65535 -> 0.0-1.0)
func DecodeFaultConfig(opMask uint16, faultType uint8, intensity uint16) *sdk.FaultConfig {
	faults := make(map[sdk.Operation]sdk.FaultSpec)

	errMsg := faultMessages[int(faultType)%len(faultMessages)]
	errorRate := float64(intensity) / 65535.0

	for i, op := range allOperations {
		if opMask&(1<<uint(i)) != 0 {
			faults[op] = sdk.FaultSpec{
				ErrorRate: errorRate,
				Error:     errMsg,
			}
		}
	}

	return sdk.NewFaultConfig(faults)
}
