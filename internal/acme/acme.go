// Package acme reads Traefik's ACME storage (acme.json) to report
// certificate presence and expiry. Read-only: flotilla never writes
// acme.json — Traefik owns it.
//
// On a flotilla-managed VPS the file lives at
// /opt/traefik/letsencrypt/acme.json (see embed/traefik/compose.yml).
package acme

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

// DefaultPath is where install.sh's Traefik bundle stores acme.json.
const DefaultPath = "/opt/traefik/letsencrypt/acme.json"

// Cert is the subset of an ACME certificate flotilla surfaces.
type Cert struct {
	Domain   string
	NotAfter time.Time
}

// acmeFile mirrors Traefik v3 acme.json. Top-level keys are resolver
// names ("le" in the flotilla bundle); we scan all resolvers.
//
//	{
//	  "le": {
//	    "Account": {...},
//	    "Certificates": [
//	      {"domain": {"main": "crm.prvms.ru", "sans": [...]},
//	       "certificate": "<base64 PEM chain>",
//	       "key": "<base64 PEM>"}
//	    ]
//	  }
//	}
type acmeFile map[string]struct {
	Certificates []struct {
		Domain struct {
			Main string   `json:"main"`
			SANs []string `json:"sans"`
		} `json:"domain"`
		Certificate string `json:"certificate"`
	} `json:"Certificates"`
}

// ReadCerts parses acme.json at path and returns one Cert per stored
// certificate (leaf expiry). A missing file returns (nil, nil) — that
// is the normal «no certs issued yet» case.
func ReadCerts(path string) ([]Cert, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read acme.json: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	var af acmeFile
	if err := json.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse acme.json: %w", err)
	}

	var out []Cert
	for _, resolver := range af {
		for _, c := range resolver.Certificates {
			if c.Certificate == "" {
				continue
			}
			notAfter, err := leafNotAfter(c.Certificate)
			if err != nil {
				// A single unparseable cert should not blind the whole
				// report; skip it and continue.
				continue
			}
			out = append(out, Cert{Domain: c.Domain.Main, NotAfter: notAfter})
		}
	}
	return out, nil
}

// FindByDomain returns the certificate whose main domain equals domain,
// or nil if none (no error — «no cert» is a valid state).
func FindByDomain(path, domain string) (*Cert, error) {
	certs, err := ReadCerts(path)
	if err != nil {
		return nil, err
	}
	for i := range certs {
		if certs[i].Domain == domain {
			return &certs[i], nil
		}
	}
	return nil, nil
}

// leafNotAfter decodes a base64-encoded PEM chain (as Traefik stores
// it) and returns the NotAfter of the first (leaf) certificate.
func leafNotAfter(b64 string) (time.Time, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return time.Time{}, fmt.Errorf("base64 decode: %w", err)
	}
	block, _ := pem.Decode(der)
	if block == nil {
		return time.Time{}, errors.New("no PEM block in certificate")
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse x509: %w", err)
	}
	return crt.NotAfter, nil
}
