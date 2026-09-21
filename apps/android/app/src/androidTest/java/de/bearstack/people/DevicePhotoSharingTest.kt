package de.bearstack.people

import android.content.ContentValues
import android.graphics.Bitmap
import android.net.Uri
import android.provider.MediaStore
import android.view.KeyEvent
import androidx.activity.ComponentActivity
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.Lifecycle
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.photos.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.ByteArrayOutputStream

@SdkSuppress(minSdkVersion=29)
class DevicePhotoSharingTest {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()
    @Test fun localFastScrollerJumpsAcrossMetadataPages() = photos { _ ->
        compose.onNodeWithTag("photo-gallery").performTouchInput {swipeUp()}
        compose.onNodeWithTag("gallery-fast-scroll").performSemanticsAction(androidx.compose.ui.semantics.SemanticsActions.SetProgress) {it(.8f)}
        compose.waitUntil(10_000) {compose.onAllNodesWithContentDescription("share-32.jpg").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("share-32.jpg").performClick()
        compose.onNodeWithText("128 von 160").assertIsDisplayed()
    }
    @SdkSuppress(minSdkVersion=30)
    @Test fun localMultiSelectionRequiresConfirmationAndSystemApproval() = photos { uris ->
        compose.onNodeWithContentDescription("share-159.jpg").performTouchInput {longClick()}
        compose.onNodeWithContentDescription("share-158.jpg").performClick()
        compose.onNodeWithText("2 / 100 ausgewählt").assertIsDisplayed()
        compose.onNodeWithContentDescription("Auswahl speichern").assertDoesNotExist()
        compose.onNodeWithContentDescription("Löschen").performClick()
        compose.onNodeWithText("Abbrechen").performClick()
        compose.onNodeWithText("2 / 100 ausgewählt").assertIsDisplayed()
        compose.onNodeWithContentDescription("Löschen").performClick()
        compose.onNode(hasText("Löschen") and hasClickAction()).performClick()
        val automation=InstrumentationRegistry.getInstrumentation().uiAutomation
        compose.waitUntil(10_000) {
            automation.rootInActiveWindow?.packageName?.toString()?.startsWith("com.android.providers.media")==true
        }
        // Declining Android's request must preserve both files and the selection.
        val deny=listOf("Don't allow", "Cancel", "Deny").flatMap {
            automation.rootInActiveWindow.findAccessibilityNodeInfosByText(it)
        }.first {it.isClickable}
        assertTrue(deny.performAction(android.view.accessibility.AccessibilityNodeInfo.ACTION_CLICK))
        compose.waitUntil(10_000) {automation.rootInActiveWindow?.packageName?.toString()==compose.activity.packageName}
        compose.waitUntil(10_000) {compose.onAllNodesWithText("2 / 100 ausgewählt").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("Löschen").performClick()
        compose.onNode(hasText("Löschen") and hasClickAction()).performClick()
        compose.waitUntil(10_000) {
            listOf("Delete", "Allow").any {text -> automation.rootInActiveWindow?.findAccessibilityNodeInfosByText(text)?.any {it.isClickable}==true}
        }
        val button=listOf("Delete", "Allow").flatMap {automation.rootInActiveWindow.findAccessibilityNodeInfosByText(it)}.first {it.isClickable}
        assertTrue(button.performAction(android.view.accessibility.AccessibilityNodeInfo.ACTION_CLICK))
        compose.waitUntil(10_000) {automation.rootInActiveWindow?.packageName?.toString()==compose.activity.packageName}
        compose.waitUntil(15_000) {compose.onAllNodesWithContentDescription("share-157.jpg").fetchSemanticsNodes().isNotEmpty() &&
            compose.onAllNodesWithContentDescription("share-159.jpg").fetchSemanticsNodes().isEmpty()}
        uris.removeAt(uris.lastIndex);uris.removeAt(uris.lastIndex)
        compose.onNodeWithContentDescription("share-158.jpg").assertDoesNotExist()
    }

    @Test fun systemChooserAndBackgroundReturnKeepTheCurrentPhotoAndScrolledGallery() = photos { uris ->
        // Cross a metadata page boundary before sharing; returning to page one
        // must not silently discard the selected photo or the grid's viewport.
        val gallery = compose.onNodeWithTag("photo-gallery")
        gallery.performScrollToKey("photo:${uris[64]}")
        compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("share-63.jpg").fetchSemanticsNodes().isNotEmpty() }
        gallery.performScrollToKey("photo:${uris[30]}")
        val thumbnail = compose.onNodeWithContentDescription("share-30.jpg")
        thumbnail.assertIsDisplayed()
        val top = thumbnail.fetchSemanticsNode().boundsInRoot.top
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        repeat(2) {
            thumbnail.performClick()
            compose.onNodeWithText("130 von 160").assertIsDisplayed()
            compose.onNodeWithContentDescription("Teilen").performClick()
            compose.waitUntil(10_000) {
                instrumentation.uiAutomation.rootInActiveWindow?.packageName?.toString() in
                    setOf("android", "com.android.intentresolver")
            }
            instrumentation.sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)
            compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("Teilen").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithText("130 von 160").assertIsDisplayed()
            // A receiving app can stop the activity completely, unlike a chooser
            // overlay. Exercise that lifecycle while the viewer remains open.
            backgroundAndResume()
            compose.onNodeWithText("130 von 160").assertIsDisplayed()
            compose.onNodeWithContentDescription("Schließen").performClick()
            thumbnail.assertIsDisplayed()
            assertEquals(top, thumbnail.fetchSemanticsNode().boundsInRoot.top, 1f)
            backgroundAndResume()
            thumbnail.assertIsDisplayed()
            assertEquals(top, thumbnail.fetchSemanticsNode().boundsInRoot.top, 1f)
        }
    }

    @Test fun mediaRemovedWhileStoppedStillInvalidatesTheOpenViewer() = photos { uris ->
        compose.onNodeWithContentDescription("share-159.jpg").performClick()
        compose.onNodeWithContentDescription("Teilen").assertIsDisplayed()
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.waitForIdle()
        InstrumentationRegistry.getInstrumentation().targetContext.contentResolver.delete(uris.last(), null, null)
        uris.removeAt(uris.lastIndex)
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("Teilen").fetchSemanticsNodes().isEmpty() }
        compose.onNodeWithTag("photo-viewer-image").assertDoesNotExist()
        compose.onNodeWithContentDescription("share-159.jpg").assertDoesNotExist()
        compose.onNodeWithContentDescription("share-158.jpg").assertIsDisplayed()
    }

    private fun backgroundAndResume() {
        compose.activityRule.scenario.moveToState(Lifecycle.State.CREATED)
        compose.waitForIdle()
        compose.activityRule.scenario.moveToState(Lifecycle.State.RESUMED)
        compose.waitForIdle()
    }

    private fun photos(test: (MutableList<Uri>) -> Unit) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        devicePhotoPermissions().forEach { instrumentation.uiAutomation.grantRuntimePermission(context.packageName, it) }
        val resolver = context.contentResolver
        val uris = mutableListOf<Uri>()
        val bitmap = Bitmap.createBitmap(32, 32, Bitmap.Config.ARGB_8888).apply { eraseColor(android.graphics.Color.GREEN) }
        val bytes = ByteArrayOutputStream().use { bitmap.compress(Bitmap.CompressFormat.JPEG, 90, it); it.toByteArray() }
        bitmap.recycle()
        try {
            repeat(160) { index ->
                val uri = checkNotNull(resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, ContentValues().apply {
                    put(MediaStore.Images.Media.DISPLAY_NAME, "share-$index.jpg")
                    put(MediaStore.Images.Media.MIME_TYPE, "image/jpeg")
                    put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/BearStackShareReturnTest")
                    put(MediaStore.Images.Media.IS_PENDING, 1)
                }))
                uris += uri
                resolver.openOutputStream(uri)!!.use { it.write(bytes) }
                resolver.update(uri, ContentValues().apply { put(MediaStore.Images.Media.IS_PENDING, 0) }, null, null)
            }
            compose.setGermanContent {
                MaterialTheme { PhotosScreen(null, null, false, {}, {}) }
            }
            compose.waitUntil(15_000) { compose.onAllNodesWithText("BearStackShareReturnTest").fetchSemanticsNodes().isNotEmpty() }
            compose.onNodeWithText("BearStackShareReturnTest").performClick()
            compose.waitUntil(15_000) { compose.onAllNodesWithContentDescription("share-159.jpg").fetchSemanticsNodes().isNotEmpty() }
            test(uris)
        } finally {
            compose.runOnUiThread { compose.activity.setContentView(android.widget.FrameLayout(compose.activity)) }
            uris.forEach { resolver.delete(it, null, null) }
        }
    }
}
