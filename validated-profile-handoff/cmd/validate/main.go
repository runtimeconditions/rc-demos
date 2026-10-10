package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/internal/producer"
	"os"
)

func main() {
	profile := flag.String("profile", "", "Profile artifact")
	catalog := flag.String("catalog", "", "local upstream extension catalog")
	key := flag.String("key", "", "trusted pipeline's private key")
	out := flag.String("out", "", "new bundle directory")
	flag.Parse()
	if *profile == "" || *catalog == "" || *key == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "-profile, -catalog, -key, and -out are required")
		os.Exit(2)
	}
	if err := run(*profile, *catalog, *key, *out); err != nil {
		fmt.Fprintln(os.Stderr, "upstream_validation_failed:", err)
		os.Exit(1)
	}
	fmt.Println("upstream_validated:", *out)
}
func run(profile, catalog, keyPath, out string) error {
	data, err := os.ReadFile(profile)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	a, err := producer.Validate(data, catalog, ed25519.PrivateKey(key), handoff.Digest(binary))
	if err != nil {
		return err
	}
	return a.Write(out)
}
