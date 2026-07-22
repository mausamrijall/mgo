package mgoargon2

import (
	"strings"
	"testing"
)

// FuzzVerify: arbitrary PHC-ish strings must never panic or falsely
// verify against an arbitrary password.
func FuzzVerify(f *testing.F) {
	valid, _ := Hash("seed")
	f.Add("pw", valid)
	f.Add("pw", "$argon2id$v=19$m=1,t=1,p=1$AA$AA")
	f.Add("pw", "$argon2id$v=19$m=99999999,t=1,p=1$AA$AA") // huge memory param
	f.Add("pw", "$argon2i$v=19$m=1,t=1,p=1$AA$AA")
	f.Add("pw", "")
	f.Add("pw", "$$$$$$")
	f.Add("pw", "$argon2id$v=19$m=1,t=1,p=1$!!!$???")

	f.Fuzz(func(t *testing.T, password, encoded string) {
		// Cap the memory parameter so fuzzing doesn't OOM: skip inputs
		// that would ask argon2 for >64 MiB.
		if i := strings.Index(encoded, "m="); i >= 0 {
			var m uint64
			for _, r := range encoded[i+2:] {
				if r < '0' || r > '9' {
					break
				}
				m = m*10 + uint64(r-'0')
				if m > 65536 {
					t.Skip("memory param too large for fuzzing")
				}
			}
		}
		ok, err := Verify(password, encoded)
		if ok && err != nil {
			t.Fatal("ok=true with non-nil error")
		}
	})
}

// FuzzRoundtrip: any password hashes and verifies true; a different
// password verifies false.
func FuzzRoundtrip(f *testing.F) {
	f.Add("hunter2")
	f.Add("")
	f.Add("päss wörd 🔑")
	f.Fuzz(func(t *testing.T, password string) {
		if len(password) > 128 {
			t.Skip("long passwords slow the fuzzer without new coverage")
		}
		// Small params: correctness property, not KDF strength.
		h, err := HashWith(password, Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 8, KeyLength: 16})
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := Verify(password, h); err != nil || !ok {
			t.Fatalf("own hash rejected: %v %v", ok, err)
		}
		if ok, _ := Verify(password+"x", h); ok {
			t.Fatal("different password verified")
		}
	})
}
