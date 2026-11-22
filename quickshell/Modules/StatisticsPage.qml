pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// This device's plays over the last days: totals, plays per day, top artists and genres.
DFlickable {
    id: root

    readonly property int days: UiConfig.statsDays
    readonly property var stats: StatsService.statistics
    readonly property bool empty: stats !== null && stats.plays === 0
    readonly property bool shown: visible && Window.visibility !== Window.Minimized && Window.visibility !== Window.Hidden
    readonly property string title: UiConfig.pageLabel("stats")
    readonly property bool rangeInHeader: column.width >= titleMetrics.advanceWidth + (Router.canGoBack ? Theme.iconButtonSize + Theme.spacingS : 0) + Theme.spacingM + rangeGroup.naturalWidth
    readonly property bool twoColumns: column.width >= PlayerMetrics.listColumnMinWidth * 2 + PlayerMetrics.sectionSpacing
    readonly property real cardPadding: Theme.spacingL
    readonly property real chartHeight: 132
    readonly property real barGap: Theme.spacingXS
    readonly property real barMaxWidth: 48
    readonly property real barMinHeight: Theme.spacingXS
    readonly property real donutMinSize: 120
    readonly property real donutMaxSize: 200
    // beside Top artists the ring grows into the card's height
    readonly property real donutSize: twoColumns ? Math.max(donutMinSize, Math.min(donutMaxSize, topArtists.implicitHeight - cardPadding * 2 - genresCard.headerHeight - Theme.spacingM)) : donutMinSize
    readonly property real donutStroke: Theme.spacingM
    readonly property real donutGap: Theme.spacingXS
    readonly property real legendDotSize: Theme.spacingM
    readonly property real legendMaxWidth: 280
    // outline rather than outlineVariant: the last share stays visible on the card in light mode
    // without a shell palette tertiary equals secondary, which would paint two genres alike
    readonly property var genreColors: Qt.colorEqual(Theme.tertiary, Theme.secondary) ? [Theme.primary, Theme.info, Theme.success, Theme.outline] : [Theme.primary, Theme.tertiary, Theme.secondary, Theme.outline]
    // the room one day's label needs under the bars: a weekday, or a day and month
    readonly property real weekdayLabelWidth: Theme.fontSizeSmall * 2.5
    readonly property real dateLabelWidth: Theme.fontSizeSmall * 4.5
    // the bars rise and the rings sweep with this, on first show and when the range changes
    property real rise: 1
    readonly property int shownDays: stats?.days ?? 0
    readonly property real riseStaggerTotal: Math.min(PlayerMetrics.listStaggerMax, PlayerMetrics.listStaggerStep * Math.max(0, (stats?.perDay?.length ?? 0) - 1))
    readonly property real riseTime: Theme.expressiveDurations.normal + riseStaggerTotal

    onShownDaysChanged: {
        if (shownDays > 0 && shown && Theme.animationsEnabled) {
            riseAnim.restart();
            return;
        }
        riseAnim.stop();
        rise = 1;
    }

    // how far bar `index` has risen, 0 to 1; each starts a stagger step after the one before it
    function barRise(index) {
        const delay = Math.min(PlayerMetrics.listStaggerMax, PlayerMetrics.listStaggerStep * index);
        return Math.max(0, Math.min(1, (rise * riseTime - delay) / Theme.expressiveDurations.normal));
    }

    function percent(n, total) {
        return I18n.tr("%1%", "percentage, %1 is a whole number").arg(total > 0 ? Math.round(n / total * 100) : 0);
    }

    function dateOf(date) {
        const p = date.split("-");
        return new Date(Number(p[0]), Number(p[1]) - 1, Number(p[2]));
    }

    function dayLabel(date) {
        return dateOf(date).toLocaleDateString(Qt.locale(), Locale.ShortFormat);
    }

    NumberAnimation {
        id: riseAnim
        target: root
        property: "rise"
        from: 0
        to: 1
        duration: root.riseTime
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
    }

    readonly property PageScroll _scroll: PageScroll {
        view: root
    }

    // the service fetches only while this shows; 0 tells it nobody does
    Binding {
        target: StatsService
        property: "statsDays"
        value: root.shown ? root.days : 0
    }
    Component.onDestruction: StatsService.statsDays = 0

    contentHeight: column.height + PlayerMetrics.pagePadding + Router.pageEndInset
    contentWidth: width
    clip: true
    // a square edge fade would paint over the pane's rounded corners
    fadeSideInset: PlayerMetrics.paneRadius

    StyledTextMetrics {
        id: titleMetrics
        font.pixelSize: Theme.fontSizeXXLarge
        text: root.title
    }

    component Card: Rectangle {
        id: card

        property string title: ""
        property string iconName: ""
        // content centred in a card taller than it
        property bool centred: false
        readonly property real headerHeight: titleRow.visible ? titleRow.height : 0
        default property alias content: body.data

        implicitHeight: cardColumn.implicitHeight + root.cardPadding * 2
        // a step above the pane, like the library cards
        color: SettingsMetrics.rowColor
        radius: Theme.cornerRadiusL

        Column {
            id: cardColumn
            x: root.cardPadding
            y: card.centred ? (card.height - height) / 2 : root.cardPadding
            width: parent.width - root.cardPadding * 2
            spacing: Theme.spacingM

            Row {
                id: titleRow
                spacing: Theme.spacingS
                visible: card.title !== ""

                DIcon {
                    anchors.verticalCenter: parent.verticalCenter
                    visible: card.iconName !== ""
                    name: card.iconName
                    size: Theme.iconSize
                    color: Theme.primary
                }

                StyledText {
                    anchors.verticalCenter: parent.verticalCenter
                    text: card.title
                    font.pixelSize: Theme.fontSizeLarge
                    font.weight: Theme.fontWeightMedium
                    color: Theme.surfaceText
                }
            }

            Column {
                id: body
                width: parent.width
            }
        }
    }

    component LegendRow: Row {
        property real natural: 0
    }

    component StatCard: Card {
        id: statCard

        centred: true
        property int value: 0
        property string label: ""
        // what the number reads, for scenarios
        readonly property string text: String(value)

        Accessible.role: Accessible.StaticText
        Accessible.name: Phrases.named(text, label)

        Column {
            width: parent.width

            StyledText {
                width: parent.width
                text: statCard.text
                font.pixelSize: Theme.fontSizeDisplayLarge
                color: Theme.surfaceText
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft
            }

            StyledText {
                width: parent.width
                text: statCard.label
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft
            }
        }
    }

    Column {
        id: column
        x: PlayerMetrics.pagePadding
        width: root.width - PlayerMetrics.pagePadding * 2
        spacing: PlayerMetrics.sectionSpacing
        LayoutMirroring.enabled: I18n.isRtl
        LayoutMirroring.childrenInherit: true

        PageHeader {
            title: root.title

            Item {
                id: headerSlot
                anchors.verticalCenter: parent.verticalCenter
                visible: root.rangeInHeader
                implicitWidth: rangeGroup.width
                implicitHeight: rangeGroup.height
            }
        }

        Item {
            id: underSlot
            width: parent.width
            visible: !root.rangeInHeader
            implicitHeight: rangeGroup.height
        }

        DButtonGroup {
            id: rangeGroup
            objectName: "statsRange"
            parent: root.rangeInHeader ? headerSlot : underSlot
            size: "small"
            model: UiConfig.statsRanges.map(n => I18n.tr("%1 days", "statistics range").arg(n))
            currentIndex: UiConfig.statsRanges.indexOf(root.days)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    SettingsData.statsDays = UiConfig.statsRanges[index];
            }
        }

        EmptyState {
            width: parent.width
            visible: root.empty
            iconName: "insights"
            text: I18n.tr("No plays in the last %1 days.", "statistics empty state").arg(root.stats?.days ?? root.days)
        }

        Column {
            width: parent.width
            spacing: PlayerMetrics.sectionSpacing
            visible: root.stats !== null && !root.empty

            // the chart with the two totals beside it, or under them on narrow windows
            Item {
                id: summary
                readonly property real side: root.twoColumns ? Math.round((width - PlayerMetrics.sectionSpacing) / 3) : width
                width: parent.width
                height: root.twoColumns ? chartCard.height : totals.height + PlayerMetrics.sectionSpacing + chartCard.height

                Grid {
                    id: totals
                    x: root.twoColumns && !I18n.isRtl ? chartCard.width + PlayerMetrics.sectionSpacing : 0
                    width: summary.side
                    columns: root.twoColumns ? 1 : 2
                    spacing: PlayerMetrics.sectionSpacing

                    StatCard {
                        id: playsCard
                        objectName: "statsPlays"
                        width: root.twoColumns ? totals.width : (totals.width - totals.spacing) / 2
                        height: root.twoColumns ? (chartCard.height - totals.spacing) / 2 : implicitHeight
                        value: root.stats?.plays ?? 0
                        label: I18n.tr("Total plays", "statistics: label under the number of plays in the range")
                    }

                    StatCard {
                        objectName: "statsArtists"
                        width: playsCard.width
                        height: playsCard.height
                        value: root.stats?.artists ?? 0
                        label: value === 1 ? I18n.tr("Artist", "track list column") : I18n.tr("Artists", "navigation destination")
                    }
                }

                Card {
                    id: chartCard
                    // as tall as the two totals stacked beside it
                    readonly property real bodyHeight: root.twoColumns ? Math.max(root.chartHeight, playsCard.implicitHeight * 2 + totals.spacing - root.cardPadding * 2 - headerHeight - Theme.spacingM * 2 - dayLabels.height) : root.chartHeight
                    x: root.twoColumns && I18n.isRtl ? summary.side + PlayerMetrics.sectionSpacing : 0
                    y: root.twoColumns ? 0 : totals.height + PlayerMetrics.sectionSpacing
                    width: root.twoColumns ? summary.width - summary.side - PlayerMetrics.sectionSpacing : summary.width
                    title: I18n.tr("Plays per day", "statistics card: a bar per day")
                    iconName: "trending_up"

                    Item {
                        width: parent.width
                        height: chartCard.bodyHeight + Theme.spacingM + dayLabels.height

                        Row {
                            id: bars
                            objectName: "statsDays"

                            readonly property var perDay: root.stats?.perDay ?? []
                            readonly property int most: perDay.reduce((m, d) => Math.max(m, d.plays), 0)
                            readonly property var busiest: perDay.reduce((b, d) => d.plays > (b?.plays ?? 0) ? d : b, null)
                            readonly property real barWidth: perDay.length > 0 ? Math.min(root.barMaxWidth, (parent.width - spacing * (perDay.length - 1)) / perDay.length) : 0
                            readonly property int hovered: chartHover.containsMouse && barWidth > 0 ? Math.min(perDay.length - 1, Math.floor((I18n.isRtl ? width - chartHover.mouseX : chartHover.mouseX) / (barWidth + spacing))) : -1

                            anchors.horizontalCenter: parent.horizontalCenter
                            height: chartCard.bodyHeight
                            spacing: root.barGap
                            Accessible.role: Accessible.Chart
                            Accessible.name: Phrases.named(Phrases.plays(root.stats?.plays ?? 0), busiest ? I18n.tr("Most on %1", "statistics chart: the day with the most plays").arg(root.dayLabel(busiest.date)) : "")

                            Repeater {
                                id: barRepeater
                                model: bars.perDay

                                Item {
                                    id: slot
                                    required property var modelData
                                    required property int index
                                    objectName: "statsBar"
                                    width: bars.barWidth
                                    height: bars.height

                                    Rectangle {
                                        anchors.bottom: parent.bottom
                                        width: parent.width
                                        height: Math.max(root.barMinHeight, bars.most > 0 ? parent.height * slot.modelData.plays / bars.most * root.barRise(slot.index) : 0)
                                        color: slot.modelData.plays > 0 ? Theme.primary : Theme.outlineVariant
                                        topLeftRadius: Theme.cornerRadiusXS
                                        topRightRadius: Theme.cornerRadiusXS
                                        bottomLeftRadius: Theme.cornerRadiusXXS
                                        bottomRightRadius: Theme.cornerRadiusXXS
                                    }
                                }
                            }
                        }

                        // a label every few days, counted back from today so today always has one
                        Row {
                            id: dayLabels
                            readonly property bool weekdays: bars.perDay.length <= 7
                            readonly property int step: Math.max(1, Math.ceil((weekdays ? root.weekdayLabelWidth : root.dateLabelWidth) / Math.max(1, bars.barWidth + bars.spacing)))
                            readonly property string format: weekdays ? "ddd" : "d MMM"
                            x: bars.x
                            y: chartCard.bodyHeight + Theme.spacingM
                            spacing: bars.spacing
                            Accessible.ignored: true

                            Repeater {
                                model: bars.perDay

                                Item {
                                    id: dayLabel
                                    required property var modelData
                                    required property int index
                                    width: bars.barWidth
                                    height: dayText.height

                                    StyledText {
                                        id: dayText
                                        anchors.horizontalCenter: parent.horizontalCenter
                                        visible: (bars.perDay.length - 1 - dayLabel.index) % dayLabels.step === 0
                                        text: root.dateOf(dayLabel.modelData.date).toLocaleDateString(Qt.locale(), dayLabels.format)
                                        font.pixelSize: Theme.fontSizeSmall
                                        color: Theme.surfaceVariantText
                                        wrapMode: Text.NoWrap
                                    }
                                }
                            }
                        }

                        MouseArea {
                            id: chartHover
                            anchors.fill: bars
                            hoverEnabled: true
                            acceptedButtons: Qt.NoButton
                        }

                        DTooltipHost {
                            hoverArea: chartHover
                            target: bars.hovered >= 0 ? barRepeater.itemAt(bars.hovered) : null
                            text: bars.hovered >= 0 ? Phrases.join(root.dayLabel(bars.perDay[bars.hovered].date), Phrases.plays(bars.perDay[bars.hovered].plays)) : ""
                        }
                    }
                }
            }

            Flow {
                width: parent.width
                spacing: PlayerMetrics.sectionSpacing

                Card {
                    id: topArtists
                    objectName: "statsTopArtists"
                    width: root.twoColumns ? (parent.width - parent.spacing) / 2 : parent.width
                    height: root.twoColumns ? Math.max(implicitHeight, genresCard.implicitHeight) : implicitHeight
                    title: I18n.tr("Top artists", "statistics card: the most played artists")

                    Column {
                        id: artistRows
                        readonly property var list: root.stats?.topArtists ?? []
                        width: parent.width
                        spacing: Theme.groupedListGap

                        Repeater {
                            model: artistRows.list

                            DListItem {
                                id: artistRow
                                required property var modelData
                                required property int index
                                readonly property bool linked: modelData.id !== "" && CatalogService.enabledLibrary(modelData.library)

                                objectName: "statsArtist"
                                width: artistRows.width
                                implicitHeight: PlayerMetrics.listRowHeight
                                enabled: linked
                                focusPolicy: linked ? Qt.StrongFocus : Qt.NoFocus
                                surfaceColor: Theme.surfaceContainerHighest
                                firstInGroup: index === 0
                                lastInGroup: index === artistRows.list.length - 1
                                Accessible.role: linked ? Accessible.Link : Accessible.ListItem
                                Accessible.name: Phrases.named(modelData.name, Phrases.plays(modelData.count))
                                onClicked: Router.go("artist", modelData.id)

                                Row {
                                    anchors.fill: parent
                                    anchors.leftMargin: Theme.spacingM
                                    anchors.rightMargin: Theme.spacingM
                                    spacing: Theme.spacingM

                                    StyledText {
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: Theme.spacingM
                                        text: String(artistRow.index + 1)
                                        font.pixelSize: Theme.fontSizeMedium
                                        color: Theme.surfaceVariantText
                                        horizontalAlignment: Text.AlignHCenter
                                    }

                                    DRingGauge {
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: Theme.avatarSize
                                        height: width
                                        value: artistRow.modelData.count / Math.max(1, artistRows.list[0]?.count ?? 0) * root.rise
                                        trackColor: Theme.outlineVariant
                                        strokeWidth: Theme.spacingXXS
                                        animated: false

                                        StyledText {
                                            anchors.centerIn: parent
                                            text: (Array.from(artistRow.modelData.name)[0] ?? "").toUpperCase()
                                            font.pixelSize: Theme.fontSizeSmall
                                            font.weight: Theme.fontWeightMedium
                                            color: Theme.surfaceText
                                        }
                                    }

                                    Column {
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: parent.width - Theme.spacingM - Theme.avatarSize - parent.spacing * 2
                                        spacing: Theme.spacingXXS

                                        StyledText {
                                            width: parent.width
                                            text: artistRow.modelData.name
                                            font.pixelSize: Theme.fontSizeMedium
                                            color: Theme.surfaceText
                                            elide: Text.ElideRight
                                            wrapMode: Text.NoWrap
                                            horizontalAlignment: Text.AlignLeft
                                        }

                                        StyledText {
                                            width: parent.width
                                            text: Phrases.plays(artistRow.modelData.count)
                                            font.pixelSize: Theme.fontSizeSmall
                                            color: Theme.surfaceVariantText
                                            elide: Text.ElideRight
                                            wrapMode: Text.NoWrap
                                            horizontalAlignment: Text.AlignLeft
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                Card {
                    id: genresCard
                    objectName: "statsGenres"

                    readonly property var genres: root.stats?.genres ?? []
                    readonly property int other: root.stats?.otherGenres ?? 0
                    readonly property int total: genres.reduce((n, g) => n + g.count, other)
                    // [{name, count, color, start, span}], angles in degrees from the top, clockwise
                    readonly property var segments: {
                        const list = genres.map((g, i) => ({
                                    "name": g.name,
                                    "count": g.count,
                                    "color": root.genreColors[i]
                                }));
                        if (other > 0)
                            list.push({
                                "name": I18n.tr("Other", "custom page group of the items that have no value to group by"),
                                "count": other,
                                "color": root.genreColors[3]
                            });
                        // round caps reach half a stroke past each end
                        const radius = (root.donutSize - root.donutStroke) / 2;
                        const gap = list.length > 1 ? (root.donutStroke + root.donutGap) / radius * 180 / Math.PI : 0;
                        let at = -90;
                        return list.map(s => {
                            const share = total > 0 ? s.count / total * 360 : 0;
                            const seg = Object.assign(s, {
                                "start": at + gap / 2,
                                "span": Math.max(0, share - gap)
                            });
                            at += share;
                            return seg;
                        });
                    }

                    width: root.twoColumns ? (parent.width - parent.spacing) / 2 : parent.width
                    height: root.twoColumns ? Math.max(implicitHeight, topArtists.implicitHeight) : implicitHeight
                    title: I18n.tr("Genres", "navigation destination")

                    StyledText {
                        width: parent.width
                        visible: genresCard.segments.length === 0
                        text: I18n.tr("No genres in these plays.", "statistics genres card when no played track has a genre")
                        font.pixelSize: Theme.fontSizeMedium
                        color: Theme.surfaceVariantText
                        wrapMode: Text.WordWrap
                        horizontalAlignment: Text.AlignLeft
                    }

                    Row {
                        width: parent.width
                        visible: genresCard.segments.length > 0
                        spacing: Theme.spacingL

                        Item {
                            anchors.verticalCenter: parent.verticalCenter
                            width: root.donutSize
                            height: width
                            Accessible.role: Accessible.Chart
                            Accessible.name: genresCard.segments.map(s => Phrases.named(s.name, root.percent(s.count, genresCard.total))).join(", ")

                            Repeater {
                                model: genresCard.segments

                                DRingGauge {
                                    required property var modelData
                                    anchors.fill: parent
                                    value: 1
                                    startAngle: modelData.start
                                    // one sweep from the top round the whole ring
                                    spanAngle: Math.max(0, Math.min(modelData.span, root.rise * 360 - modelData.start - 90))
                                    strokeWidth: root.donutStroke
                                    ringColor: modelData.color
                                    trackColor: "transparent"
                                    animated: false
                                }
                            }

                            Column {
                                anchors.centerIn: parent
                                width: parent.width - root.donutStroke * 2 - Theme.spacingS * 2

                                StyledText {
                                    width: parent.width
                                    text: genresCard.segments.length > 0 ? root.percent(genresCard.segments[0].count, genresCard.total) : ""
                                    font.pixelSize: Theme.fontSizeXLarge
                                    color: Theme.surfaceText
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.NoWrap
                                }

                                StyledText {
                                    width: parent.width
                                    text: genresCard.segments[0]?.name ?? ""
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: Theme.surfaceVariantText
                                    horizontalAlignment: Text.AlignHCenter
                                    elide: Text.ElideRight
                                    wrapMode: Text.NoWrap
                                }
                            }
                        }

                        Column {
                            id: legend
                            // as wide as the widest row needs, so a percent stays near its name on a wide card
                            readonly property real natural: {
                                let w = 0;
                                for (const row of children)
                                    w = Math.max(w, (row as LegendRow)?.natural ?? 0);
                                return w;
                            }
                            anchors.verticalCenter: parent.verticalCenter
                            width: Math.min(root.legendMaxWidth, natural, parent.width - root.donutSize - parent.spacing)
                            spacing: Theme.spacingS

                            Repeater {
                                model: genresCard.segments

                                LegendRow {
                                    id: legendRow
                                    required property var modelData
                                    natural: root.legendDotSize + legendName.implicitWidth + legendPercent.implicitWidth + spacing * 2
                                    width: legend.width
                                    spacing: Theme.spacingS

                                    Rectangle {
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: root.legendDotSize
                                        height: width
                                        radius: Theme.cornerRadiusXS
                                        color: legendRow.modelData.color
                                    }

                                    StyledText {
                                        id: legendName
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: parent.width - root.legendDotSize - legendPercent.width - parent.spacing * 2
                                        text: legendRow.modelData.name
                                        font.pixelSize: Theme.fontSizeMedium
                                        color: Theme.surfaceText
                                        elide: Text.ElideRight
                                        wrapMode: Text.NoWrap
                                        horizontalAlignment: Text.AlignLeft
                                    }

                                    StyledText {
                                        id: legendPercent
                                        anchors.verticalCenter: parent.verticalCenter
                                        text: root.percent(legendRow.modelData.count, genresCard.total)
                                        font.pixelSize: Theme.fontSizeMedium
                                        font.weight: Theme.fontWeightMedium
                                        color: Theme.surfaceText
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
