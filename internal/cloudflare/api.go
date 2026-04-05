package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/charmbracelet/log"
)

var ErrUnauthorized = errors.New("unauthorized")

// cfAPIBase is the Cloudflare API v4 base URL. It is a var so tests can override it.
var cfAPIBase = "https://api.cloudflare.com/client/v4"

var httpClient = &http.Client{Timeout: 30 * time.Second}

func EnsureAccountAccess(token, accountID string) error {
	url := fmt.Sprintf("%s/accounts/%s", cfAPIBase, accountID)
	resp, err := doRequest("GET", url, token, nil)
	if err != nil {
		return fmt.Errorf("account access check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.ReadAll(resp.Body)
		return ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("account access check failed (%d): %s", resp.StatusCode, string(body))
	}
	_, _ = io.ReadAll(resp.Body)
	return nil
}

func ConsoleURL(accountID, workerName string) string {
	return fmt.Sprintf("https://dash.cloudflare.com/%s/workers/services/view/%s", accountID, workerName)
}

type VerifyResult struct {
	AccountID   string
	AccountName string
	Email       string
}

// VerifyToken verifies the CF token and returns account info.
// Supports both dashboard API tokens (/user/tokens/verify) and
// wrangler OAuth tokens (/user) — OAuth tokens don't work with /tokens/verify.
func VerifyToken(token string) (*VerifyResult, error) {
	var email string

	verifyURL := fmt.Sprintf("%s/user/tokens/verify", cfAPIBase)
	log.Debug("verifying token", "url", verifyURL)

	start := time.Now()
	resp, err := doRequest("GET", verifyURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("token verify request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("token verify response", "status", resp.StatusCode, "elapsed", time.Since(start))

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Debug("API token verify failed, trying OAuth /user fallback")
		userURL := fmt.Sprintf("%s/user", cfAPIBase)
		start = time.Now()
		userResp, userErr := doRequest("GET", userURL, token, nil)
		if userErr != nil {
			return nil, fmt.Errorf("token verification failed: %w", userErr)
		}
		defer func() { _ = userResp.Body.Close() }()
		log.Debug("user endpoint response", "status", userResp.StatusCode, "elapsed", time.Since(start))

		userBody, _ := io.ReadAll(userResp.Body)
		if userResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("token verification failed (%d): %s", userResp.StatusCode, string(userBody))
		}

		var userResult struct {
			Success bool `json:"success"`
			Result  struct {
				Email string `json:"email"`
			} `json:"result"`
		}
		if err := json.Unmarshal(userBody, &userResult); err != nil || !userResult.Success {
			return nil, fmt.Errorf("token verification failed: %s", string(userBody))
		}
		email = userResult.Result.Email
	} else {
		var verifyResp struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(body, &verifyResp); err != nil || !verifyResp.Success {
			return nil, fmt.Errorf("token verification failed: %s", string(body))
		}

		userURL := fmt.Sprintf("%s/user", cfAPIBase)
		userResp, userErr := doRequest("GET", userURL, token, nil)
		if userErr == nil {
			defer func() { _ = userResp.Body.Close() }()
			userBody, _ := io.ReadAll(userResp.Body)
			var u struct {
				Result struct {
					Email string `json:"email"`
				} `json:"result"`
			}
			_ = json.Unmarshal(userBody, &u)
			email = u.Result.Email
		}
	}

	accountsURL := fmt.Sprintf("%s/accounts?per_page=1", cfAPIBase)
	log.Debug("fetching accounts", "url", accountsURL)

	start = time.Now()
	resp2, err := doRequest("GET", accountsURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("accounts request failed: %w", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	log.Debug("accounts response", "status", resp2.StatusCode, "elapsed", time.Since(start))

	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("accounts request failed (%d): %s", resp2.StatusCode, string(body2))
	}

	var accountsResp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body2, &accountsResp); err != nil {
		return nil, fmt.Errorf("failed to parse accounts response: %w", err)
	}
	if len(accountsResp.Result) == 0 {
		return nil, fmt.Errorf("no accounts found for this token")
	}

	return &VerifyResult{
		AccountID:   accountsResp.Result[0].ID,
		AccountName: accountsResp.Result[0].Name,
		Email:       email,
	}, nil
}

// FindOrCreateKV finds a KV namespace by title, creating it if not found.
// Returns the namespace ID.
func FindOrCreateKV(accountID, token, title string) (namespaceID string, err error) {
	nsID, err := FindKV(accountID, token, title)
	if err == nil {
		return nsID, nil
	}

	// Not found — create it
	createURL := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces", cfAPIBase, accountID)
	createBody, _ := json.Marshal(map[string]string{"title": title})

	log.Debug("creating KV namespace", "url", createURL, "title", title)
	start := time.Now()
	resp, err := doRequest("POST", createURL, token, createBody)
	if err != nil {
		return "", fmt.Errorf("KV namespace creation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("KV create response", "status", resp.StatusCode, "elapsed", time.Since(start))

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("KV namespace creation failed (%d): %s", resp.StatusCode, string(body))
	}

	var createResp struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &createResp); err != nil {
		return "", fmt.Errorf("failed to parse KV create response: %w", err)
	}
	if createResp.Result.ID == "" {
		return "", fmt.Errorf("KV namespace created but ID is empty")
	}

	return createResp.Result.ID, nil
}

// FindKV finds a KV namespace by title. Returns error if not found.
func FindKV(accountID, token, title string) (namespaceID string, err error) {
	listURL := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces?per_page=100", cfAPIBase, accountID)
	log.Debug("listing KV namespaces", "url", listURL)

	start := time.Now()
	resp, err := doRequest("GET", listURL, token, nil)
	if err != nil {
		return "", fmt.Errorf("KV list request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("KV list response", "status", resp.StatusCode, "elapsed", time.Since(start))

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("KV list failed (%d): %s", resp.StatusCode, string(body))
	}

	var listResp struct {
		Result []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return "", fmt.Errorf("failed to parse KV list response: %w", err)
	}

	for _, ns := range listResp.Result {
		if ns.Title == title {
			log.Debug("found existing KV namespace", "id", ns.ID, "title", ns.Title)
			return ns.ID, nil
		}
	}

	return "", fmt.Errorf("KV namespace %q not found", title)
}

// WriteKVValue writes a plain text value to a KV key.
// Note: body is raw string (NOT JSON-encoded), per CF API spec.
func WriteKVValue(accountID, token, nsID, key, value string) error {
	url := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s", cfAPIBase, accountID, nsID, key)
	log.Debug("writing KV value", "url", url, "key", key)

	start := time.Now()
	req, err := http.NewRequest("PUT", url, bytes.NewReader([]byte(value)))
	if err != nil {
		return fmt.Errorf("failed to create KV write request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// Note: no Content-Type header — CF expects raw text body for KV values

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("KV write request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("KV write response", "status", resp.StatusCode, "elapsed", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("KV write failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// UploadWorker uploads worker code as a multipart form with metadata.
// This is the most complex CF API call — uses multipart/form-data with:
//   - Part 1: "worker.js" with Content-Type: application/javascript+module
//   - Part 2: "metadata" with Content-Type: application/json (bindings, main_module, etc.)
func UploadWorker(accountID, token, workerName, code, kvNSID string) error {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s", cfAPIBase, accountID, workerName)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Part 1: worker.js
	jsHeader := textproto.MIMEHeader{}
	jsHeader.Set("Content-Disposition", `form-data; name="worker.js"; filename="worker.js"`)
	jsHeader.Set("Content-Type", "application/javascript+module")
	jsPart, err := w.CreatePart(jsHeader)
	if err != nil {
		return fmt.Errorf("failed to create worker.js part: %w", err)
	}
	if _, err := jsPart.Write([]byte(code)); err != nil {
		return fmt.Errorf("failed to write worker.js: %w", err)
	}

	// Part 2: metadata
	metadata := map[string]interface{}{
		"main_module":        "worker.js",
		"compatibility_date": "2024-01-01",
		"bindings": []map[string]string{{
			"type":         "kv_namespace",
			"name":         "TOKEN_STORE",
			"namespace_id": kvNSID,
		}},
	}
	metaJSON, _ := json.Marshal(metadata)
	metaHeader := textproto.MIMEHeader{}
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"; filename="metadata"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := w.CreatePart(metaHeader)
	if err != nil {
		return fmt.Errorf("failed to create metadata part: %w", err)
	}
	if _, err := metaPart.Write(metaJSON); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())

	log.Debug("uploading worker", "url", url, "size", buf.Len())
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("worker upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("worker upload response", "status", resp.StatusCode, "elapsed", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errCode := parseCFErrorCode(body)
		if errCode == 10063 {
			return fmt.Errorf("workers.dev subdomain not configured. Visit https://dash.cloudflare.com/?to=/:account/workers-and-pages to set it up, then retry")
		}
		return fmt.Errorf("worker upload failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// SetSchedule sets cron triggers for a worker.
// Body is an ARRAY of cron objects: [{"cron": "..."}]
func SetSchedule(accountID, token, workerName, cron string) error {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s/schedules", cfAPIBase, accountID, workerName)
	body, _ := json.Marshal([]map[string]string{{"cron": cron}})

	log.Debug("setting schedule", "url", url, "cron", cron)
	start := time.Now()
	resp, err := doRequest("PUT", url, token, body)
	if err != nil {
		return fmt.Errorf("schedule request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("schedule response", "status", resp.StatusCode, "elapsed", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("schedule setup failed (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SetSecret sets a worker secret.
// Body uses "text" field (NOT "value") and "type": "secret_text".
func SetSecret(accountID, token, workerName, name, value string) error {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s/secrets", cfAPIBase, accountID, workerName)
	body, _ := json.Marshal(map[string]string{
		"name": name,
		"text": value,
		"type": "secret_text",
	})

	log.Debug("setting secret", "url", url, "name", name)
	start := time.Now()
	resp, err := doRequest("PUT", url, token, body)
	if err != nil {
		return fmt.Errorf("secret request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("secret response", "status", resp.StatusCode, "elapsed", time.Since(start))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("secret %q failed (%d): %s", name, resp.StatusCode, string(respBody))
	}
	return nil
}

// DeleteWorker deletes a worker script.
func DeleteWorker(accountID, token, workerName string) error {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s", cfAPIBase, accountID, workerName)

	log.Debug("deleting worker", "url", url)
	start := time.Now()
	resp, err := doRequest("DELETE", url, token, nil)
	if err != nil {
		return fmt.Errorf("worker delete request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	log.Debug("worker delete response", "status", resp.StatusCode, "elapsed", time.Since(start))

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return nil
	case http.StatusForbidden, http.StatusUnauthorized:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrUnauthorized, string(body))
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("worker delete failed (%d): %s", resp.StatusCode, string(body))
	}
}

// parseCFErrorCode extracts the first error code from a CF API error response.
// CF error responses: {"success":false,"errors":[{"code":10063,"message":"..."}]}
func parseCFErrorCode(body []byte) int {
	var resp struct {
		Errors []struct {
			Code int `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &resp) == nil && len(resp.Errors) > 0 {
		return resp.Errors[0].Code
	}
	return 0
}

// doRequest is a helper to make authenticated CF API requests with JSON body.
func doRequest(method, url, token string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return httpClient.Do(req)
}

// CreateTail starts a tail session for real-time worker logs.
// POST /accounts/{accountId}/workers/scripts/{workerName}/tails
// Returns the tail session ID and WebSocket URL.
func CreateTail(accountID, token, workerName string) (tailID, wsURL string, err error) {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s/tails", cfAPIBase, accountID, workerName)

	log.Debug("creating tail session", "url", url)

	resp, err := doRequest("POST", url, token, nil)
	if err != nil {
		return "", "", fmt.Errorf("create tail request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("create tail failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse tail response: %w", err)
	}

	return result.Result.ID, result.Result.URL, nil
}

// DeleteTail removes a tail session.
// DELETE /accounts/{accountId}/workers/scripts/{workerName}/tails/{tailId}
func DeleteTail(accountID, token, workerName, tailID string) error {
	url := fmt.Sprintf("%s/accounts/%s/workers/scripts/%s/tails/%s", cfAPIBase, accountID, workerName, tailID)

	log.Debug("deleting tail session", "url", url)

	resp, err := doRequest("DELETE", url, token, nil)
	if err != nil {
		return fmt.Errorf("delete tail request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete tail failed (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}
