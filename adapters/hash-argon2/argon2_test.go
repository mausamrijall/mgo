package mgoargon2_test

import (
	"strings"
	"testing"

	mgoargon2 "github.com/mgo-framework/mgo/adapters/hash-argon2"
)

func TestHashAndVerify(t *testing.T) {
	h, err := mgoargon2.Hash("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("not a PHC argon2id string: %s", h)
	}
	ok, err := mgoargon2.Verify("hunter2", h)
	if err != nil || !ok {
		t.Fatalf("correct password rejected: %v %v", ok, err)
	}
	ok, err = mgoargon2.Verify("wrong", h)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: %v %v", ok, err)
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := mgoargon2.Hash("same")
	b, _ := mgoargon2.Hash("same")
	if a == b {
		t.Fatal("two hashes of the same password are identical — salt missing")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=18$m=1,t=1,p=1$AA$AA", "plainhash"} {
		if ok, err := mgoargon2.Verify("x", bad); ok || err == nil {
			t.Fatalf("garbage %q verified: %v %v", bad, ok, err)
		}
	}
}

func TestVerifyHonorsEmbeddedParams(t *testing.T) {
	// A hash produced with non-default params must still verify: the
	// params ride in the PHC string, not in code.
	h, err := mgoargon2.HashWith("pw", mgoargon2.Params{Memory: 8192, Iterations: 1, Parallelism: 2, SaltLength: 8, KeyLength: 16})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := mgoargon2.Verify("pw", h); err != nil || !ok {
		t.Fatalf("custom-param hash rejected: %v %v", ok, err)
	}
}
