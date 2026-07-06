# The remagic app format

An app is a folder. AppLoad launches it; remagic installs, configures, and
publishes it. Everything below is one directory that works identically as a
local `remagic install ./myapp` and as a published catalog entry.

```
myapp/
├── external.manifest.json    required — AppLoad manifest + remagic fields
├── icon.png                  strongly recommended — the launcher tile
├── settings.schema.json      optional — makes the app configurable
├── settings.env              never shipped — written by settings UIs
└── …binaries, scripts, assets
```

## external.manifest.json

AppLoad reads `name`, `application`, `qtfb`, `workingDirectory`,
`environment`, `args`, `aspectRatio`. remagic additionally reads (AppLoad
ignores unknown keys):

```json
{
  "id": "myapp",
  "name": "My App",
  "version": "1.0.0",
  "description": "One line for the store.",
  "application": "run.sh",
  "qtfb": true
}
```

## Settings convention

Ship a `settings.schema.json` and every settings surface — `remagic config
<app>` (browser + phone QR), the on-device Settings app, the store's
post-install step — renders it automatically:

```json
{
  "title": "My App — settings",
  "env": "settings.env",
  "fields": [
    {"key": "MYAPP_API_KEY", "label": "API key", "kind": "secret",
     "help": "Stored only on the tablet."},
    {"key": "MYAPP_MODE", "label": "Mode", "kind": "select",
     "options": ["fast", "pretty"]}
  ],
  "presets": [
    {"name": "Defaults", "values": {"MYAPP_MODE": "fast"}}
  ]
}
```

- `kind`: `text`, `secret`, or `select`.
- Values land in the `env` file (`KEY=VALUE`, chmod 600) in the app folder.
  **Your app (or its launch script) sources that file at startup** — see
  riddle's `appload-launch.sh` for the two-line pattern.
- The env file is excluded from published zips automatically: keys never
  leave the tablet.

## Publishing

From the app folder (or a staged `dist/` copy):

```
remagic publish [folder] [-version 1.0.0] [-catalog-dir <remagic checkout>]
```

It validates the folder, zips it (minus env files and junk), creates a
GitHub release `v<version>` on the folder's repo via the `gh` CLI, and
produces the catalog entry (`id`, `version`, `url`, `sha256`). With
`-catalog-dir` the entry is written into `catalog.json` in place; otherwise
it's printed for a PR to this repo.

The catalog PR is reviewed by a human on purpose: apps run as root on
people's tablets. Checksums are pinned in the catalog, and `remagic install`
refuses a mismatch.
