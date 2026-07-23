package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Code is a normalized redemption code from either upstream source.
type Code struct {
	Code    string
	Rewards string // human-readable, e.g. "Primogem ×60, Adventurer's Experience ×5"
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

const userAgent = "hoyo-codes-notifier/1.0 (+https://github.com/exec-astraea/hoyo-codes)"

func getJSON(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetchEnnead pulls active codes from api.ennead.cc.
func fetchEnnead(g Game) ([]Code, error) {
	var resp struct {
		Active []struct {
			Code    string   `json:"code"`
			Rewards []string `json:"rewards"`
		} `json:"active"`
	}
	url := fmt.Sprintf("https://api.ennead.cc/mihoyo/%s/codes", g.Ennead)
	if err := getJSON(url, &resp); err != nil {
		return nil, err
	}
	out := make([]Code, 0, len(resp.Active))
	for _, c := range resp.Active {
		if c.Code == "" {
			continue
		}
		out = append(out, Code{Code: c.Code, Rewards: strings.Join(c.Rewards, ", ")})
	}
	return out, nil
}

// fetchTori pulls active codes from hoyo-codes.seria.moe (torikushiii).
func fetchTori(g Game) ([]Code, error) {
	var resp struct {
		Codes []struct {
			Code    string `json:"code"`
			Rewards string `json:"rewards"` // "Primogem*60;Adventurer's Experience*5"
		} `json:"codes"`
	}
	url := fmt.Sprintf("https://hoyo-codes.seria.moe/codes?game=%s", g.Tori)
	if err := getJSON(url, &resp); err != nil {
		return nil, err
	}
	out := make([]Code, 0, len(resp.Codes))
	for _, c := range resp.Codes {
		if c.Code == "" {
			continue
		}
		out = append(out, Code{Code: c.Code, Rewards: normalizeToriRewards(c.Rewards)})
	}
	return out, nil
}

// normalizeToriRewards turns "A*60;B*5" into "A ×60, B ×5".
func normalizeToriRewards(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ";")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if idx := strings.LastIndex(p, "*"); idx >= 0 {
			p = p[:idx] + " ×" + p[idx+1:]
		}
		parts[i] = p
	}
	return strings.Join(parts, ", ")
}

// fetchOpengacha pulls active codes from a self-hosted OpenGachaCodes instance
// at base (e.g. http://localhost:8413). The API returns a flat array of
// {code, rewards[]}; reward quantities already use ASCII "x" (e.g. "Astrite x50").
func fetchOpengacha(g Game, base string) ([]Code, error) {
	url := fmt.Sprintf("%s/games/%s/codes", strings.TrimRight(base, "/"), g.Opengacha)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// A 404 means the slug isn't served (or a strict-path miss) — treat as
	// "nothing to contribute" rather than a source failure, so it doesn't count
	// against the all-sources-failed check. Any other non-200 is a real error.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s: HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var codes []struct {
		Code    string   `json:"code"`
		Rewards []string `json:"rewards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&codes); err != nil {
		return nil, err
	}
	out := make([]Code, 0, len(codes))
	for _, c := range codes {
		if c.Code == "" {
			continue
		}
		out = append(out, Code{Code: c.Code, Rewards: strings.Join(c.Rewards, ", ")})
	}
	return out, nil
}

// fetchCodes queries every source that serves the game and merges them,
// de-duplicating by the upper-cased code. It is tolerant: a source is skipped
// when the game has no slug for it (or, for OpenGachaCodes, when no base URL is
// configured), one source failing still uses the rest, and it only errors when
// every attempted source fails. opengachaBase is the OpenGachaCodes instance URL
// ("" disables that source entirely).
func fetchCodes(g Game, opengachaBase string) ([]Code, error) {
	merged := map[string]Code{}    // key: upper(code)
	order := make([]string, 0, 32) // preserve first-seen order
	var errs []string
	attempted := 0

	add := func(codes []Code) {
		for _, c := range codes {
			key := strings.ToUpper(strings.TrimSpace(c.Code))
			if key == "" {
				continue
			}
			existing, ok := merged[key]
			if !ok {
				merged[key] = c
				order = append(order, key)
				continue
			}
			// Fill in rewards if the earlier source didn't have any.
			if existing.Rewards == "" && c.Rewards != "" {
				existing.Rewards = c.Rewards
				merged[key] = existing
			}
		}
	}

	// try runs one source, recording an attempt and either merging its codes or
	// capturing its error. Order matters: earlier sources win reward fill-in.
	try := func(name string, fetch func() ([]Code, error)) {
		attempted++
		if codes, err := fetch(); err != nil {
			errs = append(errs, name+": "+err.Error())
		} else {
			add(codes)
		}
	}

	if g.Ennead != "" {
		try("ennead", func() ([]Code, error) { return fetchEnnead(g) })
	}
	if g.Tori != "" {
		try("tori", func() ([]Code, error) { return fetchTori(g) })
	}
	if opengachaBase != "" && g.Opengacha != "" {
		try("opengacha", func() ([]Code, error) { return fetchOpengacha(g, opengachaBase) })
	}

	if attempted == 0 {
		// No source serves this game with the current config (e.g. an
		// OpenGachaCodes-only game watched without opengachaBaseUrl set).
		return nil, fmt.Errorf("no source available for %q", g.Key)
	}
	if len(errs) == attempted {
		return nil, fmt.Errorf("all sources failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("[%s] %d of %d sources failed (using the rest): %s",
			g.Key, len(errs), attempted, strings.Join(errs, "; "))
	}

	out := make([]Code, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	return out, nil
}
