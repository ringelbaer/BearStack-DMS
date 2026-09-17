package de.bearstack.people

import android.app.Application
import android.content.ContentValues
import android.graphics.Bitmap
import android.provider.MediaStore
import android.util.Base64
import androidx.activity.ComponentActivity
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.ViewModelStore
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.connection.Profile
import de.bearstack.people.connection.ProfileStore
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.photos.DevicePhotoPreferences
import de.bearstack.people.photos.devicePhotoPermissions
import de.bearstack.people.ui.PeopleApp
import kotlinx.coroutines.*
import okhttp3.mockwebserver.*
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

@SdkSuppress(minSdkVersion=29)
class StartupTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun savedProfileShowsSplashAndOpensGalleryWithoutLoginForm() = scenario("slow")
    @Test fun silentServerTimesOutWhileLocalPhotoRemainsOpenAndRetryKeepsItOpen() = scenario("silent")
    @Test fun unavailableServerKeepsProfileAndAutomaticallyOpensLocalPhotos() = scenario("unavailable")
    @Test fun rejectedCredentialsKeepLocalPhotosAndAllowRetry() = scenario("auth")
    @Test fun changedCertificateKeepsLocalPhotosWithoutTrustingNewCertificate() = scenario("certificate")
    @Test fun firstLaunchOffersSetupAndLocalPhotosWithoutPeopleDatabase() = scenario("new")
    @Test fun serverFailureAfterSignInStillAllowsLocalPhotosWithFolderTileDisabled() = scenario("browse_failure")

    private fun scenario(initial: String) = runBlocking<Unit> {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val app = instrumentation.targetContext.applicationContext as Application
        val profiles = ProfileStore(app)
        profiles.clear()
        val preferences = DevicePhotoPreferences(app)
        val enabled = preferences.enabled
        preferences.enabled = false // Offline access must also work before opting in to the server's folder tile.
        devicePhotoPermissions().forEach { instrumentation.uiAutomation.grantRuntimePermission(app.packageName, it) }
        val uri = app.contentResolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, "offline-startup.jpg")
            put(MediaStore.Images.Media.MIME_TYPE, "image/jpeg")
            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/BearStackOfflineTest")
            put(MediaStore.Images.Media.IS_PENDING, 1)
        })!!
        val bitmap = Bitmap.createBitmap(400, 300, Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.CYAN) }
        app.contentResolver.openOutputStream(uri)!!.use { bitmap.compress(Bitmap.CompressFormat.JPEG, 90, it) }
        bitmap.recycle()
        app.contentResolver.update(uri, ContentValues().apply { put(MediaStore.Images.Media.IS_PENDING, 0) }, null, null)
        val mode = AtomicReference(initial)
        val cert = HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server = MockWebServer()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(cert).build().sslSocketFactory(), false)
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val session = request.path.orEmpty().endsWith("/session")
                if (session) when (mode.get()) {
                    "silent" -> return MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE)
                    "unavailable" -> return MockResponse().setResponseCode(503)
                    "auth" -> return MockResponse().setResponseCode(401)
                }
                if (!session && mode.get() == "browse_failure") return MockResponse().setResponseCode(503)
                check(!request.path.orEmpty().contains("/labeling/")) { "Local/reader flow accessed people endpoint" }
                return MockResponse().setHeader("Content-Type", "application/json").setBody(
                    if (session) """{"protocol":1,"can_manage_people":false,"instance":"instance","dataset":"dataset","account":"reader",
                        "settings":{"thumbnail_size":320,"folder_thumbnail_size":240,"preview_size":1280,"large_preview_size":2048,"slideshow_seconds":5,"frame_seconds":8}}"""
                    else """{"path":"","parent":"","page":1,"total":0,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[],"folders":[],"blogs":[]}"""
                ).apply { if (session && mode.get() == "slow") setHeadersDelay(5, TimeUnit.SECONDS) }
            }
        }
        server.start()
        val storedCert = if(initial == "certificate") HeldCertificate.Builder().addSubjectAlternativeName("localhost").build() else cert
        val profile = Profile(server.url("/").toString(), "reader", "secret",
            Base64.encodeToString(storedCert.certificate.encoded, Base64.NO_WRAP))
        if(initial != "new") profiles.write(profile)
        val database = lazy<LabelingDatabase> { error("Startup/local photos opened the people database") }
        val models = ViewModelStore()
        val vm = withContext(Dispatchers.Main) { PeopleViewModel(app, database).also { models.put("startup", it) } }
        suspend fun idle() { withTimeout(12_000) { while(vm.state.value.busy) delay(10) } }
        fun waitForText(text: String) { compose.waitUntil(10_000) { compose.onAllNodesWithText(text).fetchSemanticsNodes().isNotEmpty() } }
        try {
            compose.setGermanContent { PeopleApp(vm) }
            if(initial == "slow" || initial == "silent") {
                compose.onNodeWithTag("connection-splash").assertIsDisplayed()
                compose.onAllNodes(hasSetTextAction()).assertCountEquals(0)
                if(initial == "slow") {
                    compose.onRoot().captureToImage().asAndroidBitmap().let { image ->
                        File(app.cacheDir, "startup-splash.png").outputStream().use { image.compress(Bitmap.CompressFormat.PNG, 100, it) }
                    }
                } else compose.onNodeWithText("Lokale Fotos öffnen").performClick()
            }
            if(initial == "silent") {
                waitForText("BearStackOfflineTest")
                compose.onNodeWithText("BearStackOfflineTest").performClick()
                compose.waitUntil(10_000) { compose.onAllNodesWithContentDescription("offline-startup.jpg").fetchSemanticsNodes().isNotEmpty() }
                compose.onNodeWithContentDescription("offline-startup.jpg").performClick()
                compose.onNodeWithContentDescription("Informationen").assertIsDisplayed()
            }
            idle()
            assertFalse(vm.state.value.restoring)
            assertFalse(database.isInitialized())
            if(initial == "slow" || initial == "browse_failure") {
                assertTrue(vm.state.value.connected)
                assertNotNull(vm.photos)
                compose.onAllNodes(hasSetTextAction()).assertCountEquals(0)
                if(initial == "browse_failure") {
                    withTimeout(5_000) { while(vm.photos!!.state.value.loading) delay(10) }
                    assertNotNull(vm.photos!!.state.value.error)
                    compose.onNodeWithContentDescription("Weitere Optionen").performClick()
                    compose.onNodeWithText("Einstellungen").performClick()
                    compose.onNodeWithText("Lokale Fotoordner anzeigen").performClick()
                    compose.onNodeWithText("Schließen").performClick()
                    compose.onNodeWithText("Ordner").performClick()
                    compose.onNodeWithText("Dieses Gerät").performClick()
                    waitForText("BearStackOfflineTest")
                }
            } else if(initial == "new") {
                assertFalse(vm.state.value.savedConnection)
                compose.onAllNodes(hasSetTextAction()).assertCountEquals(3)
                compose.onNodeWithText("Lokale Fotos öffnen").performClick()
                waitForText("BearStackOfflineTest")
                assertNull(profiles.read())
            } else {
                assertFalse(vm.state.value.connected)
                assertNotNull(vm.state.value.error)
                assertEquals(profile, profiles.read())
                compose.onAllNodes(hasSetTextAction()).assertCountEquals(0)
                if(initial == "auth") assertEquals(R.string.error_auth, vm.state.value.error!!.resource)
                if(initial == "silent") {
                    // Timeout must leave the local viewer intact, including decoded media.
                    compose.onNodeWithContentDescription("Informationen").performClick()
                    waitForText("offline-startup.jpg")
                    androidx.test.espresso.Espresso.pressBack()
                    compose.onNodeWithContentDescription("Schließen").performClick()
                } else waitForText("BearStackOfflineTest")
                if(initial != "certificate") {
                    mode.set("healthy")
                    compose.onNodeWithText("Erneut versuchen").performClick()
                    idle()
                    assertTrue("retry: ${vm.state.value.error}", vm.state.value.connected)
                    assertEquals(profile, profiles.read())
                    compose.onNodeWithTag("connection-splash").assertDoesNotExist()
                    if(initial == "silent") compose.onNodeWithContentDescription("offline-startup.jpg").assertIsDisplayed()
                    else compose.onNodeWithText("BearStackOfflineTest").assertIsDisplayed()
                    // The server tabs become available without abandoning the local folder.
                    compose.onNodeWithText("Fotos").performClick()
                    compose.onNodeWithText("Dieses Gerät").assertDoesNotExist()
                }
            }
        } finally {
            withContext(Dispatchers.Main) { models.clear() }
            profiles.clear()
            preferences.enabled = enabled
            app.contentResolver.delete(uri, null, null)
            server.close()
        }
    }
}
