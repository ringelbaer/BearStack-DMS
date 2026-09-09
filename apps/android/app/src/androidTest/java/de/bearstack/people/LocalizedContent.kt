package de.bearstack.people

import android.content.res.Configuration
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.junit4.ComposeContentTestRule
import java.util.Locale

// Legacy interaction assertions intentionally verify the German UI. Scope its
// locale to this composition, independent of the device and other test cases.
fun ComposeContentTestRule.setGermanContent(content: @Composable () -> Unit) = setContent {
    val base = LocalContext.current
    val configuration = LocalConfiguration.current
    val context = remember(base, configuration) {
        base.createConfigurationContext(Configuration(configuration).apply { setLocale(Locale.GERMAN) })
    }
    CompositionLocalProvider(
        LocalContext provides context,
        LocalConfiguration provides context.resources.configuration,
        content = content,
    )
}
