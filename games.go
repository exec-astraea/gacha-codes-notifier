package main

// Game holds the canonical key plus the per-source slugs and display metadata.
// The Key is what appears in the GAMES env var and the state file.
type Game struct {
	Key    string // canonical internal key (GAMES env, state file)
	Name   string // human display name
	Ennead string // slug for api.ennead.cc/mihoyo/<slug>/codes
	Tori   string // slug for hoyo-codes.seria.moe/codes?game=<slug>
	Color  int    // Discord embed colour
	// Redeem is a web redemption URL template with {code} substituted,
	// or "" when the game only supports in-game redemption.
	Redeem string
}

// gameOrder is the canonical iteration order (the games map is unordered, and
// config game keys should resolve deterministically).
var gameOrder = []string{"genshin", "hsr", "zzz", "honkai3rd"}

// games is the registry of everything we know how to watch.
var games = map[string]Game{
	"genshin": {
		Key:    "genshin",
		Name:   "Genshin Impact",
		Ennead: "genshin",
		Tori:   "genshin",
		Color:  0xF2C94C,
		Redeem: "https://genshin.hoyoverse.com/en/gift?code={code}",
	},
	"hsr": {
		Key:    "hsr",
		Name:   "Honkai: Star Rail",
		Ennead: "starrail",
		Tori:   "hkrpg", // torikushiii's slug for Star Rail
		Color:  0x9B7BD8,
		Redeem: "https://hsr.hoyoverse.com/gift?code={code}",
	},
	"zzz": {
		Key:    "zzz",
		Name:   "Zenless Zone Zero",
		Ennead: "zenless",
		Tori:   "nap", // torikushiii's slug for Zenless Zone Zero
		Color:  0xFFD400,
		Redeem: "https://zenless.hoyoverse.com/redemption?code={code}",
	},
	"honkai3rd": {
		Key:    "honkai3rd",
		Name:   "Honkai Impact 3rd",
		Ennead: "honkai",
		Tori:   "honkai3rd",
		Color:  0x4A9BE0,
		Redeem: "", // Honkai Impact 3rd has no public web redemption page.
	},
}
