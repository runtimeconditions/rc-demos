package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/kratixgate"
)

func main() {
	input := flag.String("input", "/kratix/input/object.yaml", "Kratix input request")
	key := flag.String("trusted-key", "/trust/trusted.pub", "operator-provisioned public key")
	output := flag.String("output", "/handoff/verified/object.yaml", "verified request on shared volume")
	metadata := flag.String("metadata", "/kratix/metadata", "Kratix workflow metadata")
	termination := flag.String("termination-message", "/dev/termination-log", "Kubernetes container termination message")
	flag.Parse()
	err := kratixgate.Run(*input, *key, *output)
	result := handoff.Result{Status: "verified", HandoffVerified: true, Detail: "verified Profile published for consumer"}
	if err != nil {
		result = handoff.Result{Status: "handoff_execution_failed", Detail: err.Error()}
		var failure *handoff.Failure
		if errors.As(err, &failure) {
			result.Status = failure.Category
		}
	}
	status, _ := json.Marshal(map[string]any{"handoff": result, "message": result.Status})
	// A failed init container prevents Kratix's normal status writer from starting.
	// Keep the category in Pod termination status as well as logs and metadata.
	brief, _ := json.Marshal(map[string]any{"status": result.Status, "handoff_verified": result.HandoffVerified, "support_evaluated": false})
	if writeErr := os.WriteFile(*termination, brief, 0644); writeErr != nil {
		fmt.Fprintln(os.Stderr, "cannot publish termination status:", writeErr)
		os.Exit(1)
	}
	// JSON is valid YAML. On success the resolver preserves this handoff status.
	if writeErr := os.WriteFile(filepath.Join(*metadata, "status.yaml"), status, 0644); writeErr != nil {
		fmt.Fprintln(os.Stderr, "cannot publish workflow status:", writeErr)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		os.Exit(1)
	}
}
