package de.bearstack.people

import android.content.ContentValues
import android.content.res.Configuration
import android.graphics.Bitmap
import android.provider.MediaStore
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.test.*
import androidx.activity.ComponentActivity
import androidx.lifecycle.Lifecycle
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.util.Locale

@SdkSuppress(minSdkVersion=29)
class DevicePhotosScreenTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun optInDeviceFoldersViewerAndReturnStayLocalAndCanBeDisabled() = screen(Locale.GERMAN)
    @Test fun englishDeviceFoldersAndSettingUseEnglishLabels() = screen(Locale.ENGLISH)

    private fun screen(locale: Locale) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val app = instrumentation.targetContext
        val context = app.createConfigurationContext(Configuration(app.resources.configuration).apply { setLocale(locale) })
        val preferences = DevicePhotoPreferences(app)
        val previous = preferences.enabled
        preferences.enabled = false
        devicePhotoPermissions().forEach { instrumentation.uiAutomation.grantRuntimePermission(app.packageName, it) }
        val german = locale == Locale.GERMAN
        val folders = if(german) "Ordner" else "Folders"
        val device = if(german) "Dieses Gerät" else "This device"
        val setting = if(german) "Lokale Fotoordner anzeigen" else "Show local photo folders"
        val settings = if(german) "Einstellungen" else "Settings"
        val close = if(german) "Schließen" else "Close"
        val back = if(german) "Zurück" else "Back"
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, "bearstack-device-photo.jpg")
            put(MediaStore.Images.Media.MIME_TYPE, "image/jpeg")
            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/BearStackDeviceTest")
            put(MediaStore.Images.Media.IS_PENDING, 1)
        }
        val uri = app.contentResolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values)!!
        val bitmap = Bitmap.createBitmap(400, 300, Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.GREEN) }
        app.contentResolver.openOutputStream(uri)!!.use { bitmap.compress(Bitmap.CompressFormat.JPEG, 90, it) }
        bitmap.recycle()
        app.contentResolver.update(uri, ContentValues().apply { put(MediaStore.Images.Media.IS_PENDING, 0) }, null, null)
        val images = ImageLoader.Builder(app).build()
        val owner = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        var serverCalls = 0
        var lastQuery = ""
        val api = object : PhotosService {
            override suspend fun session() = DevicePhotosService.SESSION
            override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
                serverCalls++
                lastQuery = query.query
                return PhotoPage(query.path, "", page, 0, false, 1, false, false, emptyList(),
                    listOf(PhotoFolder("server", "Server folder", null, 0, false, 0, emptyList())), emptyList())
            }
            override suspend fun info(path: String): Photo = error("Local information reached server")
            override suspend fun blog(path: String): PhotoBlog = error("Local content reached server")
            override fun thumbnail(photo: Photo, size: Int): String = error("Local photo reached server")
            override fun original(photo: Photo): String = error("Local original reached server")
        }
        var removed = false
        lateinit var controller: PhotosController
        compose.runOnUiThread { controller = PhotosController(owner, api, DevicePhotosService.SESSION) }
        try {
            compose.setContent {
                val registry = checkNotNull(LocalActivityResultRegistryOwner.current)
                CompositionLocalProvider(LocalActivityResultRegistryOwner provides registry, LocalContext provides context, LocalResources provides context.resources,
                    LocalConfiguration provides context.resources.configuration) {
                    MaterialTheme { PhotosScreen(controller, images, false, {}, {}) }
                }
            }
            compose.onNodeWithText(folders).performClick()
            compose.onNodeWithText(device).assertDoesNotExist()
            compose.onNodeWithContentDescription(if(german) "Weitere Optionen" else "More options").performClick()
            compose.onNodeWithText(settings).performClick()
            compose.onNodeWithText(setting).assertIsOff().performClick().assertIsOn()
            assertTrue(DevicePhotoPreferences(app).enabled)
            compose.onNodeWithText(close).performClick()
            compose.onNodeWithText(device).assertIsDisplayed()
            compose.onNodeWithText(if(german) "Fotos" else "Photos").performClick()
            compose.onNodeWithText(device).assertDoesNotExist()
            compose.onNodeWithText(folders).performClick()
            compose.onNodeWithText(if(german) "Suchen" else "Search").performClick()
            compose.onNode(hasSetTextAction()).performTextInput("holiday")
            compose.onNode(hasSetTextAction()).performImeAction()
            compose.onNodeWithText(folders).performClick()
            compose.onNodeWithText(device).performClick()
            compose.onNodeWithText(if(german) "Suchen" else "Search").performClick()
            compose.onNode(hasSetTextAction()).assertTextContains("holiday")
            assertEquals("holiday", lastQuery)
            compose.onNodeWithText(folders).performClick()
            val callsBeforeDevice = serverCalls
            compose.onNodeWithText(device).performClick()
            compose.waitUntil(15_000) { compose.onAllNodesWithText("BearStackDeviceTest").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithText("BearStackDeviceTest").performClick()
            compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("bearstack-device-photo.jpg").fetchSemanticsNodes().isNotEmpty() }
            compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
            compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
            compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("bearstack-device-photo.jpg").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithContentDescription("bearstack-device-photo.jpg").performClick()
            compose.onNodeWithContentDescription(if(german) "Teilen" else "Share").assertIsDisplayed()
            // The common viewer presents its own zoom action after decoding the local content URI.
            compose.waitUntil(15_000) { compose.onAllNodesWithText(if(german) "Vergrößern" else "Zoom in").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithContentDescription(if(german) "Informationen" else "Information").performClick()
            compose.waitUntil(10_000) { compose.onAllNodesWithText("bearstack-device-photo.jpg").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithText("bearstack-device-photo.jpg").assertIsDisplayed()
            androidx.test.espresso.Espresso.pressBack()
            compose.onNodeWithContentDescription(close).performClick()
            compose.onNodeWithContentDescription(back).performClick()
            compose.waitUntil(10_000) { compose.onAllNodesWithText("BearStackDeviceTest").fetchSemanticsNodes().isNotEmpty() }
            app.contentResolver.delete(uri, null, null)
            removed = true
            compose.waitUntil(15_000) { compose.onAllNodesWithText("BearStackDeviceTest").fetchSemanticsNodes().isEmpty() }
            compose.onNodeWithContentDescription(back).performClick()
            compose.onNodeWithText("Server folder").assertIsDisplayed()
            assertEquals(callsBeforeDevice, serverCalls)
            compose.onNodeWithContentDescription(if(german) "Weitere Optionen" else "More options").performClick()
            compose.onNodeWithText(settings).performClick()
            compose.onNodeWithText(setting).performClick().assertIsOff()
            compose.onNodeWithText(close).performClick()
            compose.onNodeWithText(device).assertDoesNotExist()
            assertFalse(DevicePhotoPreferences(app).enabled)
        } finally {
            compose.runOnUiThread { controller.close(); owner.cancel(); images.shutdown() }
            if(!removed) app.contentResolver.delete(uri, null, null)
            preferences.enabled = previous
        }
    }
}
