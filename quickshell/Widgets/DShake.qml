import QtQuick
import qs.DCommon.Common

// A short decaying side-to-side shake for a refused input. Declare it as the item's transform
// (`transform: DShake { id: shake }`) and call shake(). Does nothing with reduce motion or animation speed None.
Translate {
    id: root

    property real distance: Style.spacingS
    property int cycles: 3
    // 0 at rest, 1 at the end of a shake
    property real progress: 0

    function shake() {
        if (Style.reduceMotion || Style.currentAnimationSpeed === Style.AnimationSpeed.None)
            return;
        run.restart();
    }

    x: (I18n.isRtl ? -1 : 1) * distance * (1 - progress) * Math.sin(progress * cycles * 2 * Math.PI)

    readonly property NumberAnimation run: NumberAnimation {
        target: root
        property: "progress"
        from: 0
        to: 1
        duration: Style.expressiveDurations.expressiveDefaultSpatial
        easing.type: Easing.Linear
        onStopped: root.progress = 0
    }
}
