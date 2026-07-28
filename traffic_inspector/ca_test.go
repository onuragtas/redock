package traffic_inspector

import (
	"crypto/x509"
	"testing"

	"redock/platform/memory"
)

func newTestDB(t *testing.T) *memory.Database {
	t.Helper()

	db, err := memory.NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("memory.NewDatabase: %v", err)
	}
	if err := memory.Register[*CAEntity](db, CATableName); err != nil {
		t.Fatalf("memory.Register: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLoadOrCreateCAManager_GeneratesAndPersists(t *testing.T) {
	db := newTestDB(t)

	ca, err := LoadOrCreateCAManager(db)
	if err != nil {
		t.Fatalf("LoadOrCreateCAManager: %v", err)
	}
	if !ca.rootCert.IsCA {
		t.Fatalf("generated root cert is not marked as CA")
	}

	entities := memory.FindAll[*CAEntity](db, CATableName)
	if len(entities) != 1 {
		t.Fatalf("expected 1 persisted CA entity, got %d", len(entities))
	}

	// Loading again against the same DB should reuse the persisted CA, not
	// generate a second one.
	ca2, err := LoadOrCreateCAManager(db)
	if err != nil {
		t.Fatalf("LoadOrCreateCAManager (reload): %v", err)
	}
	if !ca.rootCert.Equal(ca2.rootCert) {
		t.Fatalf("reloaded CA cert does not match the persisted one")
	}

	entitiesAfter := memory.FindAll[*CAEntity](db, CATableName)
	if len(entitiesAfter) != 1 {
		t.Fatalf("expected still 1 persisted CA entity after reload, got %d", len(entitiesAfter))
	}
}

func TestLoadOrCreateCAManager_RegeneratesUnparseableEntity(t *testing.T) {
	db := newTestDB(t)

	// Simulate a CA persisted by an older build using a different key
	// algorithm (this package switched from ECDSA to RSA) — the stored PEM
	// blocks parse as neither a valid cert nor a valid RSA key here.
	bogus := &CAEntity{CertPEM: "not a real cert", KeyPEM: "not a real key"}
	if err := memory.Create[*CAEntity](db, CATableName, bogus); err != nil {
		t.Fatalf("seed bogus CA entity: %v", err)
	}

	ca, err := LoadOrCreateCAManager(db)
	if err != nil {
		t.Fatalf("LoadOrCreateCAManager should self-heal instead of erroring: %v", err)
	}
	if !ca.rootCert.IsCA {
		t.Fatalf("regenerated root cert is not marked as CA")
	}

	entities := memory.FindAll[*CAEntity](db, CATableName)
	if len(entities) != 1 {
		t.Fatalf("expected the bogus entity to be replaced in place (1 row), got %d", len(entities))
	}
	if entities[0].ID != bogus.ID {
		t.Fatalf("expected regenerated CA to reuse entity ID %d, got %d", bogus.ID, entities[0].ID)
	}
	if entities[0].CertPEM == "not a real cert" {
		t.Fatalf("expected bogus CertPEM to be overwritten")
	}
}

func TestLeafCertificate_IssuedAndCached(t *testing.T) {
	db := newTestDB(t)

	ca, err := LoadOrCreateCAManager(db)
	if err != nil {
		t.Fatalf("LoadOrCreateCAManager: %v", err)
	}

	leaf, err := ca.LeafCertificate("example.com")
	if err != nil {
		t.Fatalf("LeafCertificate: %v", err)
	}

	leafCert, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatalf("parse issued leaf cert: %v", err)
	}
	if leafCert.Subject.CommonName != "example.com" {
		t.Fatalf("leaf cert CommonName = %q, want example.com", leafCert.Subject.CommonName)
	}
	if len(leafCert.DNSNames) != 1 || leafCert.DNSNames[0] != "example.com" {
		t.Fatalf("leaf cert DNSNames = %v, want [example.com]", leafCert.DNSNames)
	}

	// The leaf must chain up to and verify against the root CA.
	pool := x509.NewCertPool()
	pool.AddCert(ca.rootCert)
	if _, err := leafCert.Verify(x509.VerifyOptions{DNSName: "example.com", Roots: pool}); err != nil {
		t.Fatalf("leaf cert does not verify against root CA: %v", err)
	}

	// A second request for the same hostname must be served from cache
	// (same certificate bytes, not a freshly re-issued one).
	leaf2, err := ca.LeafCertificate("example.com")
	if err != nil {
		t.Fatalf("LeafCertificate (cached): %v", err)
	}
	if string(leaf.Certificate[0]) != string(leaf2.Certificate[0]) {
		t.Fatalf("expected cached leaf certificate to be reused, got a different certificate")
	}
}

func TestLeafCertificate_IPHostname(t *testing.T) {
	db := newTestDB(t)

	ca, err := LoadOrCreateCAManager(db)
	if err != nil {
		t.Fatalf("LoadOrCreateCAManager: %v", err)
	}

	leaf, err := ca.LeafCertificate("192.0.2.1")
	if err != nil {
		t.Fatalf("LeafCertificate: %v", err)
	}

	leafCert, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatalf("parse issued leaf cert: %v", err)
	}
	if len(leafCert.IPAddresses) != 1 || leafCert.IPAddresses[0].String() != "192.0.2.1" {
		t.Fatalf("leaf cert IPAddresses = %v, want [192.0.2.1]", leafCert.IPAddresses)
	}
}
