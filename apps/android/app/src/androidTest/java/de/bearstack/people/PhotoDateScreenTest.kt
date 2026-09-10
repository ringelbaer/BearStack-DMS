package de.bearstack.people

import android.content.res.Configuration
import android.graphics.Bitmap
import androidx.activity.ComponentActivity
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.time.LocalDate
import java.util.Locale

class PhotoDateScreenTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun germanCalendarJumpsDeepAndKeepsContinuousScrolling() = screen(Locale.GERMAN)
    @Test fun englishCalendarJumpsDeepAndKeepsContinuousScrolling() = screen(Locale.ENGLISH)
    @Test fun backgroundCancelsTheDateJumpWithoutLosingTheGrid() = screen(Locale.ENGLISH, background = true)

    private fun screen(locale: Locale, background: Boolean = false) {
        val context = compose.activity
        val image = File(context.cacheDir, "date-test.jpg")
        val bitmap = Bitmap.createBitmap(32, 32, Bitmap.Config.ARGB_8888)
        image.outputStream().use { bitmap.compress(Bitmap.CompressFormat.JPEG, 80, it) }; bitmap.recycle()
        val images = ImageLoader.Builder(context).build()
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        val session = PhotoSession("date-test", false, 240, 240, 1280, 2048, 5, 8)
        val requests = mutableListOf<Int>()
        var chosen: String? = null
        val api = object : PhotosService {
            override suspend fun session() = session
            override suspend fun browse(query: PhotoQuery, page: Int, section: String): PhotoPage {
                requests += page
                val photos = List(96) { i -> Photo("photo-$page-$i", "photo-$page-$i", "image", "image/jpeg", "1",
                    LocalDate.of(2026, 6, 10).minusDays((page - 1).toLong()).toString()+"T12:00:00Z", null, 10, 32, 32) }
                return PhotoPage("", "", page, 96000, true, 0, false, false, photos, emptyList(), emptyList())
            }
            override suspend fun locateDate(date: String): PhotoDatePosition {
                chosen = date
                if(background) awaitCancellation()
                return PhotoDatePosition("photo-100-0", "2026-03-03", 100)
            }
            override suspend fun info(path: String): Photo = error("unused")
            override suspend fun blog(path: String): PhotoBlog = error("unused")
            override fun thumbnail(photo: Photo, size: Int) = image.toURI().toString()
            override fun original(photo: Photo) = image.toURI().toString()
        }
        lateinit var controller: PhotosController
        compose.runOnUiThread { controller = PhotosController(scope, api, session) }
        val german = locale == Locale.GERMAN
        val choose = if(german) "Datum auswählen" else "Choose date"
        val jump = if(german) "Zum Datum springen" else "Jump to date"
        val cancel = if(german) "Abbrechen" else "Cancel"
        try {
            compose.setContent {
                val config = Configuration(context.resources.configuration).apply { setLocale(locale) }
                val localized = context.createConfigurationContext(config)
                CompositionLocalProvider(LocalActivityResultRegistryOwner provides context, LocalContext provides localized,
                    LocalConfiguration provides config, LocalResources provides localized.resources) {
                    MaterialTheme { PhotosScreen(controller, images, false, {}, {}) }
                }
            }
            compose.waitUntil(10000) { !controller.state.value.loading }
            val calendarBounds = compose.onNodeWithContentDescription(choose).fetchSemanticsNode().boundsInRoot
            val menuBounds = compose.onNodeWithContentDescription(if(german) "Weitere Optionen" else "More options").fetchSemanticsNode().boundsInRoot
            assertTrue("calendar must precede menu", calendarBounds.right <= menuBounds.left)
            compose.onNodeWithContentDescription(choose).performClick()
            compose.onNodeWithText(cancel).performClick()
            assertNull(chosen)
            compose.onNodeWithContentDescription(choose).performClick()
            compose.onNode(isDialog()).captureToImage().asAndroidBitmap().let { screenshot ->
                File(context.cacheDir,"date-picker-${locale.language}.png").outputStream().use {
                    screenshot.compress(Bitmap.CompressFormat.PNG,100,it)
                }
            }
            compose.onNode(hasText("15", substring = true) and hasClickAction()).performClick()
            compose.onNodeWithText(jump).performClick()
            compose.waitUntil(10000) { chosen != null }
            assertEquals("2026-06-15", chosen)
            if(background) {
                compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
                compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
                compose.waitUntil(5000) { !controller.state.value.dateLoading }
                assertFalse(requests.contains(100))
                compose.onNodeWithContentDescription("photo-1-0").assertIsDisplayed()
            } else {
                compose.waitUntil(10000) { !controller.state.value.dateLoading && controller.state.value.scrollToKey == null }
                assertNull(controller.state.value.dateError)
                compose.onNodeWithContentDescription("photo-100-0").assertIsDisplayed()
                assertTrue(requests.contains(100))
                assertFalse(requests.any { it in 3..98 })
                compose.onNodeWithTag("photo-gallery").performScrollToNode(hasContentDescription("photo-100-95"))
                compose.waitUntil(10000) { requests.contains(101) }
                assertTrue(controller.state.value.media.size <= 288)
            }
        } finally {
            compose.runOnUiThread { controller.close(); scope.cancel(); images.shutdown() }
            image.delete()
        }
    }
}
