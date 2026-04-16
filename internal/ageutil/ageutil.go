// Package ageutil wraps filippo.io/age for encrypting/decrypting task payloads.
package ageutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
)

// GenerateKeypair creates a new X25519 identity and writes it to outPath
// (age-keygen format: comment + secret key). Returns the public key string.
func GenerateKeypair(outPath string) (string, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", err
	}
	pub := id.Recipient().String()
	content := fmt.Sprintf(
		"# created: %s\n# public key: %s\n%s\n",
		"",
		pub,
		id.String(),
	)
	if err := os.WriteFile(outPath, []byte(content), 0o600); err != nil {
		return "", err
	}
	return pub, nil
}

// ReadIdentityPubkey parses an identity file and returns its public key.
func ReadIdentityPubkey(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Look for "# public key: age1..."
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# public key:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# public key:")), nil
		}
	}
	// Fallback: derive from secret key
	id, err := parseIdentity(path)
	if err != nil {
		return "", err
	}
	x, ok := id.(*age.X25519Identity)
	if !ok {
		return "", fmt.Errorf("identity is not an X25519 key")
	}
	return x.Recipient().String(), nil
}

func parseIdentity(path string) (age.Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ids, err := age.ParseIdentities(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no identities in %s", path)
	}
	return ids[0], nil
}

// EncryptToRecipients encrypts plaintext for one or more age recipient strings (age1...).
func EncryptToRecipients(plaintext []byte, recipients []string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients given")
	}
	parsed := make([]age.Recipient, 0, len(recipients))
	for _, r := range recipients {
		rec, err := age.ParseX25519Recipient(r)
		if err != nil {
			return nil, fmt.Errorf("parse recipient %q: %w", r, err)
		}
		parsed = append(parsed, rec)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, parsed...)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecryptWithIdentity decrypts ciphertext using the identity file at path.
func DecryptWithIdentity(ciphertext []byte, identityPath string) ([]byte, error) {
	id, err := parseIdentity(identityPath)
	if err != nil {
		return nil, err
	}
	r, err := age.Decrypt(bytes.NewReader(ciphertext), id)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}
