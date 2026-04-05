package cloudflare

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests use the internal package directly to access cfAPIBase var.

func TestVerifyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
		}

		switch {
		case strings.Contains(r.URL.Path, "/tokens/verify"):
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case strings.Contains(r.URL.Path, "/user") && !strings.Contains(r.URL.Path, "/tokens"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  map[string]string{"email": "test@example.com"},
			})
		case strings.Contains(r.URL.Path, "/accounts"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": []map[string]string{{"id": "acc123", "name": "Test Account"}},
			})
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	result, err := VerifyToken("test-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountID != "acc123" {
		t.Errorf("expected acc123, got %s", result.AccountID)
	}
	if result.AccountName != "Test Account" {
		t.Errorf("expected Test Account, got %s", result.AccountName)
	}
}

func TestVerifyToken_OAuthFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tokens/verify"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"success":false}`))
		case strings.Contains(r.URL.Path, "/user") && !strings.Contains(r.URL.Path, "/tokens"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  map[string]string{"email": "oauth@example.com"},
			})
		case strings.Contains(r.URL.Path, "/accounts"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": []map[string]string{{"id": "acc456", "name": "OAuth Account"}},
			})
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	result, err := VerifyToken("oauth-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountID != "acc456" {
		t.Errorf("expected acc456, got %s", result.AccountID)
	}
	if result.Email != "oauth@example.com" {
		t.Errorf("expected oauth@example.com, got %s", result.Email)
	}
}

func TestVerifyToken_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	_, err := VerifyToken("bad-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestFindOrCreateKV_Existing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/kv/namespaces") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": []map[string]string{
					{"id": "existing-ns", "title": "tokfresh-tokens-worker1"},
				},
			})
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	nsID, err := FindOrCreateKV("acc", "tok", "tokfresh-tokens-worker1")
	if err != nil {
		t.Fatal(err)
	}
	if nsID != "existing-ns" {
		t.Errorf("expected existing-ns, got %s", nsID)
	}
}

func TestFindOrCreateKV_Create(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/kv/namespaces"):
			// Return empty list
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/kv/namespaces"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["title"] != "tokfresh-tokens-new-worker" {
				t.Errorf("wrong title: %s", body["title"])
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]string{"id": "new-ns-id"},
			})
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	nsID, err := FindOrCreateKV("acc", "tok", "tokfresh-tokens-new-worker")
	if err != nil {
		t.Fatal(err)
	}
	if nsID != "new-ns-id" {
		t.Errorf("expected new-ns-id, got %s", nsID)
	}
}

func TestFindKV_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	_, err := FindKV("acc", "tok", "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent namespace")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestWriteKVValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/values/refresh_token") {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "rt_testvalue" {
			t.Errorf("expected raw text body 'rt_testvalue', got %q", string(body))
		}
		// Should NOT have Content-Type: application/json — it's raw text
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := WriteKVValue("acc", "tok", "ns123", "refresh_token", "rt_testvalue")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUploadWorker_Multipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart") {
			t.Error("expected multipart content type")
		}

		// Parse multipart to verify parts
		err := r.ParseMultipartForm(1 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := UploadWorker("acc", "tok", "test-worker", "export default {}", "kv123")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUploadWorker_Error10063(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors":  []map[string]interface{}{{"code": 10063, "message": "subdomain required"}},
		})
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := UploadWorker("acc", "tok", "worker", "code", "kv")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "subdomain") {
		t.Errorf("expected subdomain error, got: %v", err)
	}
}

func TestSetSchedule_ArrayBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/schedules") {
			t.Errorf("wrong path: %s", r.URL.Path)
		}

		// Verify array body
		var body []map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if len(body) != 1 {
			t.Errorf("expected array of 1, got %d", len(body))
		}
		if body[0]["cron"] != "0 21,2,7,12 * * *" {
			t.Errorf("wrong cron: %s", body[0]["cron"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := SetSchedule("acc", "tok", "worker", "0 21,2,7,12 * * *")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSetSecret_Format(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/secrets") {
			t.Errorf("wrong path: %s", r.URL.Path)
		}

		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Verify "text" field (not "value")
		if body["text"] != "secret-value" {
			t.Errorf("expected text field, got: %v", body)
		}
		// Verify "type": "secret_text"
		if body["type"] != "secret_text" {
			t.Errorf("expected type secret_text, got: %s", body["type"])
		}
		if body["name"] != "REFRESH_TOKEN" {
			t.Errorf("expected name REFRESH_TOKEN, got: %s", body["name"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := SetSecret("acc", "tok", "worker", "REFRESH_TOKEN", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWorker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/workers/scripts/test-worker") {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := DeleteWorker("acc", "tok", "test-worker")
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseCFErrorCode(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{
			"valid error",
			`{"success":false,"errors":[{"code":10063,"message":"subdomain"}]}`,
			10063,
		},
		{
			"no errors",
			`{"success":true,"errors":[]}`,
			0,
		},
		{
			"invalid JSON",
			`not json`,
			0,
		},
		{
			"empty body",
			``,
			0,
		},
		{
			"multiple errors returns first",
			`{"errors":[{"code":1001},{"code":1002}]}`,
			1001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCFErrorCode([]byte(tt.body))
			if got != tt.want {
				t.Errorf("parseCFErrorCode(%s) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}
}

func TestDeploy_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tokens/verify"):
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case strings.Contains(r.URL.Path, "/kv/namespaces") && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
		case strings.Contains(r.URL.Path, "/kv/namespaces") && r.Method == "POST":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]string{"id": "kv123"}})
		case strings.Contains(r.URL.Path, "/values/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/scripts/") && r.Method == "PUT" && !strings.Contains(r.URL.Path, "/schedules") && !strings.Contains(r.URL.Path, "/secrets"):
			// Verify multipart upload
			ct := r.Header.Get("Content-Type")
			if !strings.Contains(ct, "multipart") {
				t.Error("expected multipart upload")
			}
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/schedules"):
			// Verify array body
			var body []map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 1 || body[0]["cron"] == "" {
				t.Error("expected [{cron: ...}] array")
			}
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/secrets"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["type"] != "secret_text" {
				t.Error("expected type: secret_text")
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	steps := []string{}
	err := Deploy(DeployParams{
		Token:          "test",
		AccountID:      "acc",
		WorkerName:     "test-worker",
		WorkerCode:     "export default {}",
		RefreshToken:   "rt_xxx",
		CronExpression: "0 21 * * *",
		Timezone:       "UTC",
		OnProgress:     func(s string) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify all progress steps were called
	if len(steps) < 4 {
		t.Errorf("expected at least 4 progress steps, got %d: %v", len(steps), steps)
	}
}

func TestDeploy_Error10063(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch {
		case strings.Contains(r.URL.Path, "/kv/namespaces") && r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
		case strings.Contains(r.URL.Path, "/kv/namespaces") && r.Method == "POST":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]string{"id": "kv123"}})
		case strings.Contains(r.URL.Path, "/values/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/scripts/") && r.Method == "PUT" && !strings.Contains(r.URL.Path, "/schedules") && !strings.Contains(r.URL.Path, "/secrets"):
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"errors":  []map[string]interface{}{{"code": 10063, "message": "subdomain required"}},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := Deploy(DeployParams{
		Token:          "t",
		AccountID:      "a",
		WorkerName:     "w",
		WorkerCode:     "c",
		RefreshToken:   "r",
		CronExpression: "0 0 * * *",
		Timezone:       "UTC",
		OnProgress:     func(string) {},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "subdomain") {
		t.Errorf("expected subdomain error, got: %v", err)
	}
}

func TestEnsureAccountAccess_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/accounts/acc123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	if err := EnsureAccountAccess("tok", "acc123"); err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestEnsureAccountAccess_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := EnsureAccountAccess("bad-token", "acc123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestDeleteWorker_NotFound_IsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors":  []map[string]interface{}{{"code": 10007, "message": "not found"}},
		})
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := DeleteWorker("acc", "tok", "nonexistent-worker")
	if err != nil {
		t.Fatalf("expected nil for 404, got: %v", err)
	}
}

func TestDeleteWorker_Forbidden_ReturnsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors":  []map[string]interface{}{{"code": 10000, "message": "Authentication error"}},
		})
	}))
	defer server.Close()

	origBase := cfAPIBase
	cfAPIBase = server.URL
	defer func() { cfAPIBase = origBase }()

	err := DeleteWorker("acc", "bad-tok", "some-worker")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}
