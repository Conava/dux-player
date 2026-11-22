import QtQuick
import qs.Common
import qs.DCommon.Widgets as Up

// The shared StyledText whose opacity fade honors reduce motion and animation speed None, until the shared one does.
// Replaces the shared widget only where `import qs.Widgets` comes after the DCommon import.
Up.StyledText {
    Behavior on opacity {
        enabled: Theme.animationsEnabled
        NumberAnimation {
            duration: Theme.currentAnimationBaseDuration * 1.2
            easing.type: Easing.OutCubic
        }
    }
}
