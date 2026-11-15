# Vigía ProGuard / R8 Rules

# Motor en Go (gomobile AAR y puentes JNI)
-keep class com.baxner.vigia.engine.** { *; }
-keep class go.** { *; }
-dontwarn com.baxner.vigia.engine.**
-dontwarn go.**

# kotlinx.serialization (@Serializable)
-keepattributes *Annotation*, InnerClasses, Signature, RuntimeVisibleAnnotations
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

-if @kotlinx.serialization.Serializable class **
-keepclassmembers class <1> {
    static <1>$Companion Companion;
}

-if @kotlinx.serialization.Serializable class ** {
    static **$* *;
}
-keepclassmembers class <2>$<3> {
    kotlinx.serialization.KSerializer serializer(...);
}

-if @kotlinx.serialization.Serializable class **
-keep @kotlinx.serialization.internal.GeneratedSerializer class <1>$$serializer { *; }

-if @kotlinx.serialization.Serializable class **
-keepclassmembers class <1> {
    synthetic <init>(...);
    *** write$Self(...);
}

-keep @kotlinx.serialization.Serializable class com.baxner.vigia.** { *; }
-keepclassmembers class com.baxner.vigia.**$Companion {
    kotlinx.serialization.KSerializer serializer(...);
}
-keepclassmembers class com.baxner.vigia.**$$serializer {
    *;
}

# OkHttp / Okio / Tink (security-crypto)
-dontwarn okhttp3.**
-dontwarn okio.**
-dontwarn javax.annotation.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
-dontwarn com.google.errorprone.annotations.**
-dontwarn com.google.crypto.tink.**
-keepnames class okhttp3.internal.publicsuffix.PublicSuffixDatabase

# Room (entidades, DAOs y base de datos generada)
-keep class * extends androidx.room.RoomDatabase { *; }
-keep @androidx.room.Entity class * { *; }
-keep @androidx.room.Dao class * { *; }
-keep class com.baxner.vigia.data.history.** { *; }
-dontwarn androidx.room.paging.**

# Jetpack Glance (AppWidget y receptores)
-keep class * extends androidx.glance.appwidget.GlanceAppWidget { *; }
-keep class * extends androidx.glance.appwidget.GlanceAppWidgetReceiver { *; }
-keep class * implements androidx.glance.appwidget.action.ActionCallback { *; }
-keep class com.baxner.vigia.ui.widget.** { *; }
