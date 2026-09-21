package de.bearstack.people

import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.media.ThumbnailPreferences
import de.bearstack.people.photos.ThumbnailCacheSettings
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test

class ThumbnailCacheSettingsTest {
    @get:Rule val compose = createComposeRule()

    @Test fun changingBudgetPersistsAndClampsToSupportedRange() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val preferences = ThumbnailPreferences(context)
        val original = preferences.sizeMiB
        try {
            preferences.sizeMiB = 256
            compose.setGermanContent { MaterialTheme { ThumbnailCacheSettings(null) } }
            compose.onNodeWithText("Speicherbudget: 256 MiB").assertExists()
            compose.onNodeWithTag("thumbnail-cache-size").performSemanticsAction(SemanticsActions.SetProgress) { it(512f) }
            compose.onNodeWithText("Speicherbudget: 512 MiB").assertExists()
            compose.runOnIdle { assertEquals(512, ThumbnailPreferences(context).sizeMiB) }
            preferences.sizeMiB = 0; assertEquals(64, ThumbnailPreferences(context).sizeMiB)
            preferences.sizeMiB = Int.MAX_VALUE; assertEquals(2048, ThumbnailPreferences(context).sizeMiB)
        } finally { preferences.sizeMiB = original }
    }
}
