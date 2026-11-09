pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// ListenBrainz and Last.fm accounts for plays; each library picks its destinations in its own settings.
SettingsPage {
    id: root

    readonly property var listenbrainz: PlayerService.scrobble.listenbrainz ?? {}
    readonly property var lastfm: PlayerService.scrobble.lastfm ?? {}
    readonly property bool builtIn: PlayerService.scrobble.lastfmBuiltIn === true
    property bool ownKey: false
    readonly property bool keyFields: !builtIn || ownKey
    readonly property string signInFirst: I18n.tr("Available after signing in.", "why a scrobbling setting is hidden")

    function accountLine(account) {
        return account.user ? I18n.tr("Signed in as %1", "scrobbling account state, %1 is the user name").arg(account.user) : I18n.tr("Signed in", "scrobbling account state");
    }

    SettingsPageIntro {
        text: I18n.tr("A play is scrobbled after half the track or four minutes, whichever comes first. Each library chooses where its plays go.", "scrobbling settings intro")
    }

    SettingsCard {
        visible: (PlayerService.scrobble.queued ?? 0) > 0

        SettingsRow {
            searchable: false
            title: PlayerService.scrobble.queued === 1 ? I18n.tr("1 scrobble pending", "scrobbling queue state, singular") : I18n.tr("%1 scrobbles pending", "scrobbling queue state, plural").arg(PlayerService.scrobble.queued ?? 0)
            subtitle: I18n.tr("Sent automatically when the connection is restored.", "scrobbling queue state description")
        }
    }

    // brand names stay untranslated
    SettingsCard {
        title: "ListenBrainz"
        keywords: "account token"

        SettingsToggleRow {
            visible: root.listenbrainz.connected === true
            unavailableReason: root.signInFirst
            text: I18n.tr("Scrobble to ListenBrainz", "scrobbling setting")
            description: root.accountLine(root.listenbrainz)
            checked: root.listenbrainz.enabled === true
            onToggled: value => PlayerService.setScrobbleEnabled("listenbrainz", value)
        }

        SettingsRow {
            visible: root.listenbrainz.connected === true
            unavailableReason: root.signInFirst
            title: I18n.tr("Account", "scrobbling settings row")

            Row {
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingS

                DButton {
                    text: I18n.tr("Test", "scrobbling account action")
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    onClicked: PlayerService.scrobbleTest("listenbrainz")
                }

                DButton {
                    text: I18n.tr("Sign out", "scrobbling account action")
                    backgroundColor: "transparent"
                    textColor: Theme.primary
                    onClicked: PlayerService.scrobbleDisconnect("listenbrainz")
                }
            }
        }

        SettingsFieldRow {
            id: tokenRow
            visible: root.listenbrainz.connected !== true
            unavailableReason: root.accountLine(root.listenbrainz)
            title: I18n.tr("User token", "ListenBrainz sign-in field")
            subtitle: I18n.tr("Find it in your ListenBrainz profile settings. Stored in the system keyring.", "ListenBrainz sign-in hint")
            field.echoMode: TextInput.Password
            field.showPasswordToggle: true
            onAccepted: connectListenBrainz.clicked()
        }

        SettingsRow {
            visible: root.listenbrainz.connected !== true
            subtitle: connectListenBrainz.enabled ? "" : I18n.tr("Enter your user token first.", "why ListenBrainz sign-in is disabled")

            DButton {
                id: connectListenBrainz
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Sign in", "scrobbling account action")
                enabled: tokenRow.field.text.trim() !== ""
                onClicked: PlayerService.scrobbleConnect("listenbrainz", {
                    "token": tokenRow.field.text.trim()
                }, () => tokenRow.field.text = "")
            }
        }
    }

    SettingsCard {
        title: "Last.fm"
        keywords: "lastfm account"

        SettingsToggleRow {
            visible: root.lastfm.connected === true
            unavailableReason: root.signInFirst
            text: I18n.tr("Scrobble to Last.fm", "scrobbling setting")
            description: root.accountLine(root.lastfm)
            checked: root.lastfm.enabled === true
            onToggled: value => PlayerService.setScrobbleEnabled("lastfm", value)
        }

        SettingsRow {
            visible: root.lastfm.connected === true
            unavailableReason: root.signInFirst
            title: I18n.tr("Account", "scrobbling settings row")

            Row {
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingS

                DButton {
                    text: I18n.tr("Test", "scrobbling account action")
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    onClicked: PlayerService.scrobbleTest("lastfm")
                }

                DButton {
                    text: I18n.tr("Sign out", "scrobbling account action")
                    backgroundColor: "transparent"
                    textColor: Theme.primary
                    onClicked: PlayerService.scrobbleDisconnect("lastfm")
                }
            }
        }

        // step two: the user approves the app on last.fm, then comes back here
        SettingsRow {
            visible: root.lastfm.connected !== true && root.lastfm.awaitingApproval === true
            title: I18n.tr("Approve Dux Player on Last.fm", "Last.fm sign-in step")
            subtitle: I18n.tr("Approve access in your browser, then select Finish sign-in.", "Last.fm sign-in step description")

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Finish sign-in", "Last.fm sign-in action")
                onClicked: PlayerService.scrobbleFinishAuth()
            }
        }

        SettingsFieldRow {
            id: keyRow
            visible: root.lastfm.connected !== true && root.keyFields
            unavailableReason: root.accountLine(root.lastfm)
            title: I18n.tr("API key", "API key field")
            subtitle: I18n.tr("Create an API account at last.fm/api. The key and secret are stored in the system keyring.", "Last.fm sign-in hint")
        }

        SettingsFieldRow {
            id: secretRow
            visible: root.lastfm.connected !== true && root.keyFields
            unavailableReason: root.accountLine(root.lastfm)
            title: I18n.tr("Shared secret", "Last.fm sign-in field")
            field.echoMode: TextInput.Password
            field.showPasswordToggle: true
            onAccepted: connectLastfm.clicked()
        }

        SettingsRow {
            visible: root.lastfm.connected !== true
            subtitle: connectLastfm.enabled ? "" : I18n.tr("Enter the API key and shared secret first.", "why Last.fm sign-in is disabled")

            Row {
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingS

                DButton {
                    visible: root.builtIn && !root.ownKey
                    text: I18n.tr("Use your own API key", "Last.fm sign-in action")
                    backgroundColor: "transparent"
                    textColor: Theme.primary
                    onClicked: root.ownKey = true
                }

                DButton {
                    id: connectLastfm
                    text: root.builtIn && !root.ownKey ? I18n.tr("Connect", "Last.fm sign-in action") : I18n.tr("Sign in", "scrobbling account action")
                    reserveText: root.builtIn && !root.ownKey ? I18n.tr("Sign in", "scrobbling account action") : I18n.tr("Connect", "Last.fm sign-in action")
                    backgroundColor: root.lastfm.awaitingApproval ? Theme.secondaryContainer : Theme.primary
                    textColor: root.lastfm.awaitingApproval ? Theme.onSecondaryContainer : Theme.onPrimary
                    enabled: !root.keyFields || (keyRow.field.text.trim() !== "" && secretRow.field.text.trim() !== "")
                    onClicked: PlayerService.scrobbleConnect("lastfm", root.keyFields ? {
                        "apiKey": keyRow.field.text.trim(),
                        "apiSecret": secretRow.field.text.trim()
                    } : {}, () => secretRow.field.text = "")
                }
            }
        }
    }
}
