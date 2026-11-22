package httpapi

import (
	"fmt"
	"math"
	"testing"
)

func TestNearby(t *testing.T) {
	ts, s, _ := setupFull(t, 1000)
	a, b := register(t, ts, "anna"), register(t, ts, "bob")
	location := map[string]any{"lat": 55.123456, "lon": 60.654321, "consent": true}
	a.req("POST", p("/nearby"), map[string]any{"lat": 100, "lon": 0, "consent": true}, 422)
	a.req("POST", p("/nearby"), map[string]any{"lat": 0, "lon": 0}, 422)
	a.req("POST", p("/nearby"), map[string]any{"consent": true}, 422)
	a.req("POST", p("/nearby"), location, 200)
	if got := b.req("GET", p("/nearby"), nil, 200); got["visible"] != false || len(got["people"].([]any)) != 0 {
		t.Fatal(got)
	}
	b.req("POST", p("/nearby"), location, 200)
	got := a.req("GET", p("/nearby"), nil, 200)
	if len(got["people"].([]any)) != 1 {
		t.Fatal(got)
	}
	person := got["people"].([]any)[0].(map[string]any)
	if _, ok := person["lat"]; ok {
		t.Fatal("coordinates exposed")
	}
	var lat float64
	if err := s.store.Pool().QueryRow(t.Context(), `SELECT lat FROM nearby_presence WHERE user_id=$1`, a.ID).Scan(&lat); err != nil || lat != 55.12 {
		t.Fatal(lat, err)
	}
	b.req("POST", p(fmt.Sprintf("/users/%d/block", a.ID)), nil, 204)
	if got := a.req("GET", p("/nearby"), nil, 200); len(got["people"].([]any)) != 0 {
		t.Fatal(got)
	}
	b.req("DELETE", p(fmt.Sprintf("/users/%d/block", a.ID)), nil, 204)
	if _, err := s.store.Pool().Exec(t.Context(), `UPDATE nearby_presence SET expires_at=now()-interval '1 second' WHERE user_id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if got := a.req("GET", p("/nearby"), nil, 200); len(got["people"].([]any)) != 0 {
		t.Fatal(got)
	}
	a.req("DELETE", p("/nearby"), nil, 200)
	if got := a.req("GET", p("/nearby"), nil, 200); got["visible"] != false {
		t.Fatal(got)
	}
}

func TestNearbyCoordinates(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 91, -91} {
		if validNearbyCoordinates(v, 0) {
			t.Fatal(v)
		}
	}
	if !validNearbyCoordinates(0, 0) || validNearbyCoordinates(0, 181) {
		t.Fatal("bounds")
	}
}
