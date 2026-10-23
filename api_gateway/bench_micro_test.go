package api_gateway

import (
	"crypto/tls"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Per-handshake certificate lookup: old (parse files every time) vs cached.
func BenchmarkCertLoadPerHandshake(b *testing.B) {
	certFile, keyFile := writeTestCert(b, b.TempDir(), "bench")
	b.Run("old_LoadX509KeyPair", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("new_certCache", func(b *testing.B) {
		var c certCache
		for i := 0; i < b.N; i++ {
			if _, err := c.get(certFile, keyFile); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Basic-auth verification per request: bcrypt (default cost) vs cache hit.
func BenchmarkBasicAuthVerify(b *testing.B) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.DefaultCost)
	b.Run("old_bcrypt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = bcrypt.CompareHashAndPassword(hash, []byte("s3cret"))
		}
	})
	b.Run("new_cached", func(b *testing.B) {
		key := basicAuthKey("r", "u", string(hash), "s3cret")
		basicAuthOK.store(key)
		for i := 0; i < b.N; i++ {
			if !basicAuthOK.hit(key) {
				b.Fatal("miss")
			}
		}
	})
}
