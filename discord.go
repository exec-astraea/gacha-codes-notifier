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

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type embed struct {
	Title  string       `json:"title"`
	Color  int          `json:"color"`
	Fields []embedField `json:"fields,omitempty"`
}

type allowedMentions struct {
	// Parse whitelists which mention types in `content` are allowed to ping.
	Parse []string `json:"parse"`
}

type webhookPayload struct {
	Username        string           `json:"username,omitempty"`
	AvatarURL       string           `json:"avatar_url,omitempty"`
	Content         string           `json:"content,omitempty"`
	Embeds          []embed          `json:"embeds"`
	AllowedMentions *allowedMentions `json:"allowed_mentions,omitempty"`
}

// Discord limits: 25 fields per embed, 10 embeds per message.
const (
	maxFieldsPerEmbed = 25
	maxEmbedsPerMsg   = 10
)

// buildEmbed renders one game's newly-found codes into a Discord embed.
func buildEmbed(g Game, codes []Code) embed {
	fields := make([]embedField, 0, len(codes))
	for _, c := range codes {
		value := c.Rewards
		if value == "" {
			value = "Reward details unavailable"
		}
		if g.Redeem != "" {
			url := strings.ReplaceAll(g.Redeem, "{code}", c.Code)
			value += fmt.Sprintf("\n[Redeem →](%s)", url)
		}
		fields = append(fields, embedField{Name: c.Code, Value: value})
	}
	plural := "code"
	if len(codes) > 1 {
		plural = "codes"
	}
	return embed{
		Title:  fmt.Sprintf("🎁 New %s %s", g.Name, plural),
		Color:  g.Color,
		Fields: fields,
	}
}

// pluralRe matches {if-singular:TEXT} / {if-plural:TEXT} blocks. TEXT runs up to
// the first "}", so it must not itself contain "}" (role mentions / emoji are fine).
var pluralRe = regexp.MustCompile(`\{if-(singular|plural):([^}]*)\}`)

// buildContent renders the message body posted above the embed. It expands
// {count} to the number of new codes, then resolves count-conditional blocks:
// {if-singular:X} keeps X only when count == 1, {if-plural:X} only when count != 1
// (the other is dropped). This handles irregular plurals and non-English forms,
// e.g. "{count} {if-singular:code}{if-plural:codes}". Any "<@&ROLE_ID>" token
// pings, because this is the Discord message content (never an embed).
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

// postWebhook sends embeds to Discord, chunking to respect the 10-embeds and
// 25-fields-per-embed limits. `username`/`avatarURL` set the sender identity on
// every message (avatarURL "" leaves the webhook's configured avatar). `content`,
// if non-empty, is set on the first message so any mentions in it ping (mentions
// never ping from embeds).
func postWebhook(webhookURL, username, avatarURL, content string, embeds []embed) error {
	// Split any embed with too many fields into multiple embeds.
	var normalized []embed
	for _, e := range embeds {
		if len(e.Fields) <= maxFieldsPerEmbed {
			normalized = append(normalized, e)
			continue
		}
		for i := 0; i < len(e.Fields); i += maxFieldsPerEmbed {
			end := min(i+maxFieldsPerEmbed, len(e.Fields))
			chunk := e
			chunk.Fields = e.Fields[i:end]
			normalized = append(normalized, chunk)
		}
	}

	for i := 0; i < len(normalized); i += maxEmbedsPerMsg {
		end := min(i+maxEmbedsPerMsg, len(normalized))
		payload := webhookPayload{Username: username, AvatarURL: avatarURL, Embeds: normalized[i:end]}
		// Only set content on the first message so a chunked send doesn't re-ping.
		if i == 0 && content != "" {
			payload.Content = content
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
