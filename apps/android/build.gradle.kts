// Keep AGP's built-in Kotlin compiler aligned with the Compose compiler plugin.
buildscript {
    dependencies { classpath("org.jetbrains.kotlin:kotlin-gradle-plugin:2.4.20") }
}

plugins {
    id("com.android.application") version "9.4.1" apply false
    id("org.jetbrains.kotlin.plugin.compose") version "2.4.20" apply false
    id("com.google.devtools.ksp") version "2.3.12" apply false
}
