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
    // M3 fade through: the incoming page grows from this scale
    readonly property real pageEnterScale: 0.92
    // the full view's morph, as shares of its time: the pages are gone by the first, the view starts fading in at
    // the second, after them so it never shows over the pages
    readonly property real morphPagesFade: 0.3
    readonly property real morphViewFade: 0.5
    // how far the surface has grown before the view shows at all
    readonly property real morphViewCover: 0.6
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
    // list rows (tracks, albums, the queue) form one connected group on this surface
    readonly property color listRowColor: SettingsMetrics.rowColor
    readonly property real headerSearchWidth: 360
    // Appearance → Density: each step from Default adds or takes this much row height
    readonly property real densityStep: Theme.densityLevel * Theme.spacingS
    readonly property real listRowHeight: Theme.listItemHeight + densityStep
    readonly property real listRowTwoLineHeight: Theme.listItemTwoLineHeight + densityStep
    readonly property real barArtSize: Theme.buttonHeightM
    readonly property real barHeight: Theme.buttonHeightM + mediaSeekbarHeight + Theme.spacingS * 3
    readonly property real barInfoWidth: 280
    // the bar's info block and seekbar shrink to these before buttons move into the overflow menu
    readonly property real barInfoMinWidth: 160
    // the most of the bar, beside the right zone, the left zone may take: the rest is the seekbar's
    readonly property real barLeftShare: 0.4
    readonly property real barLyricMinWidth: 80
    readonly property var barInfoWidths: ({
            "s": 200,
            "m": barInfoWidth,
            "l": 360,
            "xl": 440
        })
    readonly property real barSeekMinWidth: 120
    // the shell's media chrome metrics (Bento and M3E players)
    readonly property real mediaCardMargin: Theme.spacingL
    readonly property real mediaInnerRadius: Math.max(Theme.cornerRadiusXS, Theme.cornerRadiusXL - mediaCardMargin - Theme.layerOutlineWidth)
    readonly property real mediaSeekbarHeight: 22
    // the Bento seekbar: a thicker wave with a round handle that turns into a pill while dragged
    readonly property real mediaSeekbarExpressiveHeight: 30
    readonly property real mediaSeekbarExpressiveLineWidth: 4
    readonly property real mediaSeekbarExpressiveAmplitude: 1.8
    readonly property real mediaSeekbarExpressiveWavelength: 22
    readonly property real mediaSeekbarHandleSize: 16
    readonly property real mediaSeekbarHandleDragWidth: 5
    readonly property real mediaSeekbarHandleDragHeight: 28
    readonly property real mediaSeekbarHandleGap: 7
    // Bento's art column takes this share of the width (the cover's size option), the deck the rest
    readonly property real mediaArtColumnRatioS: 0.4
    readonly property real mediaArtColumnRatioM: 0.5
    readonly property real mediaArtColumnRatioL: 0.6
    // the deck's cap grows with the window past this much width (680 is 0.4 of 1700)
    readonly property real mediaDeckMaxWidth: 680
    readonly property real mediaDeckMaxRatio: 0.4
    // narrower than this, the deck goes below the art
    readonly property real mediaDeckMinWidth: 320
    // the stacked layout needs at least this much art; with less, art and deck stay side by side
    readonly property real mediaArtMinSize: 160
    readonly property real mediaTransportMaxHeight: 70
    readonly property real mediaArtLyricsScale: 0.88
    readonly property int mediaArtRotationDuration: 30000
    // Material's shared axis for the Play on sheet: 30 dp of travel, the outgoing side gone by 35 % of it
    readonly property real mediaSheetSlideDistance: 30
    readonly property real mediaSheetFadeThreshold: 0.35
    readonly property real mediaSheetSettleProgress: 0.002
    // Google Sans Flex, condensed and rounded (the shell's expressive media title)
    readonly property var mediaTitleFontAxes: ({
            "wght": 700,
            "wdth": 25,
            "ROND": 100,
            "GRAD": 100
        })
    readonly property real mediaPlayWidthRatio: 2
    readonly property real mediaTransportPressScale: 1.2
    readonly property real mediaDeviceNameWidthRatio: 0.42
    readonly property real mediaTextScrollSpeed: 40
    readonly property int mediaTextScrollStep: 30
    readonly property real accentTrackAlpha: 0.28
    readonly property real accentSubtleAlpha: 0.55
    readonly property real accentRailMix: 0.3
    readonly property real volumePanelWidth: 280
    readonly property real sidePanelWidth: 340
    readonly property real queueWidth: 380
    // M3 modal side sheet: the page strip left uncovered when the panel overlays a compact window
    readonly property real compactPanelPeek: 56
    // a page's own search field in its header
    readonly property real scopedSearchWidth: 240
    readonly property real artShapeRatio: 0.46
    readonly property real expandedArtMaxSmall: 200
    readonly property real expandedArtMax: 480
    readonly property real columnNumberWidth: Theme.iconButtonSize
    readonly property real columnTimeWidth: Theme.buttonMinWidth + Theme.spacingXL
    readonly property real columnLibraryWidth: 140
    // a track row narrower than this drops favorite, rating, library, cloud and duration from its end: they would leave the title under 170
    readonly property real fullRowWidth: 480
    readonly property real columnGenreWidth: 120
    readonly property real columnYearWidth: Theme.buttonMinWidth
    readonly property real columnTitleMinWidth: Theme.buttonMinWidth
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
    // the Material card's art grows with the window from these floors, in the same proportion as the floors (200, 480, 640)
    readonly property real expandedArtRatioS: 0.25
    readonly property real expandedArtRatioM: 0.6
    readonly property real expandedArtRatioL: 0.8
    readonly property real lyricsLeadHeightDivisor: 12
    readonly property real lyricsLeadWidthDivisor: 11
    readonly property real lyricsLineInactiveScale: 0.8
    readonly property real lyricsLineInactiveAlpha: 0.4
    readonly property real lyricsRestAlpha: 0.35
    // seconds of cue time the fill is averaged over, so a word sung in a few frames still glides
    readonly property real lyricsSweepWindow: 0.24
    // width of the soft edge ahead of the fill, in row heights
    readonly property real lyricsSweepLead: 0.9
    readonly property int lyricsLineLinger: 500
    // a line grows and lights up over this share of the seconds since the line before it, within the bounds
    readonly property real lyricsMoveShare: 0.3
    readonly property real lyricsMoveMin: 0.4
    readonly property real lyricsMoveMax: 1
    // share of its move a line is on its way before it starts, so it is sung in place
    readonly property real lyricsLineLeadIn: 0.4
    // a clock that steps back less than this (s) right after the centre advanced keeps the new row
    readonly property real lyricsFollowBackstepSlack: 1.5
    // the list scrolls on a critically damped spring that settles in about ten time constants
    readonly property int lyricsScrollDuration: 1000
    readonly property real lyricsScrollRate: 10000 / lyricsScrollDuration
    readonly property real lyricsScrollStiffness: lyricsScrollRate * lyricsScrollRate
    readonly property real lyricsScrollDamping: 2 * lyricsScrollRate
    // the spring stops this close to the line and lands on it
    readonly property real lyricsScrollSettleDistance: 0.5
    readonly property real lyricsScrollSettleSpeed: 10
    // a line's scale change: a gentle start and a long settle
    readonly property var lyricsTrackCurve: [0.25, 0.3, 0.13, 1, 1, 1]
    // rows fade out over this share of the view's height towards its edges, up to the most below
    readonly property real lyricsEdgeFadeRatio: 0.2
    readonly property real lyricsEdgeFadeMax: Theme.spacingXL * 2
    readonly property real lyricsBreakGap: 5
    readonly property real lyricsBreakIndicatorScale: 1.2
    // below this view width the follow button shows only its icon
    readonly property real lyricsFollowLabelMinWidth: 240
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
    // a typed filter rule value applies after this pause; each apply rewrites settings and re-runs the query
    readonly property int ruleEditDelay: 300
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
