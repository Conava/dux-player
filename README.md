# Dux Player

A music player for Linux in the style of [Dux Shell](https://github.com/duxlabs-dev/dux-shell). It plays local
libraries and streams from Navidrome, Subsonic and Jellyfin servers.

> **Status: 0.0.1-alpha.** An early development version for collaborators. Not released, not packaged, and
> settings formats may still change.

## Features

- Local libraries (several folders, indexed and watched) and any number of server libraries
- Local playlists that mix tracks from any library, plus server playlists
- Offline rules per server library (favorites, playlists, albums, artists, recently and most played)
- Gapless playback, crossfade, volume normalization, skipping leading and trailing silence, equalizer
- Synced lyrics, scrobbling (through the server, or to ListenBrainz and Last.fm), casting through
  [dux-cast](https://github.com/Conava/dux-cast)
- Keeps playing with the window closed; MPRIS, a tray icon and a CLI (`dplayer play`, `dplayer next`, …) for
  keybinds
- Customizable now-playing bar, side panel and full view, home page, lists and menus

## Requirements

- [Quickshell](https://quickshell.org) with Qt 6, and mpv 0.38 or newer
- Go 1.27 to build
- Optional: ffmpeg (loudness and silence measurement), PipeWire with WirePlumber (multiple outputs, equalizer,
  system volume), a Secret Service keyring (server passwords), `cast-helper` (casting)

## Build and run

```sh
git clone --recursive https://github.com/Conava/dux-player
cd dux-player
make run      # build and run against the QML in this checkout
make help     # every other target: lint, test, build, packaging
```

## Online services

Dux Player only contacts the servers you add, except for these features, each of which can be turned off in
Settings:

- **Artist information:** artist names are sent to MusicBrainz, Wikidata and Wikipedia, and to fanart.tv and
  Last.fm once you add your own API key. Biographies from Wikipedia are available under
  [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).
- **Lyrics:** track title, artist, album and duration are sent to the enabled lyrics providers.
- **Scrobbling:** played tracks are sent to ListenBrainz or Last.fm when you connect an account.

## Contributing

The project is built largely with AI agents. [AGENTS.md](AGENTS.md) holds the rules for code, design and
commits, for people and agents alike. Run `make lint test build` before sending a change.

## License

MIT, see [LICENSE](LICENSE). Tag reading and writing uses [TagLib](https://taglib.org) (LGPL-2.1) through
[go-taglib](https://github.com/sentriz/go-taglib); the audio fixtures in `core/internal/tagwrite/testdata` come
from TagLib's test data.
