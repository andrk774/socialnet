package httpapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type kubePod struct {
	Name       string   `json:"name"`
	Phase      string   `json:"phase"`
	Node       string   `json:"node"`
	Ready      int      `json:"ready"`
	Containers int      `json:"containers"`
	Restarts   int      `json:"restarts"`
	Reasons    []string `json:"reasons"`
}
type kubeDeployment struct {
	Name    string `json:"name"`
	Desired int    `json:"desired"`
	Ready   int    `json:"ready"`
	Updated int    `json:"updated"`
}
type kubeEvent struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
	Object string `json:"object"`
	Count  int    `json:"count"`
	At     string `json:"at"`
}
type kubeState struct {
	Enabled     bool             `json:"enabled"`
	Namespace   string           `json:"namespace"`
	At          time.Time        `json:"at"`
	Pods        []kubePod        `json:"pods"`
	Deployments []kubeDeployment `json:"deployments"`
	Events      []kubeEvent      `json:"events"`
	Errors      []string         `json:"errors"`
	Truncated   bool             `json:"truncated"`
}

// Only a configured HTTPS endpoint is queried. Credentials never reach the browser.
func (s *Server) handleKubernetes(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	state := kubeState{Enabled: cfg.KubeURL != "", Namespace: cfg.KubeNamespace, At: time.Now().UTC(), Pods: []kubePod{}, Deployments: []kubeDeployment{}, Events: []kubeEvent{}, Errors: []string{}}
	if !state.Enabled {
		writeJSON(w, 200, state)
		return
	}
	if !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`).MatchString(cfg.KubeNamespace) {
		state.Errors = append(state.Errors, "Неверный namespace в конфигурации.")
		writeJSON(w, 200, state)
		return
	}
	client, token, err := kubeClient(cfg.KubeURL, cfg.KubeCAFile, cfg.KubeTokenFile)
	if err != nil {
		state.Errors = append(state.Errors, "Не удалось прочитать настройки подключения Kubernetes (HTTPS, CA, токен).")
		writeJSON(w, 200, state)
		return
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	read := func(path string, v any) bool {
		if err := kubeRead(ctx, client, cfg.KubeURL+path, token, v); err != nil {
			state.Errors = append(state.Errors, err.Error())
			return false
		}
		return true
	}
	var pods struct {
		Metadata struct{ Continue string }
		Items    []struct {
			Metadata struct{ Name string }
			Spec     struct {
				NodeName   string
				Containers []json.RawMessage
			}
			Status struct {
				Phase             string
				ContainerStatuses []struct {
					Ready        bool
					RestartCount int
					State        struct {
						Waiting    struct{ Reason string }
						Terminated struct{ Reason string }
					}
				}
			}
		}
	}
	if read("/api/v1/namespaces/"+cfg.KubeNamespace+"/pods?limit=200", &pods) {
		state.Truncated = pods.Metadata.Continue != ""
		for _, p := range pods.Items {
			v := kubePod{Name: p.Metadata.Name, Node: p.Spec.NodeName, Phase: p.Status.Phase, Containers: len(p.Spec.Containers), Reasons: []string{}}
			for _, c := range p.Status.ContainerStatuses {
				if c.Ready {
					v.Ready++
				}
				v.Restarts += c.RestartCount
				if c.State.Waiting.Reason != "" {
					v.Reasons = append(v.Reasons, c.State.Waiting.Reason)
				}
				if c.State.Terminated.Reason != "" {
					v.Reasons = append(v.Reasons, c.State.Terminated.Reason)
				}
			}
			state.Pods = append(state.Pods, v)
		}
	}
	var deps struct {
		Metadata struct{ Continue string }
		Items    []struct {
			Metadata struct{ Name string }
			Spec     struct{ Replicas int }
			Status   struct {
				ReadyReplicas   int
				UpdatedReplicas int
			}
		}
	}
	if read("/apis/apps/v1/namespaces/"+cfg.KubeNamespace+"/deployments?limit=200", &deps) {
		state.Truncated = state.Truncated || deps.Metadata.Continue != ""
		for _, d := range deps.Items {
			state.Deployments = append(state.Deployments, kubeDeployment{d.Metadata.Name, d.Spec.Replicas, d.Status.ReadyReplicas, d.Status.UpdatedReplicas})
		}
	}
	var events struct {
		Metadata struct{ Continue string }
		Items    []struct {
			Type           string
			Reason         string
			Count          int
			LastTimestamp  string
			InvolvedObject struct {
				Kind string
				Name string
			}
		}
	}
	if read("/api/v1/namespaces/"+cfg.KubeNamespace+"/events?limit=200", &events) {
		state.Truncated = state.Truncated || events.Metadata.Continue != ""
		for _, e := range events.Items {
			state.Events = append(state.Events, kubeEvent{e.Type, e.Reason, e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, e.Count, e.LastTimestamp})
		}
	}
	writeJSON(w, 200, state)
}
func kubeClient(endpoint, caFile, tokenFile string) (*http.Client, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, "", fmt.Errorf("invalid endpoint")
	}
	ca, err := os.ReadFile(caFile) // #nosec G304 -- Path comes only from trusted server configuration.
	if err != nil {
		return nil, "", err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, "", fmt.Errorf("invalid CA")
	}
	token, err := os.ReadFile(tokenFile) // #nosec G304 -- Path comes only from trusted server configuration.
	if err != nil || len(strings.TrimSpace(string(token))) == 0 {
		return nil, "", fmt.Errorf("invalid token")
	}
	return &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, strings.TrimSpace(string(token)), nil
}
func kubeRead(ctx context.Context, client *http.Client, endpoint, token string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("kubernetes: неверный адрес")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kubernetes: соединение недоступно или истекло время ожидания")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return fmt.Errorf("kubernetes API: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return fmt.Errorf("kubernetes: ответ слишком большой или не прочитан")
	}
	if json.Unmarshal(data, v) != nil {
		return fmt.Errorf("kubernetes: неверный формат ответа")
	}
	return nil
}
