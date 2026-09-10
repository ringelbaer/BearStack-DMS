package de.bearstack.people

import android.content.Intent
import android.view.KeyEvent
import androidx.activity.ComponentActivity
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.*
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.photos.PhotoShareAction
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.util.Locale

class PhotoShareActionTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    @Test fun sharesTheCurrentlyDisplayedPhotoInGerman() = sharesCurrent(Locale.GERMAN)
    @Test fun sharesTheCurrentlyDisplayedPhotoInEnglish() = sharesCurrent(Locale.ENGLISH)
    private fun sharesCurrent(locale: Locale) {
        val service = ShareTestService()
        var photo by mutableStateOf(shareTestPhoto)
        var launched: Intent? = null
        var paused = false
        val filesBefore = shareFiles()
        try {
            content(locale) {
                PhotoShareAction(photo, service, onShare = { paused = true }, launch = { launched = it })
            }
            val label = if(locale == Locale.GERMAN) "Teilen" else "Share"
            compose.onNodeWithContentDescription(label).assertIsDisplayed()
            assertNull(service.selected)
            val second = shareTestPhoto.copy(path = "second.png", name = "second.png", mime = "image/png")
            compose.runOnUiThread { photo = second }
            compose.onNodeWithContentDescription(label).performClick()
            compose.waitUntil(5000) { launched != null }
            assertTrue(paused)
            assertEquals(second, service.selected)
        } finally { (shareFiles() - filesBefore).forEach { it.delete() } }
    }

    @Test fun cancellationAndBackgroundingNeverOpenChooser() {
        val service = ShareTestService().apply { mode = "cancel" }
        val filesBefore = shareFiles()
        content(Locale.ENGLISH) {
            PhotoShareAction(shareTestPhoto, service, onShare = {}, launch = { fail("cancelled share launched") })
        }
        compose.onNodeWithContentDescription("Share").performClick()
        compose.waitUntil(5000) { service.entered.isCompleted }
        compose.onNodeWithText("Preparing first.jpg for sharing …").assertIsDisplayed()
        compose.onNodeWithText("Cancel").performClick()
        compose.waitUntil(5000) { shareFiles() == filesBefore }
        compose.onNodeWithContentDescription("Share").assertIsEnabled().performClick()
        compose.waitUntil(5000) { shareFiles() != filesBefore }
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.waitUntil(5000) { shareFiles() == filesBefore }
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        compose.onNodeWithText("Preparing first.jpg for sharing …").assertDoesNotExist()
    }

    @Test fun errorsAreLocalizedAndRetryUsesANewTransfer() {
        val service = ShareTestService().apply { mode = "failure" }
        var launched = false
        val filesBefore = shareFiles()
        try {
            content(Locale.ENGLISH) {
                PhotoShareAction(shareTestPhoto, service, onShare = {}, launch = { launched = true })
            }
            compose.onNodeWithContentDescription("Share").performClick()
            val message = "The image could not be prepared for sharing. Please try again."
            compose.waitUntil(5000) { compose.onAllNodesWithText(message).fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithText(message).assertIsDisplayed()
            compose.onNodeWithText("secret server details", substring = true).assertDoesNotExist()
            compose.onNodeWithText("Close").performClick()
            compose.runOnUiThread { service.mode = "success" }
            compose.onNodeWithContentDescription("Share").performClick()
            compose.waitUntil(5000) { launched }
        } finally { (shareFiles() - filesBefore).forEach { it.delete() } }
    }

    @Test fun defaultActionOpensTheAndroidSystemChooser() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val service = ShareTestService()
        val filesBefore = shareFiles()
        try {
            content(Locale.ENGLISH) {
                PhotoShareAction(shareTestPhoto, service, onShare = {})
            }
            compose.onNodeWithContentDescription("Share").performClick()
            compose.waitUntil(10_000) {
                instrumentation.uiAutomation.rootInActiveWindow?.packageName?.toString() in setOf("android", "com.android.intentresolver")
            }
            assertEquals(1, (shareFiles() - filesBefore).size)
        } finally {
            instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
            (shareFiles() - filesBefore).forEach { it.delete() }
        }
    }

    private fun shareFiles() = java.io.File(compose.activity.cacheDir, "photo-sharing").listFiles()?.toSet().orEmpty()

    private fun content(locale: Locale, content: @Composable () -> Unit) = compose.setLocalizedContent(locale) {
        CompositionLocalProvider(LocalActivityResultRegistryOwner provides compose.activity) {
            MaterialTheme(content = content)
        }
    }
}
