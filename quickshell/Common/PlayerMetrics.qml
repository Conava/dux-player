pragma Singleton

import QtQuick
import Quickshell
import qs.Common

Singleton {
    readonly property real windowWidth: 1180
    readonly property real windowHeight: 820
    readonly property real windowMinWidth: 640
    readonly property real windowMinHeight: 480
    readonly property real screenMargin: 100
    readonly property real compactBreakpoint: 760
    readonly property real paneRadius: Theme.cornerRadiusL
    readonly property real pagePadding: Theme.spacingXL
    readonly property real pageHeaderHeight: Theme.fontSizeXXLarge + Theme.spacingXL + Theme.spacingM
    readonly property real gridCellWidth: 184
    readonly property real gridCaptionHeight: Theme.fontSizeMedium + Theme.fontSizeSmall + Theme.spacingM * 2
    readonly property real shelfArtSize: 152
    readonly property int shelfLimit: 12
    readonly property real detailArtSize: 232
    // an artist's biography before "Show more"
    readonly property int biographyLines: 4
    readonly property real rowArtSize: Theme.avatarSize + Theme.spacingXS
    readonly property real barArtSize: Theme.buttonHeightM
    readonly property real barHeight: Theme.buttonHeightS + Theme.sliderHandleHeight + Theme.spacingS * 2
    readonly property real barInfoWidth: 280
    // the bar's info block and seekbar shrink to these before buttons move into the overflow menu
    readonly property real barInfoMinWidth: 160
    readonly property real barSeekMinWidth: 120
    // the shell's media chrome metrics (Bento and M3E players)
    readonly property real mediaCardMargin: Theme.spacingL
    readonly property real mediaInnerRadius: Math.max(Theme.cornerRadiusXS, Theme.cornerRadiusXL - mediaCardMargin - Theme.layerOutlineWidth)
    readonly property real mediaSeekbarHeight: 22
    readonly property real mediaPlayWidthRatio: 1.5
    readonly property real mediaPlayWidthStep: 0.75
    readonly property real mediaTransportPressScale: 1.2
    readonly property real mediaDeviceNameWidthRatio: 0.42
    readonly property real mediaTextScrollSpeed: 40
    readonly property int mediaTextScrollStep: 30
    readonly property real accentTrackAlpha: 0.28
    readonly property real accentSubtleAlpha: 0.55
    readonly property real volumePanelWidth: 280
    readonly property real sidePanelWidth: 340
    readonly property real queueWidth: 380
    // a page's own search field in its header
    readonly property real scopedSearchWidth: 240
    readonly property real artShapeRatio: 0.46
    readonly property real expandedArtMaxSmall: 200
    readonly property real expandedArtMax: 480
    readonly property real columnNumberWidth: Theme.iconButtonSize
    readonly property real columnTimeWidth: Theme.buttonMinWidth
    readonly property real columnLibraryWidth: 140
    readonly property real columnGenreWidth: 120
    readonly property real columnYearWidth: Theme.buttonMinWidth
    readonly property real columnRatingWidth: 96
    // the title column's share of the free width, against the other columns' fill weights
    readonly property real titleColumnWeight: 3
    readonly property real eqBandLabelWidth: Theme.buttonMinWidth
    readonly property var playbackSpeeds: [0.5, 0.75, 1, 1.25, 1.5, 2]
    readonly property real tileSizeS: 128
    readonly property real tileSizeL: 232
    readonly property real lyricsScaleS: 0.8
    readonly property real lyricsScaleL: 1.25
    readonly property real expandedArtMaxLarge: 640
    readonly property real lyricsNearOpacity: 0.55
    readonly property real lyricsFarOpacity: 0.3
    readonly property real lyricsLineHeight: 1.25
    readonly property real lyricsLeadHeightDivisor: 12
    readonly property real lyricsLeadWidthDivisor: 11
    readonly property int positionTick: 250
    // a small position correction fades out over about this long (ms) instead of jumping
    readonly property int positionEase: 600
    // the daemon reports a new track's position once it has played a second; until then the position runs
    // at most this far (s), so a track that is still loading doesn't jump back
    readonly property real positionLead: 1.25
    readonly property int outputDelayMax: 2000
    // covers are asked for at this edge length, so servers send a small image
    readonly property int coverFetchSize: 600
    readonly property int coverFlushDelay: 50
    readonly property int searchLimit: 50
    readonly property int searchDelay: 250
    readonly property int searchMinLength: 2
    // the most track refs the daemon takes in one play request (its list bound)
    readonly property int queueRefsMax: 5000
    // the folder watcher reports changes every few seconds while files move
    readonly property int indexSettleDelay: 3000
    readonly property int trackPage: 500
    readonly property int cacheListLimit: 500
    // the daemon's bounds for a Played / Most played offline rule (10 MB to 100000 tracks), and the defaults
    readonly property real offlineRuleMinGB: 0.01
    readonly property int offlineRuleMaxTracks: 100000
    readonly property int offlineRuleDefaultTracks: 100
    readonly property real offlinePlayedDefaultBytes: 1e9
    // the search picker of albums, artists and playlists lists this many matches
    readonly property int offlinePickerLimit: 200
    readonly property int historyPage: 100
    readonly property int shelfFetchLimit: 50
    // the Most played page and shelves, merged over every library
    readonly property int mostPlayedLimit: 200
    readonly property int statsRefreshDelay: 1000
    // a position event this far from the running position means someone seeked
    readonly property real seekDetectThreshold: 1.5
    // seconds a position report may move the clock before the lyrics schedule their next line again
    readonly property real lyricsResyncDrift: 0.15
    readonly property int snackbarDuration: 4000
}
