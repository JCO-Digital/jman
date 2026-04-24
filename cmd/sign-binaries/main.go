package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
)

// This script is used in the CI pipeline to sign the built binaries using
// the private key stored in a GitHub Secret.
//
// Usage: MINISIGN_PRIVATE_KEY=<base64_priv_key> go run bin/sign_binaries.go <binary_path1> <binary_path2> ...

func main() {
	privKeyBase64 := os.Getenv("MINISIGN_PRIVATE_KEY")
	if privKeyBase64 == "" {
		log.Fatal("MINISIGN_PRIVATE_KEY environment variable is not set")
	}

	privKeyBytes, err := base64.StdEncoding.DecodeString(privKeyBase64)
	if err != nil {
		log.Fatalf("failed to decode private key: %v", err)
	}

	if len(privKeyBytes) != ed25519.PrivateKeySize {
		log.Fatalf("invalid private key size: expected %d, got %d", ed25519.PrivateKeySize, len(privKeyBytes))
	}

	privKey := ed25519.PrivateKey(privKeyBytes)

	if len(os.Args) < 2 {
		log.Fatal("usage: go run bin/sign_binaries.go <file1> <file2> ...")
	}

	for _, path := range os.Args[1:] {
		if err := signFile(path, privKey); err != nil {
			log.Fatalf("failed to sign %s: %v", path, err)
		}
		fmt.Printf("Successfully signed %s -> %s.minisig\n", path, path)
	}
}

func signFile(path string, privKey ed25519.PrivateKey) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Generate the Ed25519 signature
	signature := ed25519.Sign(privKey, content)

	// Encode signature to base64 for the signature file
	sigBase64 := base64.StdEncoding.EncodeToString(signature)

	// Write signature to <path>.minisig
	sigPath := path + ".minisig"
	return os.WriteFile(sigPath, []byte(sigBase64), 0644)
}
