package httpapi

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"socialnet/internal/config"
	"strings"
	"testing"
)

func TestKubernetesMonitor(t *testing.T) {
	token := "secret-test-token"
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("bad authentication/method")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/pods") {
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"api-1"},"spec":{"nodeName":"node-1","containers":[{}],"secret":"do-not-return"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":2}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer api.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	tok := filepath.Join(dir, "token")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: api.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tok, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Config{KubeURL: api.URL, KubeCAFile: ca, KubeTokenFile: tok, KubeNamespace: "socialnet"}}
	rr := httptest.NewRecorder()
	s.handleKubernetes(rr, httptest.NewRequest("GET", "/", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `"restarts":2`) || strings.Contains(body, token) || strings.Contains(body, "do-not-return") {
		t.Fatal(body)
	}
	if _, _, err := kubeClient("http://localhost", ca, tok); err == nil {
		t.Fatal("HTTP accepted")
	}
	s.cfg.KubeURL = ""
	rr = httptest.NewRecorder()
	s.handleKubernetes(rr, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatal(rr.Body.String())
	}
}
