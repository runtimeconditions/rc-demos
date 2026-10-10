package main

import (
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"os"
)

func main() {
	bundle := flag.String("bundle", "", "received bundle directory")
	trusted := flag.String("trusted-key", "", "locally configured public key (outside bundle)")
	devBind := flag.String("dev-bind", "", "existing development consumer executable")
	envOut := flag.String("env-out", "", "optional environment fulfillment output")
	flag.Parse()
	if *bundle == "" || *trusted == "" || *devBind == "" {
		fmt.Fprintln(os.Stderr, "-bundle, -trusted-key, and -dev-bind are required")
		os.Exit(2)
	}
	key, err := os.ReadFile(*trusted)
	if err != nil || len(key) != ed25519.PublicKeySize {
		_ = json.NewEncoder(os.Stdout).Encode(handoff.Result{Status: "trust_configuration_error", Detail: "cannot read configured Ed25519 public key"})
		os.Exit(1)
	}
	result := handoff.Consume(*bundle, ed25519.PublicKey(key), *devBind, *envOut)
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if result.Status == "unsupported" {
		os.Exit(3)
	}
	if result.Status != "supported" {
		os.Exit(1)
	}
}
