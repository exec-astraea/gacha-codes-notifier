package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type allowedMentions struct {
	// Parse whitelists which mention types in `content` are allowed to ping.
	Parse []string `json:"parse"`
}

type webhookPayload struct {
	Username        string           `json:"username,omitempty"`
	AvatarURL       string           `json:"avatar_url,omitempty"`
	Content         string           `json:"content,omitempty"`
	AllowedMentions *allowedMentions `json:"allowed_mentions,omitempty"`
}

// maxContentLen is Discord's per-message content limit (2000 characters). We
// chunk on code boundaries so a code block is never split across messages.
const maxContentLen = 2000

// renderCode renders one code as a plain-text block. Masked links ([text](url))
// do not render in normal message content — only in embeds — so the redeem URL
// is written raw, wrapped in <> to suppress its link-preview card.
func renderCode(g Game, c Code) string {
	value := c.Rewards
	if value == "" {
		value = "Reward details unavailable"
	}
	block := fmt.Sprintf("**%s**\n%s", c.Code, value)
	if g.Redeem != "" {
		url := strings.ReplaceAll(g.Redeem, "{code}", c.Code)
		block += fmt.Sprintf("\nRedeem: <%s>", url)
	}
	return block
}

// buildMessages renders a game's newly-found codes into one or more Discord
// message bodies (plain text, no embeds), splitting on code boundaries so no
// message exceeds maxContentLen. `header`, if non-empty, is the config message
// (with any "<@&ROLE_ID>" ping) and leads the first message so mentions fire.
func buildMessages(g Game, header string, codes []Code) []string {
	plural := "code"
	if len(codes) > 1 {
		plural = "codes"
	}
	title := fmt.Sprintf("🎁 **New %s %s**", g.Name, plural)

	var lead strings.Builder
	if header != "" {
		lead.WriteString(header)
		lead.WriteString("\n")
	}
	lead.WriteString(title)

	var msgs []string
	cur := lead.String()
	for _, c := range codes {
		block := renderCode(g, c)
		candidate := cur + "\n\n" + block
		// Start a new message when appending would overflow — unless the current
		// message is empty (an oversized lone block is sent as-is).
		if len(candidate) > maxContentLen && cur != "" {
			msgs = append(msgs, cur)
			cur = block
		} else {
			cur = candidate
		}
	}
	if cur != "" {
		msgs = append(msgs, cur)
	}
	return msgs
}

// pluralRe matches {if-singular:TEXT} / {if-plural:TEXT} blocks. TEXT runs up to
// the first "}", so it must not itself contain "}" (role mentions / emoji are fine).
var pluralRe = regexp.MustCompile(`\{if-(singular|plural):([^}]*)\}`)

// buildContent renders the config message that leads the first announcement. It
// expands {count} to the number of new codes, then resolves count-conditional
// blocks: {if-singular:X} keeps X only when count == 1, {if-plural:X} only when
// count != 1 (the other is dropped). This handles irregular plurals and
// non-English forms, e.g. "{count} {if-singular:code}{if-plural:codes}". Any
// "<@&ROLE_ID>" token pings, because this becomes Discord message content.
func buildContent(message string, count int) string {
	s := strings.ReplaceAll(message, "{count}", strconv.Itoa(count))
	plural := count != 1
	return pluralRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := pluralRe.FindStringSubmatch(m)
		kind, text := sub[1], sub[2]
		if (kind == "plural") == plural {
			return text
		}
		return ""
	})
}

// postWebhook posts plain-text messages to Discord in order. `username`/
// `avatarURL` set the sender identity on every message (avatarURL "" leaves the
// webhook's configured avatar). Only the first message may ping: the config
// header (the sole carrier of a "<@&ROLE_ID>" token) leads it, and allowed
// mentions are enabled there alone so a chunked send never re-pings.
func postWebhook(webhookURL, username, avatarURL string, messages []string) error {
	for i, msg := range messages {
		payload := webhookPayload{Username: username, AvatarURL: avatarURL, Content: msg}
		if i == 0 {
			payload.AllowedMentions = &allowedMentions{Parse: []string{"roles", "users", "everyone"}}
		}
		if err := postPayload(webhookURL, payload); err != nil {
			return err
		}
	}
	return nil
}

// postPayload POSTs one webhook message, retrying on HTTP 429 per the
// Retry-After header. A rate-limited chunk that just errored out would abandon
// the remaining chunks, and their codes are already marked sent — so retrying
// here keeps a multi-message announce from silently dropping codes.
func postPayload(webhookURL string, payload webhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	const maxRetries = 3
	for attempt := 0; ; attempt++ {
		resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			wait := retryAfter(resp)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			log.Printf("discord rate-limited (429); retrying in %s", wait)
			time.Sleep(wait)
			continue
		}

		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return fmt.Errorf("discord webhook HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}

		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil
	}
}

// retryAfter returns how long to wait before retrying a 429, from the
// Retry-After header (Discord sends it in seconds, possibly fractional). Falls
// back to 1s when absent/unparseable, and caps the wait so a bad header can't
// stall the check indefinitely.
func retryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
			d := time.Duration(secs * float64(time.Second))
			if d > 30*time.Second {
				d = 30 * time.Second
			}
			return d
		}
	}
	return time.Second
}
