package main

// Game holds the canonical key plus the per-source slugs and display metadata.
// The Key is what appears in the GAMES env var and the state file.
type Game struct {
	Key       string // canonical internal key (GAMES env, state file)
	Name      string // human display name
	Ennead    string // slug for api.ennead.cc/mihoyo/<slug>/codes
	Seria     string // slug for hoyo-codes.seria.moe/codes?game=<slug>
	Opengacha string // slug for <opengachaBaseUrl>/games/<slug>/codes ("" = not served there)
	// Redeem is a web redemption URL template with {code} substituted,
	// or "" when the game only supports in-game redemption.
	Redeem string
}

// gameOrder is the canonical iteration order (the games map is unordered, and
// config game keys should resolve deterministically).
var gameOrder = []string{"genshin", "hsr", "zzz", "honkai3rd", "wuwa", "endfield"}

// games is the registry of everything we know how to watch.
var games = map[string]Game{
	"genshin": {
		Key:       "genshin",
		Name:      "Genshin Impact",
		Ennead:    "genshin",
		Seria:     "genshin",
		Opengacha: "genshin",
		Redeem:    "https://genshin.hoyoverse.com/en/gift?code={code}",
	},
	"hsr": {
		Key:       "hsr",
		Name:      "Honkai: Star Rail",
		Ennead:    "starrail",
		Seria:     "hkrpg",    // seriaati's slug for Star Rail
		Opengacha: "starrail", // OpenGachaCodes slug for Star Rail
		Redeem:    "https://hsr.hoyoverse.com/gift?code={code}",
	},
	"zzz": {
		Key:       "zzz",
		Name:      "Zenless Zone Zero",
		Ennead:    "zenless",
		Seria:     "nap",     // seriaati's slug for Zenless Zone Zero
		Opengacha: "zenless", // OpenGachaCodes slug for Zenless Zone Zero
		Redeem:    "https://zenless.hoyoverse.com/redemption?code={code}",
	},
	"honkai3rd": {
		Key:       "honkai3rd",
		Name:      "Honkai Impact 3rd",
		Ennead:    "honkai",
		Seria:     "honkai3rd",
		Opengacha: "", // OpenGachaCodes doesn't serve Honkai Impact 3rd.
		Redeem:    "", // Honkai Impact 3rd has no public web redemption page.
	},
	"wuwa": {
		Key:  "wuwa",
		Name: "Wuthering Waves",
		// Not a HoYoverse game — the HoYo-only sources (ennead, seria) don't
		// serve it, so its codes come solely from OpenGachaCodes. A source with
		// an empty slug is skipped in fetchCodes.
		Ennead:    "",
		Seria:     "",
		Opengacha: "wuwa",
		Redeem:    "", // Wuthering Waves redeems codes in-game (no public web page).
	},
	"endfield": {
		Key:  "endfield",
		Name: "Arknights: Endfield",
		// Not a HoYoverse game — OpenGachaCodes-only, like wuwa.
		Ennead:    "",
		Seria:     "",
		Opengacha: "endfield",
		Redeem:    "", // Arknights: Endfield redeems codes in-game (no public web page).
	},
}
