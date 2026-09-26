package de.bearstack.people.photos

import android.content.Intent
import androidx.activity.compose.BackHandler
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
import de.bearstack.people.text.*
import kotlinx.coroutines.*

internal const val MAX_PHOTO_SELECTION = 100

@Composable internal fun PhotoSelectionActions(controller: PhotosController, state: PhotosState) {
    BackHandler(state.selecting) { controller.clearSelection() }
    val photos = state.selection.values.toList()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val sharing = remember(controller, context) { PhotoSharing(context, controller.service) }
    val chooser = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) {}
    var pendingSave by remember(controller) { mutableStateOf<List<Photo>>(emptyList()) }
    val folder = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        val selected = pendingSave
        pendingSave = emptyList()
        if(uri != null && selected.isNotEmpty()) controller.downloads?.saveAll(selected, uri)
    }
    var task by remember(controller) { mutableStateOf<Job?>(null) }
    var busy by remember(controller) { mutableStateOf(false) }
    var error by remember(controller) { mutableStateOf<UiText?>(null) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, controller) {
        val observer = LifecycleEventObserver { _, event -> if(event == Lifecycle.Event.ON_STOP) task?.cancel() }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer); task?.cancel() }
    }
    if(state.selecting) Surface(color=MaterialTheme.colorScheme.secondaryContainer) {
        Column {
            Row(Modifier.fillMaxWidth().padding(horizontal=4.dp)) {
                IconButton(onClick=controller::clearSelection) {
                    Icon(painterResource(R.drawable.ic_back), stringResource(R.string.photos_selection_close))
                }
                Text(stringResource(R.string.photos_selection_count, photos.size, MAX_PHOTO_SELECTION),
                    Modifier.weight(1f).padding(vertical=14.dp), style=MaterialTheme.typography.titleSmall)
                IconButton(enabled=photos.isNotEmpty() && !busy, onClick={
                    val selected = photos.toList()
                    busy = true; error = null
                    task = scope.launch {
                        try { sharing.share(selected, { chooser.launch(it) }) }
                        catch(e: CancellationException) { throw e }
                        catch(e: Exception) { error = (e as? DescribedFailure)?.userText ?: UiText(R.string.photos_share_error) }
                        finally { busy = false }
                    }
                }) { Icon(painterResource(R.drawable.ic_share), stringResource(R.string.photos_share)) }
                controller.downloads?.let { downloads ->
                    val status by downloads.state.collectAsState()
                    IconButton(enabled=photos.isNotEmpty() && !status.active, onClick={pendingSave=photos.toList();folder.launch(null)}) {
                        Icon(painterResource(R.drawable.ic_download), stringResource(R.string.photos_selection_save))
                    }
                }
                if(controller.session.imageGroups && controller.session.canManagePeople && controller.service !is DevicePhotosService)
                    PhotoGroupSelectionAction(controller,photos)
                if(controller.service is DevicePhotosService)
                    DevicePhotoDeleteAction(controller, photos, controller::clearSelection)
            }
        }
    }
    // Downloads continue when selection mode is dismissed.
    controller.downloads?.let { PhotoDownloadStatus(it) }
    if(busy || error != null) AlertDialog(onDismissRequest={task?.cancel();error=null},
        title={Text(stringResource(R.string.photos_share))},
        text={if(busy) CircularProgressIndicator() else error?.let {Text(uiStrings()(it))}},
        confirmButton={TextButton(onClick={task?.cancel();error=null}) {Text(stringResource(if(busy) R.string.photos_cancel else R.string.photos_close))}})
}
