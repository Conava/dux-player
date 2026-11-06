# Dux Player

A music player for Linux in the style of [Dux Shell](https://github.com/duxlabs-dev/dux-shell). 
It is intended for use within the Dux Shell and picks up its settings and theming, but can be run and configured standalone.
Dux Player plays local libraries and streams from Navidrome, Subsonic and Jellyfin servers.

> **Status: 0.0.1-alpha.** Early Development Version

![Home with the player bar](docs/screenshots/home.png)

| Full view, Bento | Full view, Material |
|---|---|
| ![Full view in the Bento style](docs/screenshots/now-playing.png) | ![Full view in the Material style](docs/screenshots/now-playing-material.png) |

<details>
<summary>More screenshots</summary>

| Synced lyrics | Queue beside the player |
|---|---|
| ![Synced lyrics in the Bento full view](docs/screenshots/lyrics.png) | ![Queue beside the Material player](docs/screenshots/material-queue.png) |
| **Side panel** | **Light mode** |
| ![Home with the side panel](docs/screenshots/side-panel.png) | ![Home in light mode](docs/screenshots/light.png) |
| **Albums** | **Album** |
| ![Albums page](docs/screenshots/albums.png) | ![An album page](docs/screenshots/album.png) |
| **Playlists** | **Add to playlists** |
| ![Playlists page](docs/screenshots/playlists.png) | ![Dialog listing which playlists hold the track](docs/screenshots/playlist-dialog.png) |
| **Settings** | **Settings search** |
| ![Now playing settings](docs/screenshots/settings.png) | ![Settings search results](docs/screenshots/settings-search.png) |

</details>

## Features
- Local libraries (several folders, indexed and watched) and any number of server libraries
- Local playlists that mix tracks from any library, plus server native playlists; add a track to several at once and see which ones hold it
- Offline rules per server library, caching and preloading the next songs for a seamless experience; favorites and ratings set while a server is unreachable are sent once it is back
- Gapless playback, crossfade, volume normalization, skipping leading and trailing silence, equalizer several outputs at once
- Synced lyrics with word-by-word fill, scrobbling (through the server, or to ListenBrainz and Last.fm), casting through [dux-cast](https://github.com/Conava/dux-cast)
- Keeps playing with the window closed; MPRIS, a tray icon and a CLI (`dplayer play`, `dplayer next`, …) for keybinds
- Fully customizable now-playing bar, side panel and full view in Bento or Material design as well as customizable home page, lists and menus, with searchable settings

## Requirements

- [Quickshell](https://quickshell.org) with Qt 6
- Recommended: mpv 0.38 or newer. Without it a built-in player plays FLAC, MP3, Ogg Vorbis, Opus, WAV and AIFF through PulseAudio or PipeWire (pipewire-pulse), but not AAC/M4A, ALAC, WavPack, APE or Musepack files; it has no crossfade, speed control, equalizer or several outputs at once, and can't seek in a track the server converts while it plays
- Go 1.27 to build
- Optional: ffmpeg (loudness and silence measurement), PipeWire with WirePlumber (multiple outputs, equalizer, system volume), a Secret Service keyring (server passwords), `cast-helper` (casting)

## Build and run

```sh
git clone --recursive https://github.com/Conava/dux-player
cd dux-player
make run      # build and run against the QML in this checkout, with hot reload
```

To install it as a regular application for your user:

```sh
make build
make install PREFIX=~/.local
```

Install system-wide:

```sh
make build
make install
```

`make help` to list every target.

## Troubleshooting

The daemon logs to `~/.local/state/dplayer/daemon.log` and the window to `ui.log` next to it, including mpv's warnings and the mpv version found at startup. To watch it live, run `dplayer quit`, then `dplayer daemon` in a terminal, and open the window.

## Online services

Dux Player only contacts the servers you add, except for these features, each of which can be turned off in Settings:

- **Artist information:** artist names are sent to MusicBrainz, Wikidata and Wikipedia, and to fanart.tv and Last.fm once you add your own API key. Biographies from Wikipedia are available under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).
- **Lyrics:** track title, artist, album and duration are sent to the enabled lyrics providers.
- **Scrobbling:** played tracks are sent to ListenBrainz or Last.fm when you connect an account. Last.fm needs your own [API account](https://www.last.fm/api/account/create).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE). 
Tag reading and writing uses [TagLib](https://taglib.org) (LGPL-2.1) through [go-taglib](https://github.com/sentriz/go-taglib). 
The built-in player uses [flac](https://github.com/mewkiz/flac) (public domain), [go-mp3](https://github.com/hajimehoshi/go-mp3) (Apache-2.0), [oggvorbis](https://github.com/jfreymuth/oggvorbis) (MIT), [gopus](https://github.com/thesyncim/gopus) (BSD-3-Clause) and [pulse](https://github.com/jfreymuth/pulse) (MIT). 
`make install` puts the license text of every dependency in `share/licenses/dux-player/THIRD_PARTY_LICENSES`.
