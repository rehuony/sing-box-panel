// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// IdentityFile in the data directory holds the private key and self-signed
// certificate the panel presents to nodes that connect to it directly.
const IdentityFile = "node-tls.pem"

// Identity is the panel's TLS certificate for direct node connections and
// the pin that enrollment codes carry.
type Identity struct {
	Certificate tls.Certificate
	Pin         string
}

// LoadOrCreateIdentity reads the panel's node TLS identity in dataDir and
// creates it the first time. Concurrent first callers share one identity.
func LoadOrCreateIdentity(dataDir string) (Identity, error) {
	path := filepath.Join(dataDir, IdentityFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		generated, generateErr := generateIdentity(time.Now())
		if generateErr != nil {
			return Identity{}, generateErr
		}
		if err := atomicfile.Write(path, generated, 0o600, atomicfile.Options{Exclusive: true}); err != nil && !errors.Is(err, os.ErrExist) {
			return Identity{}, fmt.Errorf("create node TLS identity: %w", err)
		}
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return Identity{}, fmt.Errorf("read node TLS identity: %w", err)
	}
	certificate, err := tls.X509KeyPair(raw, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("parse node TLS identity %s: %w", path, err)
	}
	return Identity{Certificate: certificate, Pin: nodeapi.PublicKeyPin(certificate.Leaf.RawSubjectPublicKeyInfo)}, nil
}

// generateIdentity returns a new P-256 key and a self-signed certificate in
// PEM. Nodes pin the key, so the certificate's names and lifetime carry no
// trust.
func generateIdentity(now time.Time) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "sing-box-panel"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(100, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})
	return append(encoded, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})...), nil
}
