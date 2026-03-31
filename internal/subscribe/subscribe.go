package subscribe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

const subscribeEndpoint = "https://tokfresh.com/api/subscribe"

// Subscribe sends an email subscription request to the TokFresh web endpoint.
// This allows CLI users to receive notifications about breaking changes
// that require running `tokfresh upgrade`.
func Subscribe(email string) error {
	body, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Debug("subscribing to breaking change notifications", "endpoint", subscribeEndpoint)
	resp, err := httpClient.Post(subscribeEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("subscription request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success           bool   `json:"success"`
		AlreadySubscribed bool   `json:"alreadySubscribed"`
		Error             string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("subscription failed: %s", result.Error)
	}

	if result.AlreadySubscribed {
		log.Debug("email already subscribed")
	}

	return nil
}
