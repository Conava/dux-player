pragma Singleton

import QtQuick
import Quickshell
import qs.Common
import qs.Services

// Shared wording: joins, counts with their singular form, and the library and device lines several views show.
// A count has a "1 …" term and a "%1 …" term, like the shell's I18n.duration.
Singleton {
    function repeatState(mode) {
        switch (mode) {
        case "all":
            return I18n.tr("Repeat all", "transport button state");
        case "one":
            return I18n.tr("Repeat one", "transport button state");
        }
        return I18n.tr("Repeat off", "transport button state");
    }

    function join(first, second) {
        return I18n.tr("%1 · %2", "two facts joined by a dot").arg(first).arg(second);
    }

    // joins the non-empty parts
    function joinAll(parts) {
        const kept = parts.filter(p => p !== "" && p !== undefined && p !== null);
        return kept.length > 0 ? kept.reduce((all, part) => join(all, part)) : "";
    }

    // an accessible name: a title and its detail
    function named(title, detail) {
        return detail ? I18n.tr("%1, %2", "accessible name: a title and its detail").arg(title).arg(detail) : title;
    }

    function tracks(n) {
        return n === 1 ? I18n.tr("1 track", "track count, singular") : I18n.tr("%1 tracks", "track count, plural").arg(n);
    }

    function albums(n) {
        return n === 1 ? I18n.tr("1 album", "album count, singular") : I18n.tr("%1 albums", "album count, plural").arg(n);
    }

    function artists(n) {
        return n === 1 ? I18n.tr("1 artist", "artist count, singular") : I18n.tr("%1 artists", "artist count, plural").arg(n);
    }

    function playlists(n) {
        return n === 1 ? I18n.tr("1 playlist", "playlist count, singular") : I18n.tr("%1 playlists", "playlist count, plural").arg(n);
    }

    function genres(n) {
        return n === 1 ? I18n.tr("1 genre", "genre count, singular") : I18n.tr("%1 genres", "genre count, plural").arg(n);
    }

    // the count for a list of one type
    function count(type, n) {
        switch (type) {
        case "albums":
            return albums(n);
        case "artists":
            return artists(n);
        case "playlists":
            return playlists(n);
        case "genres":
            return genres(n);
        }
        return tracks(n);
    }

    function plays(n) {
        return n === 1 ? I18n.tr("1 play", "play count, singular") : I18n.tr("%1 plays", "play count, plural").arg(n);
    }

    function moreTracks(n) {
        return n === 1 ? I18n.tr("1 more track", "list overflow, singular") : I18n.tr("%1 more tracks", "list overflow, plural").arg(n);
    }

    function selected(n) {
        return n === 1 ? I18n.tr("1 selected", "selection count, singular") : I18n.tr("%1 selected", "selection count, plural").arg(n);
    }

    function libraryTrackCount(libraryId) {
        return PlayerService.albums.filter(a => a.library === libraryId).reduce((sum, a) => sum + a.trackCount, 0);
    }

    function libraryCounts(libraryId) {
        return join(albums(PlayerService.albums.filter(a => a.library === libraryId).length), tracks(libraryTrackCount(libraryId)));
    }

    // "" unless the library is being read
    function libraryBusy(library) {
        if (!library.scanning)
            return "";
        const progress = CatalogService.scanProgress[library.id];
        if (progress?.total > 0)
            return I18n.tr("Scanning… %1 of %2", "library row status while scanning").arg(progress.done).arg(progress.total);
        return PlayerService.isRemote(library.id) ? I18n.tr("Syncing…", "library status while a server library syncs") : I18n.tr("Scanning…", "library status while folders are read");
    }

    function isLocalOutput(device) {
        return device.kind !== "default" && device.kind !== "cast";
    }

    // the second line of a playback device row
    function deviceNote(device) {
        if (!device.available)
            return I18n.tr("Not connected", "playback device state");
        if (device.kind === "default")
            return I18n.tr("Follows the system setting", "playback device note");
        if (device.kind === "cast" && PlayerService.casting && PlayerService.selectedOutputs.indexOf(device.id) >= 0)
            return I18n.tr("Playing now", "playback device note for the device being cast to");
        if (device.kind === "cast")
            return I18n.tr("Chromecast", "playback device note");
        return "";
    }

    // A reason the daemon sends as a code plus English; the English is the fallback for a code this version
    // doesn't know. detail: the folder of folderMissing.
    function reason(code, english, detail) {
        switch (code) {
        case "serverUnreachable":
            return I18n.tr("Server unreachable", "why a server library or its tracks can't be used");
        case "signInExpired":
            return I18n.tr("Sign-in expired. Sign in again.", "why a server library can't be used");
        case "quickConnectOff":
            return I18n.tr("Quick Connect is turned off on this server", "why Quick Connect can't sign in");
        case "keyringLocked":
            return I18n.tr("The system keyring is locked", "why a server library can't sign in");
        case "noKeyring":
            return I18n.tr("No system keyring is running", "why a server library can't sign in");
        case "openFailed":
            return I18n.tr("Couldn't open the library", "library error");
        case "folderMissing":
            if (detail)
                return I18n.tr("Folder not found: %1. It was removed or its drive isn't mounted.", "library error, %1 is the folder").arg(detail);
            break;
        case "foldersMissing":
            return I18n.tr("Folders not found. They were removed or their drive isn't mounted.", "library error");
        case "trackFolderMissing":
            return I18n.tr("Folder not found. It was removed or its drive isn't mounted.", "why a track can't play");
        case "needsMpv":
            return I18n.tr("Requires mpv to play this format", "why a track can't play");
        case "libraryOff":
            return I18n.tr("Library turned off or removed", "why a track can't play");
        case "outputFailed":
            return I18n.tr("Couldn't switch outputs", "playback device error");
        case "eqNoPipeWire":
            return I18n.tr("Requires PipeWire (pipewire and pw-cli)", "why the equalizer can't start");
        case "eqNoServer":
            return I18n.tr("PipeWire isn't running", "why the equalizer can't start");
        case "eqFailed":
            return I18n.tr("PipeWire filter failed to start", "why the equalizer can't start");
        case "notRegularFile":
            return I18n.tr("Not a regular file", "why ratings weren't written to a file");
        case "otherOwner":
            return I18n.tr("Owned by another user", "why ratings weren't written to a file");
        case "hardLinks":
            return I18n.tr("Has other hard links", "why ratings weren't written to a file");
        case "readOnlyFile":
            return I18n.tr("Read-only file", "why ratings weren't written to a file");
        case "readOnlyFolder":
            return I18n.tr("Read-only folder", "why ratings weren't written to a file");
        case "writeFailed":
            return I18n.tr("Couldn't write the file", "why ratings weren't written to a file");
        case "listFailed":
            return I18n.tr("Couldn't read the library's marked tracks", "why writing ratings to files couldn't start");
        case "scanFailed":
            return I18n.tr("Couldn't scan the library", "library error");
        case "noFFmpeg":
            return I18n.tr("ffmpeg is not installed. Tracks without ReplayGain tags play without loudness correction.", "library notice");
        case "watchFailed":
            return I18n.tr("Changes on disk show only after a rescan", "library notice");
        case "tooManyFolders":
            return I18n.tr("Too many folders to watch (fs.inotify.max_user_watches); some changes show only after a rescan", "library notice");
        case "plainHttp":
            return I18n.tr("The connection isn't encrypted. Sign-in tokens and playback can be read on the network. Use an https address.", "library notice");
        case "passwordSignIn":
            return I18n.tr("Signs in with the password because this server doesn't accept token sign-in.", "library notice");
        case "signInRefused":
            return I18n.tr("Incorrect user name or password", "why a server connection test failed");
        case "passwordNeedsHttps":
            return I18n.tr("Signing in with the password requires an https server address", "why a server connection test failed");
        case "certificateInvalid":
            return I18n.tr("The server's certificate isn't trusted", "why a server connection test failed");
        case "badAddress":
            return I18n.tr("That isn't an http(s) server address", "why a server connection test failed");
        case "credentialsInAddress":
            return I18n.tr("Enter the user name and password in their own fields, not in the server address", "why a server connection test failed");
        case "redirected":
            return I18n.tr("The server redirected to another address. Use that address instead.", "why a server connection test failed");
        case "testFailed":
            return I18n.tr("Couldn't connect to this server", "why a server connection test failed");
        case "daemonDisconnected":
            return I18n.tr("Lost the connection to the player", "why a request to the player daemon failed");
        case "daemonTimeout":
            return I18n.tr("The player didn't answer in time", "why a request to the player daemon failed");
        case "daemonOffline":
            return I18n.tr("Not connected to the player", "why a request to the player daemon failed");
        case "busy":
            return I18n.tr("Ratings are already being written", "why writing ratings to files couldn't start");
        }
        return english ?? "";
    }

    // a library's error in the UI's language ("" without one)
    function libraryError(library) {
        return library ? reason(library.errorCode, library.error, library.folders?.[0]) : "";
    }
}
