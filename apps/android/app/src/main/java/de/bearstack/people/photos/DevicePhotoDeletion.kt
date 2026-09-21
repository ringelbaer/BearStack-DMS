package de.bearstack.people.photos

import android.Manifest
import android.app.Activity
import android.app.RecoverableSecurityException
import android.content.Context
import android.content.IntentSender
import android.content.pm.PackageManager
import android.os.Build
import android.provider.MediaStore
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.IntentSenderRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo
import kotlinx.coroutines.*

// Deliberately separate from PhotosService: server services have no deletion API.
internal suspend fun deleteDevicePhotos(context: Context, service: DevicePhotosService, photos: List<Photo>,
    consent: suspend (IntentSender) -> Boolean): Boolean {
    require(photos.isNotEmpty() && photos.size <= MAX_PHOTO_SELECTION)
    val uris = photos.distinctBy {it.path}.map { devicePhotoUri(it.path) }
    // Check current access before opening a system request or touching a file.
    photos.forEach { service.info(it.path) }
    val resolver = context.contentResolver
    if(Build.VERSION.SDK_INT >= 30) {
        val request = withContext(Dispatchers.IO) { MediaStore.createDeleteRequest(resolver, uris) }
        return consent(request.intentSender) // Android performs the confirmed deletion.
    }
    for(uri in uris) {
        currentCoroutineContext().ensureActive()
        try { withContext(Dispatchers.IO) { resolver.delete(uri, null, null) } }
        catch(e: SecurityException) {
            if(Build.VERSION.SDK_INT == 29 && e is RecoverableSecurityException) {
                if(!consent(e.userAction.actionIntent.intentSender)) return false
                withContext(Dispatchers.IO) { resolver.delete(uri, null, null) }
            } else throw e
        }
    }
    return true
}

@Composable internal fun DevicePhotoDeleteAction(controller: PhotosController, photos: List<Photo>, onDeleted: () -> Unit) {
    val service = controller.service as? DevicePhotosService ?: return
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var confirm by remember(controller) { mutableStateOf<List<Photo>?>(null) }
    var busy by remember(controller) { mutableStateOf(false) }
    var error by remember(controller) { mutableStateOf(false) }
    var consent by remember(controller) { mutableStateOf<CompletableDeferred<Boolean>?>(null) }
    var permission by remember(controller) { mutableStateOf<CompletableDeferred<Boolean>?>(null) }
    val deleted by rememberUpdatedState(onDeleted)
    val system = rememberLauncherForActivityResult(ActivityResultContracts.StartIntentSenderForResult()) {
        consent?.complete(it.resultCode == Activity.RESULT_OK)
    }
    val writePermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        permission?.complete(it)
    }
    IconButton(enabled=photos.isNotEmpty() && !busy, onClick={ confirm=photos.toList() }) {
        Icon(painterResource(R.drawable.ic_delete), stringResource(R.string.photos_delete))
    }
    confirm?.let { selected -> AlertDialog(onDismissRequest={confirm=null},
        title={Text(stringResource(R.string.photos_delete))},
        text={Text(pluralStringResource(R.plurals.photos_delete_confirm,selected.size,selected.size))},
        dismissButton={TextButton(onClick={confirm=null}) {Text(stringResource(R.string.photos_cancel))}},
        confirmButton={TextButton(onClick={
            confirm=null;busy=true;error=false;controller.localMutation(true)
            scope.launch {
                try {
                    if(Build.VERSION.SDK_INT <= 28 && ContextCompat.checkSelfPermission(context,
                            Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
                        val answer = CompletableDeferred<Boolean>().also { permission=it }
                        writePermission.launch(Manifest.permission.WRITE_EXTERNAL_STORAGE)
                        if(!answer.await()) return@launch
                    }
                    if(deleteDevicePhotos(context, service, selected) { sender ->
                        val answer = CompletableDeferred<Boolean>().also {consent=it}
                        system.launch(IntentSenderRequest.Builder(sender).build())
                        answer.await()
                    }) deleted()
                } catch(e: CancellationException) { throw e }
                catch(_: Exception) { error=true }
                finally {busy=false;consent=null;permission=null;controller.localMutation(false)}
            }
        }) {Text(stringResource(R.string.photos_delete))}})
    }
    if(busy) AlertDialog(onDismissRequest={},title={Text(stringResource(R.string.photos_delete))},
        text={CircularProgressIndicator()},confirmButton={})
    if(error) AlertDialog(onDismissRequest={error=false},title={Text(stringResource(R.string.photos_delete))},
        text={Text(stringResource(R.string.photos_delete_error))},
        confirmButton={TextButton(onClick={error=false}) {Text(stringResource(R.string.photos_close))}})
}
