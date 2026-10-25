package webpush

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"
)

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Контрольный пример из RFC 8291, Appendix A.
func TestRFC8291Vector(t *testing.T) {
	plain := []byte("When I grow up, I want to be a watermelon")
	asPriv, err := ecdh.P256().NewPrivateKey(mustB64(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	uaPriv, err := ecdh.P256().NewPrivateKey(mustB64(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	if err != nil {
		t.Fatal(err)
	}
	uaPub := mustB64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	salt := mustB64(t, "DGv6ra1nlYgDCS1FRnbzlw")
	auth := mustB64(t, "BTBZMqHH6r4Tts7J_aSIgg")
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"

	if string(uaPriv.PublicKey().Bytes()) != string(uaPub) {
		t.Fatal("test vector keys mismatch")
	}
	got, err := encrypt(plain, uaPub, auth, asPriv, salt)
	if err != nil {
		t.Fatal(err)
	}
	if b64.EncodeToString(got) != want {
		t.Fatalf("encryption does not match RFC 8291:\n got %s\nwant %s", b64.EncodeToString(got), want)
	}
	dec, err := Decrypt(mustB64(t, want), uaPriv, auth)
	if err != nil || string(dec) != string(plain) {
		t.Fatalf("decrypt: %q %v", dec, err)
	}
}

func TestRoundTripAndVAPID(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	body, err := Encrypt([]byte(`{"title":"привет"}`), ua.PublicKey().Bytes(), auth)
	if err != nil {
		t.Fatal(err)
	}
	if pt, err := Decrypt(body, ua, auth); err != nil || string(pt) != `{"title":"привет"}` {
		t.Fatalf("roundtrip: %q %v", pt, err)
	}

	pub, priv, _ := GenerateVAPID()
	v, err := NewVAPID(pub, priv, "mailto:a@b.c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.authHeader("https://fcm.googleapis.com/fcm/send/x"); err != nil {
		t.Fatal(err)
	}
	pub2, _, _ := GenerateVAPID()
	if _, err := NewVAPID(pub2, priv, ""); err == nil {
		t.Fatal("mismatched VAPID keys accepted")
	}
}

func TestEndpointSSRF(t *testing.T) {
	ok := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/abc",
		"https://db5p.notify.windows.com/w/?token=abc",
	}
	bad := []string{
		"http://fcm.googleapis.com/fcm/send/abc",        // не https
		"https://169.254.169.254/latest/meta-data/",     // метаданные облака
		"https://localhost/admin",                       // свой сервер
		"https://db:5432/",                              // внутренний сервис
		"https://fcm.googleapis.com.evil.com/x",         // похожий домен
		"https://evilfcm.googleapis.com@attacker.com/x", // userinfo
		"https://fcm.googleapis.com:8443/x",             // нестандартный порт
		"https://notify.windows.com.attacker.net/x",
		"javascript:alert(1)",
	}
	for _, e := range ok {
		if err := ValidateEndpoint(e, DefaultHosts); err != nil {
			t.Errorf("rejected valid %s", e)
		}
	}
	for _, e := range bad {
		if err := ValidateEndpoint(e, DefaultHosts); err == nil {
			t.Errorf("accepted dangerous %s", e)
		}
	}
}
