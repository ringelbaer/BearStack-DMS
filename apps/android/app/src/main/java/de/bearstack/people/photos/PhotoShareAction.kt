package de.bearstack.people.photos

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotosService
import de.bearstack.people.text.*
import kotlinx.coroutines.*

@Composable internal fun PhotoShareAction(photo: Photo, service: PhotosService, onShare: () -> Unit,
    launch: ((Intent) -> Unit)? = null) {
    val context = LocalContext.current
    val chooser = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { }
    val sharing = remember(context, service) { PhotoSharing(context, service) }
    val scope = rememberCoroutineScope()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var task by remember(photo.path, service) { mutableStateOf<Job?>(null) }
    var active by remember(photo.path, service) { mutableStateOf(false) }
    var error by remember(photo.path, service) { mutableStateOf<UiText?>(null) }
    DisposableEffect(lifecycle, photo.path, service) {
        val observer = LifecycleEventObserver { _, event -> if(event == Lifecycle.Event.ON_STOP) task?.cancel() }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer); task?.cancel() }
    }
    IconButton(enabled = !active, onClick = {
        onShare()
        active = true
        error = null
        task = scope.launch {
            try { sharing.share(photo, launch ?: { chooser.launch(it) }) }
            catch(e: CancellationException) { throw e }
            catch(e: ActivityNotFoundException) { error = UiText(R.string.photos_share_unavailable) }
            catch(e: Exception) { error = if(e is DescribedFailure) e.userText else UiText(R.string.photos_share_error) }
            finally { active = false }
        }
    }) { Icon(painterResource(R.drawable.ic_share), stringResource(R.string.photos_share)) }
    if(active || error != null) AlertDialog(
        onDismissRequest = { task?.cancel(); error = null },
        title = { Text(stringResource(R.string.photos_share)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                if(active) {
                    Text(stringResource(R.string.photos_share_preparing, photo.name))
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                } else error?.let { Text(uiStrings()(it)) }
            }
        },
        confirmButton = {
            TextButton(onClick = { task?.cancel(); error = null }) {
                Text(stringResource(if(active) R.string.photos_cancel else R.string.photos_close))
            }
        }
    )
}
