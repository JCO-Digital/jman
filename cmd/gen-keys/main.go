package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
)

// This script generates a new Ed25519 key pair for signing jman binaries.
// The public key should be hardcoded in the app, and the private key should
// be stored securely (e.g., as a GitHub Secret).

func main() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatal(err)
	}

	pubBase64 := base64.StdEncoding.EncodeToString(pub)
	privBase64 := base64.StdEncoding.EncodeToString(priv)

	fmt.Println("Generated new key pair for jman signing:")
	fmt.Println()
	fmt.Printf("Public Key (for internal/update/keys.go):\n%s\n", pubBase64)
	fmt.Println()
	fmt.Printf("Private Key (for GitHub Secret MINISIGN_PRIVATE_KEY):\n%s\n", privBase64)
	fmt.Println()
	fmt.Println("Keep the private key secret! If it is compromised, you must rotate the keys.")
}
