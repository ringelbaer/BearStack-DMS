package de.bearstack.people.photos

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import de.bearstack.people.R

/** One settings surface for both the gallery and people workflows. */
@Composable internal fun AppSettingsDialog(controller: PhotosController?, connected: Boolean,
    onConnection: () -> Unit, onDismiss: () -> Unit, connectionEnabled: Boolean = true,
    onDeviceChanged: (Boolean) -> Unit = {}) {
    val context=LocalContext.current
    val preferences=remember(context) { DevicePhotoPreferences(context) }
    var enabled by remember { mutableStateOf(preferences.enabled) }
    var access by remember { mutableStateOf(devicePhotoAccess(context)) }
    val lifecycle=LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle,context) {
        val observer=LifecycleEventObserver {_,event ->
            if(event==Lifecycle.Event.ON_RESUME) access=devicePhotoAccess(context)
        }
        lifecycle.addObserver(observer)
        onDispose {lifecycle.removeObserver(observer)}
    }
    var confirmDisconnect by rememberSaveable { mutableStateOf(false) }
    val permission=rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        access=devicePhotoAccess(context)
        onDeviceChanged(enabled)
    }
    AlertDialog(onDismissRequest=onDismiss, title={Text(stringResource(R.string.photos_settings))},
        text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(stringResource(R.string.connection_title),style=MaterialTheme.typography.titleSmall)
            Text(stringResource(if(connected) R.string.connection_disconnect_help else R.string.connection_local_help))
            TextButton(enabled=connectionEnabled,onClick={
                if(connected) confirmDisconnect=true else {onDismiss();onConnection()}
            }) {Text(stringResource(if(connected) R.string.connection_disconnect else R.string.connection_setup))}
            HorizontalDivider()
            Row(Modifier.toggleable(value=enabled,role=Role.Switch,onValueChange={value ->
                preferences.enabled=value
                enabled=value
                onDeviceChanged(value)
                if(value && access==DevicePhotoAccess.NONE) permission.launch(devicePhotoPermissions())
            }),verticalAlignment=Alignment.CenterVertically) {
                Text(stringResource(R.string.photos_device_enable),Modifier.weight(1f))
                Switch(checked=enabled,onCheckedChange=null,modifier=Modifier.padding(start=12.dp))
            }
            Text(stringResource(R.string.photos_device_setting_help))
            if(enabled) DeviceAccessControls(access) {permission.launch(devicePhotoPermissions())}
            HorizontalDivider()
            ThumbnailCacheSettings(controller?.thumbnailCache,unavailable=controller!=null && controller.thumbnailCache==null)
        }},confirmButton={TextButton(onClick=onDismiss) {Text(stringResource(R.string.photos_close))}})
    if(confirmDisconnect) AlertDialog(onDismissRequest={confirmDisconnect=false},
        title={Text(stringResource(R.string.connection_disconnect_title))},
        text={Text(stringResource(R.string.connection_disconnect_help),Modifier.verticalScroll(rememberScrollState()))},
        confirmButton={TextButton(enabled=connectionEnabled,onClick={confirmDisconnect=false;onDismiss();onConnection()}) {
            Text(stringResource(R.string.connection_disconnect_confirm))
        }},dismissButton={TextButton(onClick={confirmDisconnect=false}) {Text(stringResource(R.string.photos_cancel))}})
}
