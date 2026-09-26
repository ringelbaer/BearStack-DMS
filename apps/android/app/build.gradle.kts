plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("com.google.devtools.ksp")
}
val signingPath = providers.environmentVariable("BEARSTACK_ANDROID_KEYSTORE").orNull
val releaseSmoke = providers.gradleProperty("bearstack.releaseSmoke").orNull == "true"
android {
    namespace = "de.bearstack.people"
    compileSdk = 37
    sourceSets["androidTest"].assets.directories.add("schemas")
    // Reuse the repository's photo fixture for visual checks, only in test APKs.
    sourceSets["androidTest"].assets.directories.add(rootProject.file("../../_site-src/docs/assets/images").path)
    defaultConfig {
        applicationId = "de.bearstack.people"
        minSdk = 26
        // API 37 requires a new LAN permission flow for self-hosted servers.
        // Keep the tested target until that flow and denial/retry tests exist.
        //noinspection OldTargetApi
        targetSdk = 36
        versionCode = 49
        versionName = rootProject.file("VERSION").readText().trim()
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    if (signingPath != null) signingConfigs.create("privateRelease") {
        storeFile = file(signingPath)
        storePassword = providers.environmentVariable("BEARSTACK_ANDROID_STORE_PASSWORD").get()
        keyAlias = providers.environmentVariable("BEARSTACK_ANDROID_KEY_ALIAS").get()
        keyPassword = providers.environmentVariable("BEARSTACK_ANDROID_KEY_PASSWORD").get()
    }
    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (signingPath != null) signingConfig = signingConfigs.getByName("privateRelease")
            // Only the explicit release smoke build uses the local test key.
            // Normal release packaging retains the production signing policy.
            else if (releaseSmoke) signingConfig = signingConfigs.getByName("debug")
        }
    }
    buildFeatures { compose = true; buildConfig = true }
    androidResources { localeFilters += listOf("de", "en") }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    testOptions { unitTests.isReturnDefaultValues = true }
    lint { warningsAsErrors = true }
    packaging { resources.excludes += "/META-INF/{AL2.0,LGPL2.1}" }
}
kotlin { compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) } }
ksp { arg("room.schemaLocation", "$projectDir/schemas") }
dependencies {
    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    debugImplementation("androidx.compose.ui:ui-tooling")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.11.0")
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
    implementation("io.coil-kt:coil-compose:2.7.0")
    implementation("androidx.media3:media3-exoplayer:1.11.1")
    implementation("androidx.media3:media3-ui:1.11.1")
    implementation("androidx.media3:media3-datasource-okhttp:1.11.1")
    implementation("androidx.room:room-runtime:2.8.5")
    ksp("androidx.room:room-compiler:2.8.5")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:5.5.0")
    testImplementation("org.json:json:20260814")
    androidTestImplementation(platform("androidx.compose:compose-bom:2026.09.00"))
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.7.0")
    androidTestImplementation("com.squareup.okhttp3:mockwebserver:5.5.0")
    androidTestImplementation("com.squareup.okhttp3:okhttp-tls:5.5.0")
}
