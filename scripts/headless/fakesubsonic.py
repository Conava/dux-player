#!/usr/bin/env python3
"""A fake OpenSubsonic server for headless runs, on 127.0.0.1 and a free port. Accepts any credentials.

usage: fakesubsonic.py MANIFEST PORTFILE
MANIFEST is JSON: {"artists": [{"name", "albums": [{"name", "year", "genre", "tracks": [{"title", "track",
"duration", "path"}]}]}]}. The port goes to PORTFILE once the server listens. Favorites, ratings and plays live in
memory; methods it lacks answer the way a server says "not implemented". While PORTFILE's sibling file "delay" holds
a number of milliseconds, every REST call except stream and download waits that long before it is answered.
"""
import json
import mimetypes
import os
import random
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

VERSION = "1.16.1"
STARTED = int(time.time() * 1000)
CREATED = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
lock = threading.Lock()
artists, albums, songs = {}, {}, {}
playlists = {}
DELAY_FILE = ""


def load(manifest):
    with open(manifest) as f:
        data = json.load(f)
    for a, artist in enumerate(data["artists"], 1):
        ar = {"id": f"ar-{a}", "name": artist["name"], "coverArt": f"ar-{a}", "albums": []}
        artists[ar["id"]] = ar
        for b, album in enumerate(artist["albums"], 1):
            al = {"id": f"al-{a}-{b}", "name": album["name"], "artist": ar["name"], "artistId": ar["id"],
                  "year": album.get("year", 2020), "genre": album.get("genre", ""), "coverArt": f"al-{a}-{b}",
                  "created": CREATED, "songs": []}
            albums[al["id"]] = al
            ar["albums"].append(al["id"])
            for t in album["tracks"]:
                s = {"id": f"tr-{a}-{b}-{t['track']}", "parent": al["id"], "isDir": False, "type": "music",
                     "title": t["title"], "album": al["name"], "albumId": al["id"], "artist": ar["name"],
                     "artistId": ar["id"], "track": t["track"], "discNumber": 1, "year": al["year"],
                     "genre": al["genre"], "duration": t["duration"], "coverArt": al["id"],
                     "size": os.path.getsize(t["path"]), "suffix": os.path.splitext(t["path"])[1][1:],
                     "contentType": content_type(t["path"]), "created": CREATED, "file": t["path"],
                     "path": f"{ar['name']}/{al['name']}/{os.path.basename(t['path'])}"}
                songs[s["id"]] = s
                al["songs"].append(s["id"])
    first = sorted(songs)[: max(1, len(songs) // 3)]
    playlists["pl-1"] = {"id": "pl-1", "name": "Server Mix", "owner": "headless", "public": True,
                         "readonly": False, "created": CREATED, "changed": CREATED, "songs": first}


def content_type(path):
    # Python's table says audio/x-flac; servers send the registered type.
    if path.endswith(".flac"):
        return "audio/flac"
    return mimetypes.guess_type(path)[0] or "application/octet-stream"


def plain(item, hidden=("songs", "albums", "file")):
    return {k: v for k, v in item.items() if k not in hidden}


def song(sid):
    return plain(songs[sid])


def album(aid, with_songs=False):
    al = albums[aid]
    out = plain(al)
    out["songCount"] = len(al["songs"])
    out["duration"] = sum(songs[s]["duration"] for s in al["songs"])
    out["playCount"] = sum(songs[s].get("playCount", 0) for s in al["songs"])
    played = [songs[s]["played"] for s in al["songs"] if "played" in songs[s]]
    if played:
        out["played"] = max(played)
    if with_songs:
        out["song"] = [song(s) for s in al["songs"]]
    return out


def artist(aid, with_albums=False):
    ar = artists[aid]
    out = plain(ar)
    out["albumCount"] = len(ar["albums"])
    if with_albums:
        out["album"] = [album(a) for a in ar["albums"]]
    return out


def playlist(pid, with_songs=False):
    p = playlists[pid]
    out = plain(p)
    out["songCount"] = len(p["songs"])
    out["duration"] = sum(songs[s]["duration"] for s in p["songs"])
    if with_songs:
        out["entry"] = [song(s) for s in p["songs"]]
    return out


def index(entries):
    groups = {}
    for a in sorted(entries, key=lambda a: a["name"].lower()):
        groups.setdefault(a["name"][:1].upper(), []).append(a)
    return [{"name": k, "artist": v} for k, v in groups.items()]


class Fail(Exception):
    def __init__(self, code, message):
        super().__init__(message)
        self.code, self.message = code, message


def need(q, key):
    if key not in q:
        raise Fail(10, f"Required parameter is missing: {key}")
    return q[key][0]


def find(table, q, key="id"):
    item = need(q, key)
    if item not in table:
        raise Fail(70, "The requested data was not found")
    return item


def num(q, key, default):
    try:
        return int(q.get(key, [default])[0])
    except ValueError:
        raise Fail(10, f"{key} is not a number")


def album_list(q):
    kind = need(q, "type")
    ids = sorted(albums, key=lambda a: albums[a]["name"].lower())
    stats = {a: album(a) for a in ids}
    if kind == "alphabeticalByArtist":
        ids.sort(key=lambda a: (albums[a]["artist"].lower(), albums[a]["name"].lower()))
    elif kind == "starred":
        ids = [a for a in ids if "starred" in albums[a]]
    elif kind == "frequent":
        ids = sorted((a for a in ids if stats[a]["playCount"]), key=lambda a: -stats[a]["playCount"])
    elif kind == "recent":
        ids = sorted((a for a in ids if "played" in stats[a]), key=lambda a: stats[a]["played"], reverse=True)
    elif kind == "highest":
        ids = sorted((a for a in ids if albums[a].get("userRating")), key=lambda a: -albums[a]["userRating"])
    elif kind == "random":
        random.shuffle(ids)
    elif kind == "newest":
        ids.reverse()
    offset, size = num(q, "offset", 0), num(q, "size", 10)
    return {"albumList2": {"album": [stats[a] for a in ids[offset:offset + size]]}}


def search(q):
    term = q.get("query", [""])[0].strip().strip('"').lower()

    def pick(table, key, make, prefix):
        hits = [i for i in sorted(table) if term in table[i][key].lower()]
        offset, count = num(q, prefix + "Offset", 0), num(q, prefix + "Count", 20)
        return [make(i) for i in hits[offset:offset + count]]

    return {"searchResult3": {"artist": pick(artists, "name", artist, "artist"),
                              "album": pick(albums, "name", album, "album"),
                              "song": pick(songs, "title", song, "song")}}


def star(q, on):
    stamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    for key, table in (("id", None), ("albumId", albums), ("artistId", artists)):
        for item in q.get(key, []):
            target = table if table is not None else next((t for t in (songs, albums, artists) if item in t), None)
            if target is None or item not in target:
                raise Fail(70, "The requested data was not found")
            if on:
                target[item]["starred"] = stamp
            else:
                target[item].pop("starred", None)
    return {}


def set_rating(q):
    item, rating = need(q, "id"), num(q, "rating", 0)
    target = next((t for t in (songs, albums, artists) if item in t), None)
    if target is None:
        raise Fail(70, "The requested data was not found")
    if rating:
        target[item]["userRating"] = max(0, min(rating, 5))
    else:
        target[item].pop("userRating", None)
    return {}


def scrobble(q):
    for sid in q.get("id", []):
        if sid in songs and q.get("submission", ["true"])[0] == "true":
            songs[sid]["playCount"] = songs[sid].get("playCount", 0) + 1
            songs[sid]["played"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return {}


def create_playlist(q):
    ids = q.get("songId", [])
    for sid in ids:
        if sid not in songs:
            raise Fail(70, "The requested data was not found")
    if "playlistId" in q:
        pid = find(playlists, q, "playlistId")
        playlists[pid]["songs"] = ids
    else:
        pid = f"pl-{len(playlists) + 1}"
        playlists[pid] = {"id": pid, "name": need(q, "name"), "owner": "headless", "public": False,
                          "readonly": False, "created": CREATED, "changed": CREATED, "songs": ids}
    return {"playlist": playlist(pid, True)}


def update_playlist(q):
    p = playlists[find(playlists, q, "playlistId")]
    if "name" in q:
        p["name"] = q["name"][0]
    drop = {int(i) for i in q.get("songIndexToRemove", []) if i.isdigit()}
    p["songs"] = [s for i, s in enumerate(p["songs"]) if i not in drop] + [s for s in q.get("songIdToAdd", []) if s in songs]
    return {}


def starred():
    return {"starred2": {"artist": [artist(a) for a in sorted(artists) if "starred" in artists[a]],
                         "album": [album(a) for a in sorted(albums) if "starred" in albums[a]],
                         "song": [song(s) for s in sorted(songs) if "starred" in songs[s]]}}


def no_cover(q):
    raise Fail(70, "Cover art not found")


METHODS = {
    "ping": lambda q: {},
    "getLicense": lambda q: {"license": {"valid": True}},
    "getOpenSubsonicExtensions": lambda q: {"openSubsonicExtensions": [{"name": "songLyrics", "versions": [1]}]},
    "getScanStatus": lambda q: {"scanStatus": {"scanning": False, "count": len(songs)}},
    "getArtists": lambda q: {"artists": {"ignoredArticles": "", "index": index([artist(a) for a in artists])}},
    "getIndexes": lambda q: {"indexes": {"lastModified": STARTED, "ignoredArticles": "",
                                         "index": index([artist(a) for a in artists])}},
    "getArtist": lambda q: {"artist": artist(find(artists, q), True)},
    "getArtistInfo2": lambda q: {"artistInfo2": {"biography": f"{artists[find(artists, q)]['name']} exists for tests.",
                                                 "similarArtist": [artist(a) for a in sorted(artists) if a != q["id"][0]]}},
    "getAlbumList2": album_list,
    "getAlbum": lambda q: {"album": album(find(albums, q), True)},
    "getSong": lambda q: {"song": song(find(songs, q))},
    "search3": search,
    "getStarred2": lambda q: starred(),
    "star": lambda q: star(q, True),
    "unstar": lambda q: star(q, False),
    "setRating": set_rating,
    "getPlaylists": lambda q: {"playlists": {"playlist": [playlist(p) for p in sorted(playlists)]}},
    "createPlaylist": create_playlist,
    "updatePlaylist": update_playlist,
    "getPlaylist": lambda q: {"playlist": playlist(find(playlists, q), True)},
    "getRandomSongs": lambda q: {"randomSongs": {"song": [song(s) for s in random.sample(sorted(songs), min(num(q, "size", 10), len(songs)))]}},
    "scrobble": scrobble,
    "getLyricsBySongId": lambda q: find(songs, q) and {"lyricsList": {"structuredLyrics": []}},
    "getCoverArt": no_cover,
}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.handle_rest(urlsplit(self.path).query)

    def do_POST(self):
        try:
            length = max(0, min(int(self.headers.get("Content-Length") or 0), 1 << 20))
        except ValueError:
            self.send_error(400, "Content-Length is not a number")
            return
        self.handle_rest(urlsplit(self.path).query + "&" + self.rfile.read(length).decode("utf-8", "replace"))

    def handle_rest(self, query):
        path = urlsplit(self.path).path
        if not path.startswith("/rest/"):
            self.send_error(404)
            return
        method = path[len("/rest/"):].removesuffix(".view")
        q = parse_qs(query, keep_blank_values=True)
        try:
            if method == "stream" or method == "download":
                with lock:
                    file = songs[find(songs, q)]["file"]
                self.send_file(file)
                return
            self.delay()
            if method not in METHODS:
                raise Fail(0, f"{method} is not implemented")
            with lock:
                payload = METHODS[method](q)
            self.reply(dict(status="ok", **payload))
        except Fail as e:
            self.reply({"status": "failed", "error": {"code": e.code, "message": e.message}})

    def delay(self):
        try:
            with open(DELAY_FILE) as f:
                ms = int(f.read().strip() or 0)
        except (OSError, ValueError):
            return
        time.sleep(max(0, min(ms, 10000)) / 1000)

    def reply(self, body):
        body.update(version=VERSION, type="fakesubsonic", serverVersion="1.0", openSubsonic=True)
        data = json.dumps({"subsonic-response": body}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def send_file(self, file):
        size = os.path.getsize(file)
        start, end = 0, size - 1
        spec = self.headers.get("Range", "")
        ranged = spec.startswith("bytes=") and "," not in spec
        if ranged:
            first, _, last = spec[len("bytes="):].partition("-")
            try:
                if first:
                    start, end = int(first), min(int(last), size - 1) if last else size - 1
                else:
                    start = max(size - int(last), 0)
            except ValueError:
                ranged = False
                start, end = 0, size - 1
            if ranged and (start >= size or end < start):
                self.send_response(416)
                self.send_header("Content-Range", f"bytes */{size}")
                self.end_headers()
                return
        self.send_response(206 if ranged else 200)
        self.send_header("Content-Type", content_type(file))
        self.send_header("Accept-Ranges", "bytes")
        self.send_header("Content-Length", str(end - start + 1))
        if ranged:
            self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
        self.end_headers()
        with open(file, "rb") as f:
            f.seek(start)
            left = end - start + 1
            try:
                while left > 0:
                    chunk = f.read(min(left, 1 << 16))
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    left -= len(chunk)
            except (BrokenPipeError, ConnectionResetError):
                pass

    def log_message(self, fmt, *args):
        # The query carries the (fake) credentials; the method is enough.
        sys.stderr.write("%s %s\n" % (self.command, urlsplit(self.path).path))


def main():
    global DELAY_FILE
    manifest, portfile = sys.argv[1], sys.argv[2]
    DELAY_FILE = os.path.join(os.path.dirname(portfile), "delay")
    load(manifest)
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.daemon_threads = True
    with open(portfile + ".tmp", "w") as f:
        f.write(str(server.server_address[1]))
    os.replace(portfile + ".tmp", portfile)
    server.serve_forever()


if __name__ == "__main__":
    main()
