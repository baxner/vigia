package com.baxner.vigia.ui.widget

import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetReceiver

/** Puente de Android hacia [VigiaWidget] (ver `AndroidManifest.xml` y `res/xml/vigia_widget_info.xml`). */
class VigiaWidgetReceiver : GlanceAppWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = VigiaWidget()
}
