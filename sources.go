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

func getJSON(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "hoyo-codes-notifier/1.0 (+https://github.com/exec-astraea/hoyo-codes)")
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

// fetchCodes queries both sources and merges them, de-duplicating by the
// upper-cased code. It is tolerant: if one source fails the other is still
// used; it only errors when both fail.
func fetchCodes(g Game) ([]Code, error) {
	merged := map[string]Code{}    // key: upper(code)
	order := make([]string, 0, 32) // preserve first-seen order
	var errs []string

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

	if codes, err := fetchEnnead(g); err != nil {
		errs = append(errs, "ennead: "+err.Error())
	} else {
		add(codes)
	}
	if codes, err := fetchTori(g); err != nil {
		errs = append(errs, "tori: "+err.Error())
	} else {
		add(codes)
	}

	if len(errs) == 2 {
		return nil, fmt.Errorf("all sources failed: %s", strings.Join(errs, "; "))
	}
	if len(errs) == 1 {
		log.Printf("[%s] one source failed (using the other): %s", g.Key, errs[0])
	}

	out := make([]Code, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	return out, nil
}
