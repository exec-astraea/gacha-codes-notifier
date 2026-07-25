package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "/app/config.yaml"
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}

	httpClient.Timeout = time.Duration(cfg.HTTPTimeout) * time.Second

	// The schedule runs in this location; cron.WithLocation makes it explicit.
	loc := time.Local
	if cfg.Timezone != "" {
		l, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			log.Fatalf("invalid timezone %q: %v", cfg.Timezone, err)
		}
		loc = l
	}

	// loadConfig has already validated the games (webhook + message present).
	var watched []Game
	for _, key := range cfg.Games {
		watched = append(watched, games[key])
	}

	state, err := loadState(cfg.StateFile)
	if err != nil {
		log.Printf("warning: could not read state (%v); starting fresh", err)
	}

	log.Printf("hoyo-codes starting: watching %d game(s), schedule %q, TZ %s",
		len(watched), cfg.Schedule, loc)

	check := func() { runCheck(cfg, watched, state) }

	if cfg.RunOnStart {
		check()
	}

	c := cron.New(cron.WithLocation(loc))
	if _, err := c.AddFunc(cfg.Schedule, check); err != nil {
		log.Fatalf("invalid schedule %q: %v", cfg.Schedule, err)
	}
	c.Start()

	// Block until interrupted.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
	c.Stop()
}

// runCheck fetches codes for every watched game and announces the new ones,
// each to its own webhook with its own message.
func runCheck(cfg Config, watched []Game, state *State) {
	log.Println("checking for new codes…")
	dirty := false

	for _, g := range watched {
		codes, err := fetchCodes(g, cfg.OpengachaBaseURL)
		if err != nil {
			log.Printf("[%s] fetch failed: %v", g.Key, err)
			continue
		}

		firstRun := !state.hasGame(g.Key)

		// Collect codes we haven't sent yet, but don't mark them until we've
		// actually acted on them (announced, or deliberately suppressed).
		var fresh []Code
		for _, c := range codes {
			if state.sent(g.Key, upper(c.Code)) {
				continue
			}
			fresh = append(fresh, c)
		}

		markFresh := func() {
			for _, c := range fresh {
				state.mark(g.Key, upper(c.Code))
			}
			dirty = true
		}

		switch {
		case len(fresh) == 0:
			if firstRun {
				// Seed the game even with no codes, so a later fetch that *does*
				// find codes is announced instead of mistaken for first-run backlog.
				state.seed(g.Key)
				dirty = true
			}
			log.Printf("[%s] no new codes (%d active)", g.Key, len(codes))
		case firstRun && cfg.MarkExistingOnFirstRun:
			// Backlog on first sight: record as sent without announcing.
			markFresh()
			log.Printf("[%s] first run: marked %d existing code(s) as sent", g.Key, len(fresh))
		default:
			nc := cfg.Notify[g.Key]
			header := buildContent(nc.Message, len(fresh))
			messages := buildMessages(g, header, fresh)
			if err := postWebhook(nc.Webhook, nc.Username, nc.AvatarURL, messages); err != nil {
				// Nothing reached Discord, so leave codes unmarked and retry them
				// next check — but cap attempts so a persistently failing post
				// (bad webhook, a bug) can't retry and log forever. A code that
				// hits the cap is marked done (given up on) instead of announced.
				var gaveUp []Code
				for _, c := range fresh {
					if state.recordAttempt(g.Key, upper(c.Code)) >= cfg.MaxPostAttempts {
						state.mark(g.Key, upper(c.Code))
						gaveUp = append(gaveUp, c)
					}
				}
				dirty = true
				if len(gaveUp) > 0 {
					log.Printf("[%s] discord post failed: %v; gave up on %d code(s) after %d attempts: %s",
						g.Key, err, len(gaveUp), cfg.MaxPostAttempts, codeList(gaveUp))
				} else {
					log.Printf("[%s] discord post failed (will retry next check): %v", g.Key, err)
				}
			} else {
				markFresh()
				log.Printf("[%s] announced %d new code(s): %s", g.Key, len(fresh), codeList(fresh))
			}
		}
	}

	if dirty {
		if err := state.save(); err != nil {
			log.Printf("warning: failed to save state: %v", err)
		}
	}
}
