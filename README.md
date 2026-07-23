# hoyo-codes

Watches for new **gacha redemption codes** (Genshin Impact, Honkai: Star Rail,
Zenless Zone Zero, Honkai Impact 3rd, Wuthering Waves, and Arknights: Endfield)
and posts them to **Discord webhooks** on a schedule. Codes come from community
HoYoverse APIs plus an optional self-hosted [OpenGachaCodes](config.example.yaml)
instance (`opengachaBaseUrl`) — the sole source for non-HoYo games like Wuthering
Waves and Arknights: Endfield.

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
