pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// ListenBrainz and Last.fm accounts for plays from local libraries; server libraries report to their server.
SettingsPage {
    id: root

    readonly property var listenbrainz: PlayerService.scrobble.listenbrainz ?? {}
    readonly property var lastfm: PlayerService.scrobble.lastfm ?? {}
    readonly property var disabled: PlayerService.scrobble.disabledLibraries ?? []
    // a build with the app's own Last.fm key signs in with one button; an own key is the advanced path
    readonly property bool lastfmBuiltinKey: PlayerService.scrobble.lastfmBuiltinKey === true
    property bool lastfmOwnKey: false
    readonly property bool lastfmKeyFields: root.lastfm.connected !== true && (!lastfmBuiltinKey || lastfmOwnKey)

    function accountLine(account) {
        return account.user ? I18n.tr("Signed in as %1", "scrobbling account state, %1 is the user name").arg(account.user) : I18n.tr("Signed in", "scrobbling account state");
    }

    SettingsPageIntro {
        text: I18n.tr("A play is scrobbled after half the track or four minutes, whichever comes first. Server libraries scrobble to their server, which may forward the play. Local libraries scrobble to the accounts below. No play is scrobbled twice.", "scrobbling settings intro")
    }

    // brand names stay untranslated
    SettingsCard {
        title: "ListenBrainz"

        SettingsToggleRow {
            visible: root.listenbrainz.connected === true
            text: I18n.tr("Scrobble to ListenBrainz", "scrobbling setting")
            description: root.accountLine(root.listenbrainz)
            checked: root.listenbrainz.enabled === true
            onToggled: value => PlayerService.setScrobbleEnabled("listenbrainz", value)
        }

        SettingsRow {
            visible: root.listenbrainz.connected === true
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

        SettingsToggleRow {
            visible: root.lastfm.connected === true
            text: I18n.tr("Scrobble to Last.fm", "scrobbling setting")
            description: root.accountLine(root.lastfm)
            checked: root.lastfm.enabled === true
            onToggled: value => PlayerService.setScrobbleEnabled("lastfm", value)
        }

        SettingsRow {
            visible: root.lastfm.connected === true
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
            subtitle: I18n.tr("Allow access on the Last.fm page in your browser, then select Finish sign-in.", "Last.fm sign-in step description")

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Finish sign-in", "Last.fm sign-in action")
                onClicked: PlayerService.scrobbleFinishAuth()
            }
        }

        SettingsRow {
            visible: root.lastfm.connected !== true && root.lastfmBuiltinKey
            title: I18n.tr("Last.fm account", "Last.fm sign-in row")
            subtitle: I18n.tr("Opens Last.fm in your browser to sign in.", "Last.fm sign-in hint with the app's own key")

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Sign in", "scrobbling account action")
                backgroundColor: root.lastfm.awaitingApproval ? Theme.secondaryContainer : Theme.primary
                textColor: root.lastfm.awaitingApproval ? Theme.onSecondaryContainer : Theme.onPrimary
                onClicked: PlayerService.scrobbleConnect("lastfm", {})
            }
        }

        SettingsRow {
            visible: root.lastfm.connected !== true && root.lastfmBuiltinKey
            title: I18n.tr("Use your own API key", "Last.fm advanced sign-in option")
            clickable: true
            Accessible.checkable: true
            Accessible.checked: root.lastfmOwnKey
            onClicked: root.lastfmOwnKey = !root.lastfmOwnKey

            DIcon {
                anchors.verticalCenter: parent.verticalCenter
                name: root.lastfmOwnKey ? "expand_less" : "expand_more"
                size: Theme.iconSize
                color: Theme.surfaceVariantText
            }
        }

        SettingsFieldRow {
            id: keyRow
            visible: root.lastfmKeyFields
            title: I18n.tr("API key", "API key field")
            subtitle: I18n.tr("Create an API account at last.fm/api and enter its key and shared secret. Stored in the system keyring.", "Last.fm sign-in hint")
        }

        SettingsFieldRow {
            id: secretRow
            visible: root.lastfmKeyFields
            title: I18n.tr("Shared secret", "Last.fm sign-in field")
            field.echoMode: TextInput.Password
            field.showPasswordToggle: true
            onAccepted: connectLastfm.clicked()
        }

        SettingsRow {
            visible: root.lastfmKeyFields
            subtitle: connectLastfm.enabled ? "" : I18n.tr("Enter the API key and shared secret first.", "why Last.fm sign-in is disabled")

            DButton {
                id: connectLastfm
                anchors.verticalCenter: parent.verticalCenter
                text: I18n.tr("Sign in", "scrobbling account action")
                backgroundColor: root.lastfm.awaitingApproval || root.lastfmBuiltinKey ? Theme.secondaryContainer : Theme.primary
                textColor: root.lastfm.awaitingApproval || root.lastfmBuiltinKey ? Theme.onSecondaryContainer : Theme.onPrimary
                enabled: keyRow.field.text.trim() !== "" && secretRow.field.text.trim() !== ""
                onClicked: PlayerService.scrobbleConnect("lastfm", {
                    "apiKey": keyRow.field.text.trim(),
                    "apiSecret": secretRow.field.text.trim()
                }, () => secretRow.field.text = "")
            }
        }
    }

    SettingsCard {
        title: I18n.tr("Libraries", "the music libraries: page, settings tab, section")

        Repeater {
            model: PlayerService.libraries

            SettingsToggleRow {
                required property var modelData
                readonly property bool viaServer: PlayerService.hasCapability(modelData.id, "scrobble")
                readonly property bool silentServer: PlayerService.isRemote(modelData.id) && !viaServer
                text: modelData.name
                description: viaServer ? I18n.tr("Scrobbled to the server", "scrobbling library row") : silentServer ? I18n.tr("This server doesn't support scrobbling", "scrobbling library row") : I18n.tr("Scrobbled to your accounts", "scrobbling library row")
                enabled: !silentServer
                checked: !silentServer && root.disabled.indexOf(modelData.id) < 0
                onToggled: value => PlayerService.setScrobbleLibrary(modelData.id, value)
            }
        }

        SettingsRow {
            visible: (PlayerService.scrobble.queued ?? 0) > 0
            title: PlayerService.scrobble.queued === 1 ? I18n.tr("1 scrobble pending", "scrobbling queue state, singular") : I18n.tr("%1 scrobbles pending", "scrobbling queue state, plural").arg(PlayerService.scrobble.queued ?? 0)
            subtitle: I18n.tr("Sent automatically when the connection is restored.", "scrobbling queue state description")
        }
    }
}
