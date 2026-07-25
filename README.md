# hoyo-codes

Watches for new **gacha redemption codes** (Genshin Impact, Honkai: Star Rail,
Zenless Zone Zero, Honkai Impact 3rd, Wuthering Waves, and Arknights: Endfield)
and posts them to **Discord webhooks** on a schedule. Codes come from community
HoYoverse APIs plus an optional self-hosted
[OpenGachaCodes](https://github.com/torikushiii/OpenGachaCodes) instance
(`opengachaBaseUrl`) — the sole source for non-HoYo games like Wuthering Waves
and Arknights: Endfield. See [Sources](#sources).

## Run

```sh
cp config.example.yaml config.yaml    # then add your webhook(s) + messages

docker build -t hoyo-codes .
docker run -d --name hoyo-codes \
  -e TZ=Europe/Amsterdam \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -v "$PWD/data:/app/data" \
  hoyo-codes
```

Or without Docker: `CONFIG_FILE=config.yaml go run .`

Configuration lives in `config.yaml` — see
[`config.example.yaml`](config.example.yaml) for the fully-commented template.

Architecture, config reference, and development notes: **[AGENTS.md](AGENTS.md)**.

## Sources

Codes are merged from every source that serves a given game and de-duplicated by
code (case-insensitive). The first source to report a code wins; a later one only
fills in rewards the earlier one left blank. One source failing is tolerated — a
game is skipped only when *all* of its sources fail.

| Source | Serves | Notes |
| ------ | ------ | ----- |
| [hoyoverse-api](https://github.com/torikushiii/hoyoverse-api) by [@torikushiii](https://github.com/torikushiii) | HoYoverse titles | Public API at `api.ennead.cc`. |
| [hoyo-codes](https://github.com/seriaati/hoyo-codes) by [@seriaati](https://github.com/seriaati) | HoYoverse titles | Public API at `hoyo-codes.seria.moe`; codes are validated against HoYoLAB before being served. |
| [OpenGachaCodes](https://github.com/torikushiii/OpenGachaCodes) by [@torikushiii](https://github.com/torikushiii) | All titles, incl. non-HoYo | **Self-hosted**, opt-in via `opengachaBaseUrl`. The only source for Wuthering Waves and Arknights: Endfield. |

## Credits

This project is a thin notifier — the actual work of finding and validating codes
belongs to others:

- [**hoyoverse-api**](https://github.com/torikushiii/hoyoverse-api) (AGPL-3.0) and
  [**OpenGachaCodes**](https://github.com/torikushiii/OpenGachaCodes) by
  [@torikushiii](https://github.com/torikushiii).
- [**hoyo-codes**](https://github.com/seriaati/hoyo-codes) (GPL-3.0) by
  [@seriaati](https://github.com/seriaati), built for
  [Hoyo Buddy](https://github.com/seriaati/hoyo-buddy).

## License

[AGPL-3.0](LICENSE) © Alisa Frelia @exec-astraea.
