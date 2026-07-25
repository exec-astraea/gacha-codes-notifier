package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultUsername is the Discord sender name used when a game sets no username.
const defaultUsername = "Hoyo Codes"

// GameNotify is the notification config for one game: where to post, what to
// say, and how the sender appears. Webhook and Message are required; Username
// and AvatarURL are optional per-game overrides. To ping a role, include its
// "<@&ROLE_ID>" token in Message — it leads the Discord message body, so
// mentions in it fire.
type GameNotify struct {
	Webhook   string // Discord webhook URL for this game
	Message   string // text leading the announcement; supports {count}
	Username  string // Discord sender name (defaults to defaultUsername)
	AvatarURL string // Discord sender avatar image URL ("" = webhook's own avatar)
}

// Config is the resolved runtime configuration.
type Config struct {
	Schedule               string                // cron expression (5 fields)
	Timezone               string                // IANA tz for the schedule; "" = container TZ
	Games                  []string              // canonical game keys to watch (ordered)
	Notify                 map[string]GameNotify // per-game key -> resolved notify config
	DataDir                string                // dir holding the sent-codes state file
	StateFile              string                // full path to sent.json
	RunOnStart             bool                  // run one check immediately at startup
	MarkExistingOnFirstRun bool                  // mark active backlog sent on a game's first run
	HTTPTimeout            int                   // per-request HTTP timeout, seconds
	MaxPostAttempts        int                   // give up announcing a code after this many failed checks
	OpengachaBaseURL       string                // self-hosted OpenGachaCodes base URL ("" = source disabled)
}

// --- on-disk YAML shape ---

type gameConf struct {
	Webhook   string `yaml:"webhook"`
	Message   string `yaml:"message"`
	Username  string `yaml:"username"`  // optional Discord sender name
	AvatarURL string `yaml:"avatarUrl"` // optional Discord sender avatar image URL
}

type fileConf struct {
	Schedule               string              `yaml:"schedule"`
	Timezone               string              `yaml:"timezone"`
	DataDir                string              `yaml:"dataDir"`
	RunOnStart             *bool               `yaml:"runOnStart"`
	MarkExistingOnFirstRun *bool               `yaml:"markExistingOnFirstRun"`
	HTTPTimeout            int                 `yaml:"httpTimeout"`
	MaxPostAttempts        int                 `yaml:"maxPostAttempts"`
	OpengachaBaseURL       string              `yaml:"opengachaBaseUrl"`
	Games                  map[string]gameConf `yaml:"games"`
}

func orString(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func orBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func orInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// loadConfig reads and resolves the YAML config at path.
func loadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	defer f.Close()

	var fc fileConf
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // reject typos / unknown keys
	if err := dec.Decode(&fc); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg := Config{
		Schedule:               orString(fc.Schedule, "*/30 * * * *"),
		Timezone:               strings.TrimSpace(fc.Timezone),
		DataDir:                orString(fc.DataDir, "/app/data"),
		RunOnStart:             orBool(fc.RunOnStart, true),
		MarkExistingOnFirstRun: orBool(fc.MarkExistingOnFirstRun, true),
		HTTPTimeout:            orInt(fc.HTTPTimeout, 20),
		MaxPostAttempts:        orInt(fc.MaxPostAttempts, 10),
		OpengachaBaseURL:       strings.TrimRight(strings.TrimSpace(fc.OpengachaBaseURL), "/"),
		Notify:                 map[string]GameNotify{},
	}
	cfg.StateFile = filepath.Join(cfg.DataDir, "sent.json")

	// Warn about game keys we don't recognise so typos don't silently no-op.
	for key := range fc.Games {
		if _, ok := games[key]; !ok {
			log.Printf("warning: config game %q is not recognised (known: %s), ignoring",
				key, strings.Join(gameOrder, ", "))
		}
	}

	// Resolve watched games in a stable, canonical order. Every listed game must
	// define both a webhook and a message. To stop watching a game, remove it
	// (or comment it out) rather than toggling a flag.
	var problems []string
	for _, key := range gameOrder {
		gc, ok := fc.Games[key]
		if !ok {
			continue
		}
		if strings.TrimSpace(gc.Webhook) == "" {
			problems = append(problems, fmt.Sprintf("games.%s: webhook is required", key))
		}
		if strings.TrimSpace(gc.Message) == "" {
			problems = append(problems, fmt.Sprintf("games.%s: message is required", key))
		}
		// A game with no reachable source (only OpenGachaCodes serves it, but no
		// opengachaBaseUrl is set) can never produce codes — fail startup so the
		// misconfiguration is loud rather than a silent no-op every check.
		g := games[key]
		opengachaUsable := g.Opengacha != "" && cfg.OpengachaBaseURL != ""
		if g.Ennead == "" && g.Seria == "" && !opengachaUsable {
			problems = append(problems, fmt.Sprintf("games.%s: only OpenGachaCodes serves this game — set opengachaBaseUrl to watch it", key))
		}
		cfg.Games = append(cfg.Games, key)
		cfg.Notify[key] = GameNotify{
			Webhook:   gc.Webhook,
			Message:   gc.Message,
			Username:  orString(gc.Username, defaultUsername),
			AvatarURL: strings.TrimSpace(gc.AvatarURL),
		}
	}

	if len(cfg.Games) == 0 {
		problems = append(problems, "no games are configured (define at least one under `games:`)")
	}
	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid config %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}

	return cfg, nil
}
