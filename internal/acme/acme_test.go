package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeCertB64 produces a self-signed cert for `cn` expiring at
// `notAfter`, PEM-encoded then base64'd the way Traefik stores it.
func makeCertB64(t *testing.T, cn string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBytes)
}

func writeAcmeJSON(t *testing.T, dir string, certs map[string]time.Time) string {
	t.Helper()
	type domain struct {
		Main string `json:"main"`
	}
	type cert struct {
		Domain      domain `json:"domain"`
		Certificate string `json:"certificate"`
	}
	resolver := struct {
		Certificates []cert `json:"Certificates"`
	}{}
	for cn, exp := range certs {
		resolver.Certificates = append(resolver.Certificates, cert{
			Domain:      domain{Main: cn},
			Certificate: makeCertB64(t, cn, exp),
		})
	}
	doc := map[string]any{"le": resolver}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "acme.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadCerts_MissingFile(t *testing.T) {
	certs, err := ReadCerts(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if certs != nil {
		t.Errorf("missing file → %v, want nil", certs)
	}
}

func TestReadCerts_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "acme.json")
	_ = os.WriteFile(p, nil, 0o600)
	certs, err := ReadCerts(p)
	if err != nil {
		t.Fatalf("empty file: %v", err)
	}
	if certs != nil {
		t.Errorf("empty file → %v, want nil", certs)
	}
}

func TestReadCerts_ParsesExpiry(t *testing.T) {
	dir := t.TempDir()
	exp := time.Now().Add(60 * 24 * time.Hour).UTC().Truncate(time.Second)
	path := writeAcmeJSON(t, dir, map[string]time.Time{
		"crm.prvms.ru": exp,
	})

	certs, err := ReadCerts(path)
	if err != nil {
		t.Fatalf("ReadCerts: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("len = %d, want 1", len(certs))
	}
	if certs[0].Domain != "crm.prvms.ru" {
		t.Errorf("Domain = %q", certs[0].Domain)
	}
	if !certs[0].NotAfter.Equal(exp) {
		t.Errorf("NotAfter = %v, want %v", certs[0].NotAfter, exp)
	}
}

func TestFindByDomain(t *testing.T) {
	dir := t.TempDir()
	exp := time.Now().Add(30 * 24 * time.Hour)
	path := writeAcmeJSON(t, dir, map[string]time.Time{
		"a.example.com": exp,
		"b.example.com": exp,
	})

	got, err := FindByDomain(path, "b.example.com")
	if err != nil {
		t.Fatalf("FindByDomain: %v", err)
	}
	if got == nil || got.Domain != "b.example.com" {
		t.Errorf("FindByDomain(b) = %+v, want b.example.com", got)
	}

	none, err := FindByDomain(path, "absent.example.com")
	if err != nil {
		t.Fatalf("FindByDomain: %v", err)
	}
	if none != nil {
		t.Errorf("FindByDomain(absent) = %+v, want nil", none)
	}
}

func TestReadCerts_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "acme.json")
	_ = os.WriteFile(p, []byte("{not json"), 0o600)
	if _, err := ReadCerts(p); err == nil {
		t.Fatal("corrupt acme.json should error")
	}
}

func TestReadCerts_SkipsUnparseableCert(t *testing.T) {
	dir := t.TempDir()
	doc := map[string]any{
		"le": map[string]any{
			"Certificates": []map[string]any{
				{"domain": map[string]string{"main": "bad.example.com"}, "certificate": "not-base64-or-pem"},
			},
		},
	}
	data, _ := json.Marshal(doc)
	p := filepath.Join(dir, "acme.json")
	_ = os.WriteFile(p, data, 0o600)

	certs, err := ReadCerts(p)
	if err != nil {
		t.Fatalf("ReadCerts: %v", err)
	}
	if len(certs) != 0 {
		t.Errorf("unparseable cert should be skipped, got %v", certs)
	}
}
