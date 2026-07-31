package kie

import "fmt"

func recoverAsError(report func(error)) {
	if value := recover(); value != nil {
		report(fmt.Errorf("kie: panic recovered: %v", value))
	}
}
