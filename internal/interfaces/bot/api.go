package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// longPollTimeout is the getUpdates server-side wait; the HTTP client
// timeout must exceed it.
const longPollTimeout = 50

// apiClient is a minimal Telegram Bot API client over plain HTTP.
// The token is embedded in the URL and must never appear in logs or errors.
type apiClient struct {
	base string
	http *http.Client
}

func newAPIClient(token string) *apiClient {
	return &apiClient{
		base: "https://api.telegram.org/bot" + token,
		http: &http.Client{Timeout: (longPollTimeout + 10) * time.Second},
	}
}

type user struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// displayName returns the best human-readable label for the user.
func (u *user) displayName() string {
	if u.Username != "" {
		return u.Username
	}
	return u.FirstName
}

type chat struct {
	ID int64 `json:"id"`
}

type message struct {
	From *user  `json:"from"`
	Chat chat   `json:"chat"`
	Text string `json:"text"`
}

type update struct {
	UpdateID int      `json:"update_id"`
	Message  *message `json:"message"`
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// call performs one Bot API method call and unmarshals result into out (if non-nil).
func (c *apiClient) call(ctx context.Context, method string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/"+method, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s request: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if !apiResp.OK {
		return fmt.Errorf("%s: telegram api error: %s", method, apiResp.Description)
	}
	if out != nil {
		if err := json.Unmarshal(apiResp.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

func (c *apiClient) getMe(ctx context.Context) (*user, error) {
	var me user
	if err := c.call(ctx, "getMe", url.Values{}, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

func (c *apiClient) getUpdates(ctx context.Context, offset int) ([]update, error) {
	params := url.Values{
		"timeout":         {strconv.Itoa(longPollTimeout)},
		"allowed_updates": {`["message"]`},
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	var updates []update
	if err := c.call(ctx, "getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *apiClient) sendMessage(ctx context.Context, chatID int64, text string) error {
	return c.call(ctx, "sendMessage", url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"text":    {text},
	}, nil)
}

func (c *apiClient) sendChatAction(ctx context.Context, chatID int64, action string) error {
	return c.call(ctx, "sendChatAction", url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"action":  {action},
	}, nil)
}
