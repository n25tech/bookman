package auth

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "correct horse") {
		t.Fatal("password should verify")
	}
	if CheckPassword(h, "wrong horse") || CheckPassword("garbage", "x") {
		t.Fatal("wrong password verified")
	}
}

func TestToken(t *testing.T) {
	tok, hash, err := NewToken()
	if err != nil || tok == "" || HashToken(tok) != hash || hash == tok {
		t.Fatalf("bad token %q %q %v", tok, hash, err)
	}
}
