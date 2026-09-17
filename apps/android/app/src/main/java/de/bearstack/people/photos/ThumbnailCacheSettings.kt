package de.bearstack.people.photos

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import de.bearstack.people.R
import de.bearstack.people.media.*
import kotlin.math.roundToInt

@Composable internal fun ThumbnailCacheSettings(cache: ThumbnailCache?, unavailable: Boolean = false) {
    val context = LocalContext.current
    val preferences = remember(context) { ThumbnailPreferences(context) }
    var size by remember { mutableIntStateOf(preferences.sizeMiB) }
    val state = cache?.state?.collectAsStateWithLifecycle()?.value
    Text(stringResource(R.string.photos_cache_title), style = MaterialTheme.typography.titleMedium)
    Text(stringResource(R.string.photos_cache_size, size))
    Slider(value = size.toFloat(), onValueChange = { size = (it / 64).roundToInt() * 64 },
        valueRange = 64f..2048f, steps = 30, modifier = Modifier.fillMaxWidth().testTag("thumbnail-cache-size"),
        onValueChangeFinished = { preferences.sizeMiB = size; cache?.resize(size) })
    Text(stringResource(R.string.photos_cache_help), style = MaterialTheme.typography.bodySmall)
    if (unavailable) Text(stringResource(R.string.photos_cache_unavailable), color = MaterialTheme.colorScheme.error)
    state?.let {
        val used = (it.usage.bytes + MIB - 1) / MIB
        val protected = (it.usage.pinnedBytes + MIB - 1) / MIB
        Text(stringResource(R.string.photos_cache_usage, used, protected, it.usage.pinnedEntries, it.usage.requiredEntries))
        if (it.usage.pinnedBytes > it.usage.budget)
            Text(stringResource(R.string.photos_cache_minimum, protected))
        if (it.syncing) {
            LinearProgressIndicator(Modifier.fillMaxWidth())
            Text(stringResource(R.string.photos_cache_loading))
        } else if (it.failed || it.usage.pinnedEntries < it.usage.requiredEntries) {
            Text(stringResource(R.string.photos_cache_incomplete), color = MaterialTheme.colorScheme.error)
            TextButton(onClick = { cache.refresh(force = true) }) { Text(stringResource(R.string.photos_retry)) }
        }
    }
}
