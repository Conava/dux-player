pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import Quickshell

// A server library's address and sign-in: a password, tested before it is saved, or a Quick Connect code.
Column {
    id: root

    // the library signing in again, "" for a new one
    property string libraryId: ""
    property string kind: ""
    // a new library's name, for its Quick Connect sign-in
    property string name: ""
    // "" | "testing" | "ok" | "failed"
    property string testState: ""
    property string testMessage: ""
    // "password" | "quickConnect"
    property string method: "password"
    // submit() waits for the connection test it started
    property bool savePending: false
    property int testTicket: 0

    readonly property bool editing: libraryId !== ""
    readonly property var library: editing ? PlayerService.library(libraryId) : null
    readonly property string url: urlField.text.trim()
    readonly property string user: userField.text.trim()
    readonly property string password: passwordField.text
    readonly property var quickConnect: CatalogService.quickConnect
    readonly property string quickState: quickConnect.state
    // a library that signs in with a code has no password to ask for, until its sign-in expires
    readonly property bool codeLibrary: editing && library?.auth === "quickConnect"
    readonly property bool signInExpired: editing && library?.errorCode === "signInExpired"
    readonly property bool quickOffered: editing ? (library?.capabilities ?? []).indexOf("quickConnect") >= 0 && url === (library?.url ?? "") : CatalogService.quickConnectKinds.indexOf(kind) >= 0
    readonly property bool quickMode: quickOffered && method === "quickConnect"
    readonly property bool passwordForm: !quickMode
    readonly property bool keepsPassword: editing && passwordField.text === "" && !noPassword.checked
    // a login is tested before it is saved, unless the saved password is kept
    readonly property bool needsTest: testState !== "ok" && !keepsPassword
    readonly property bool quickBusy: quickMode && ["requesting", "waiting", "approved"].indexOf(quickState) >= 0
    readonly property bool valid: blocker === "" && !quickMode
    readonly property string blocker: {
        if (codeLibrary && keepsPassword)
            return I18n.tr("Enter the password.", "edit server dialog, why save is disabled");
        if (url === "" || user === "")
            return I18n.tr("Enter the server address and user name.", "add server dialog, why add is disabled");
        if (editing && url !== (library?.url ?? "") && keepsPassword)
            return I18n.tr("Enter the password again for a new server address.", "edit server dialog, why save is disabled");
        return "";
    }

    // the fields typed in
    signal edited
    // submit() passed: the sign-in was tested or needs no test
    signal ready

    function reset(address, account) {
        urlField.text = address;
        userField.text = account;
        passwordField.text = "";
        noPassword.checked = false;
        method = "password";
        CatalogService.cancelQuickConnect();
        invalidateTest();
        testMessage = "";
        if (signInExpired && codeLibrary && quickOffered)
            selectMethod("quickConnect");
    }

    function clear() {
        passwordField.text = "";
        invalidateTest();
        CatalogService.cancelQuickConnect();
    }

    function focusFirst() {
        urlField.forceActiveFocus();
    }

    function startQuickConnect() {
        if (url === "")
            return;
        CatalogService.startQuickConnect(libraryId, kind, name, url);
    }

    function selectMethod(next) {
        method = next;
        invalidateTest();
        if (next === "quickConnect")
            startQuickConnect();
        else
            CatalogService.cancelQuickConnect();
    }

    // A field edit or closing makes a running test's reply stale.
    function invalidateTest() {
        testTicket++;
        testState = "";
        savePending = false;
    }

    function runTest() {
        testState = "testing";
        PlayerService.testServer(kind, url, user, passwordField.text, ++testTicket);
    }

    function submit() {
        if (!valid || testState === "testing")
            return;
        if (needsTest) {
            savePending = true;
            runTest();
            return;
        }
        ready();
    }

    // the libraries.update fields of an edit; a password left out keeps the saved one
    function patch() {
        return {
            "url": url,
            "user": user,
            "password": noPassword.checked ? "" : passwordField.text || undefined,
            // back to a password, from a library that signed in with a code
            "auth": codeLibrary ? "" : undefined
        };
    }

    width: parent?.width ?? 0
    onKindChanged: {
        invalidateTest();
        method = "password";
        CatalogService.cancelQuickConnect();
    }
    Component.onDestruction: CatalogService.cancelQuickConnect()

    Connections {
        target: PlayerService
        function onServerTestFinished(ok, message, ticket) {
            if (ticket !== root.testTicket || root.testState !== "testing")
                return;
            root.testState = ok ? "ok" : "failed";
            root.testMessage = message;
            const pending = root.savePending;
            root.savePending = false;
            if (ok && pending)
                root.ready();
        }
    }

    StyledText {
        width: parent.width
        visible: root.signInExpired
        text: Phrases.reason("signInExpired")
        font.pixelSize: Theme.fontSizeMedium
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    DTextField {
        id: urlField
        width: parent.width
        outlined: true
        labelText: I18n.tr("Server address", "add server dialog field")
        placeholderText: I18n.tr("https://music.example.net", "example server address in the add server dialog")
        leftIconName: "dns"
        enabled: !root.quickBusy
        onTextChanged: {
            root.invalidateTest();
            root.edited();
            if (["off", "failed", "expired"].indexOf(root.quickState) >= 0)
                CatalogService.cancelQuickConnect();
        }
    }

    DButtonGroup {
        visible: root.quickOffered
        model: [
            {
                "text": I18n.tr("Password", "add server dialog field"),
                "icon": "key"
            },
            {
                "text": I18n.tr("Quick Connect", "add server dialog sign-in method"),
                "icon": "qr_code"
            }
        ]
        currentIndex: root.method === "quickConnect" ? 1 : 0
        onSelectionChanged: (index, selected) => {
            if (selected && root.method !== (index === 1 ? "quickConnect" : "password"))
                root.selectMethod(index === 1 ? "quickConnect" : "password");
        }
    }

    SettingsGroup {
        visible: root.quickMode
        width: parent.width

        SettingsRow {
            visible: root.quickState === "requesting" || root.quickState === "approved"
            title: root.quickState === "approved" ? I18n.tr("Approved. Signing in…", "Quick Connect state") : I18n.tr("Getting a code…", "Quick Connect state")
            searchable: false

            DLoadingIndicator {
                anchors.verticalCenter: parent.verticalCenter
                size: Theme.iconSizeMedium
            }
        }

        SettingsRow {
            visible: root.quickState === "" && root.url === ""
            title: I18n.tr("Enter the server address.", "Quick Connect state before a code can be asked for")
            iconName: "dns"
            searchable: false
        }

        SettingsRow {
            visible: root.quickState === "waiting"
            title: root.quickConnect.code
            titleColor: Theme.primary
            subtitle: I18n.tr("On a signed-in device, open Settings, then Quick Connect, and enter this code.", "Quick Connect instructions")
            searchable: false

            DActionButton {
                anchors.verticalCenter: parent.verticalCenter
                iconName: "content_copy"
                tooltipText: I18n.tr("Copy", "copy a config snippet")
                Accessible.name: tooltipText
                onClicked: {
                    Quickshell.clipboardText = root.quickConnect.code;
                    Router.notify(I18n.tr("Copied", "toast after text was copied to the clipboard"));
                }
            }
        }

        SettingsRow {
            visible: root.quickState === "waiting"
            title: I18n.tr("Waiting for approval", "Quick Connect state")
            subtitle: I18n.tr("The code expires after 10 minutes.", "Quick Connect hint")
            searchable: false

            DLoadingIndicator {
                anchors.verticalCenter: parent.verticalCenter
                size: Theme.iconSizeMedium
            }
        }

        SettingsRow {
            visible: ["failed", "expired", "off"].indexOf(root.quickState) >= 0
            title: root.quickState === "expired" ? I18n.tr("The code expired", "Quick Connect state") : root.quickState === "off" ? Phrases.reason("quickConnectOff") : I18n.tr("Couldn't sign in with Quick Connect", "Quick Connect state")
            subtitle: root.quickState === "expired" ? I18n.tr("It wasn't approved in time.", "Quick Connect state") : root.quickState === "off" ? I18n.tr("Sign in with a password instead.", "Quick Connect state") : Phrases.reason(root.quickConnect.errorCode, "")
            iconName: "error"
            iconColor: Theme.error
            searchable: false
        }
    }

    DButton {
        visible: root.quickMode && (root.quickState === "failed" || root.quickState === "expired" || (root.quickState === "" && root.url !== ""))
        text: root.quickState === "expired" ? I18n.tr("Get a new code", "Quick Connect action") : root.quickState === "" ? I18n.tr("Get a code", "Quick Connect action") : I18n.tr("Try again", "library action that checks an unavailable library again")
        backgroundColor: Theme.secondaryContainer
        textColor: Theme.onSecondaryContainer
        enabled: root.url !== ""
        onClicked: root.startQuickConnect()
    }

    DTextField {
        id: userField
        visible: root.passwordForm
        width: parent.width
        outlined: true
        labelText: I18n.tr("User name", "add server dialog field")
        leftIconName: "person"
        onTextChanged: {
            root.invalidateTest();
            root.edited();
        }
    }

    DTextField {
        id: passwordField
        visible: root.passwordForm
        width: parent.width
        outlined: true
        labelText: I18n.tr("Password", "add server dialog field")
        supportingText: root.editing ? noPassword.checked ? "" : I18n.tr("Leave empty to keep the saved password. Stored in the system keyring.", "edit server dialog password hint") : I18n.tr("Leave empty if the account has none. Stored in the system keyring.", "add server dialog password hint")
        enabled: !noPassword.checked
        echoMode: TextInput.Password
        showPasswordToggle: true
        leftIconName: "key"
        onTextChanged: {
            root.invalidateTest();
            root.edited();
        }
    }

    DToggle {
        id: noPassword
        visible: root.passwordForm && root.editing
        width: parent.width
        text: I18n.tr("Account has no password", "edit server dialog toggle")
        onToggled: value => {
            checked = value;
            passwordField.text = "";
            root.invalidateTest();
            root.edited();
        }
    }

    Row {
        visible: root.passwordForm
        spacing: Theme.spacingM

        DButton {
            text: I18n.tr("Test connection", "add server dialog action")
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            busy: root.testState === "testing"
            enabled: root.testState !== "testing" && root.url !== ""
            onClicked: root.runTest()
        }

        StyledText {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.testState === "ok" || root.testState === "failed"
            text: root.testState === "ok" ? I18n.tr("Connected", "add server dialog, connection test passed") : I18n.tr("Couldn't connect to server", "add server dialog, connection test failed title")
            font.pixelSize: Theme.fontSizeMedium
            color: root.testState === "ok" ? Theme.primary : Theme.error
        }
    }

    StyledText {
        width: parent.width
        visible: root.passwordForm && root.testState === "failed" && root.testMessage !== ""
        text: root.testMessage
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }
}
