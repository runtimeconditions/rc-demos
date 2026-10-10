// keygen provisions a fresh experiment trust anchor outside the artifact bundle.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("out", "", "new private trust directory")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(out string) error {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.Mkdir(out, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "signer.key"), key, 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "trusted.pub"), pub, 0644)
}
