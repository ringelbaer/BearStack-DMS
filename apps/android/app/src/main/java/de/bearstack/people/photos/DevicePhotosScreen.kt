package de.bearstack.people.photos

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.database.ContentObserver
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.ImageLoader
import coil.memory.MemoryCache
import de.bearstack.people.R
import de.bearstack.people.data.remote.PhotoQuery
import de.bearstack.people.text.uiStrings

internal enum class DevicePhotoAccess { NONE, SELECTED, FULL }
// The explicit SDK argument keeps permission decisions testable across Android versions.
@SuppressLint("InlinedApi")
internal fun devicePhotoPermissions(sdk: Int = Build.VERSION.SDK_INT): Array<String> = when {
    sdk >= 34 -> arrayOf(Manifest.permission.READ_MEDIA_IMAGES, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)
    sdk >= 33 -> arrayOf(Manifest.permission.READ_MEDIA_IMAGES)
    else -> arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE)
}
@SuppressLint("InlinedApi")
internal fun devicePhotoAccess(sdk: Int, granted: (String) -> Boolean): DevicePhotoAccess = when {
    granted(if(sdk >= 33) Manifest.permission.READ_MEDIA_IMAGES else Manifest.permission.READ_EXTERNAL_STORAGE) -> DevicePhotoAccess.FULL
    sdk >= 34 && granted(Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED) -> DevicePhotoAccess.SELECTED
    else -> DevicePhotoAccess.NONE
}
internal fun devicePhotoAccess(context: Context) = devicePhotoAccess(Build.VERSION.SDK_INT) {
    ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED
}
internal class DevicePhotoPreferences(context: Context) {
    private val store = context.applicationContext.getSharedPreferences("device_photos", Context.MODE_PRIVATE)
    var enabled: Boolean
        get() = store.getBoolean("enabled", false)
        set(value) { store.edit().putBoolean("enabled", value).apply() }
}

@Composable
fun PhotosScreen(controller: PhotosController, images: ImageLoader, canManage: Boolean,
    onPeople: () -> Unit, onConnection: () -> Unit) {
    val context = LocalContext.current
    val preferences = remember(context) { DevicePhotoPreferences(context) }
    var enabled by remember { mutableStateOf(preferences.enabled) }
    var deviceOpen by rememberSaveable { mutableStateOf(false) }
    var settingsOpen by rememberSaveable { mutableStateOf(false) }
    var search by rememberSaveable { mutableStateOf(controller.state.value.query.query) }
    var access by remember { mutableStateOf(devicePhotoAccess(context)) }
    var revision by remember { mutableIntStateOf(0) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    var foreground by remember(lifecycle) { mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) }
    val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        access = devicePhotoAccess(context)
        revision++
    }
    DisposableEffect(lifecycle, context) {
        val observer = LifecycleEventObserver { _, event ->
            if(event == Lifecycle.Event.ON_RESUME) {
                access = devicePhotoAccess(context)
                revision++
                foreground = true
            } else if(event == Lifecycle.Event.ON_PAUSE) foreground = false
        }
        lifecycle.addObserver(observer)
        onDispose { lifecycle.removeObserver(observer) }
    }
    val serverState = rememberSaveableStateHolder()
    if(deviceOpen && enabled) {
        DevicePhotosScreen(access, foreground, revision, onAccess={ permission.launch(devicePhotoPermissions()) },
            onSettings={ settingsOpen = true }, onLeave={ tab ->
                deviceOpen = false
                if(tab != 1) controller.open(PhotoQuery(query=if(tab == 2) search.trim() else "", recursive=true), tab=tab)
            })
    } else serverState.SaveableStateProvider("server") {
        ServerPhotosScreen(controller, images, canManage, onPeople, onConnection, search, { search = it },
            onSettings={ settingsOpen = true }, onDevice=if(enabled) ({ deviceOpen = true }) else null)
    }
    if(settingsOpen) AlertDialog(onDismissRequest={ settingsOpen = false },
        title={ Text(stringResource(R.string.photos_settings)) },
        text={ Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Row(Modifier.toggleable(value=enabled, role=Role.Switch, onValueChange={ value ->
                    preferences.enabled = value
                    enabled = value
                    if(!value) deviceOpen = false
                    else if(access == DevicePhotoAccess.NONE) permission.launch(devicePhotoPermissions())
                }), verticalAlignment=Alignment.CenterVertically) {
                Text(stringResource(R.string.photos_device_enable), Modifier.weight(1f))
                Switch(checked=enabled, onCheckedChange=null, modifier=Modifier.padding(start=12.dp))
            }
            Text(stringResource(R.string.photos_device_setting_help))
            if(enabled) DeviceAccessControls(access) { permission.launch(devicePhotoPermissions()) }
        } }, confirmButton={ TextButton(onClick={ settingsOpen = false }) { Text(stringResource(R.string.photos_close)) } })
}

@Composable
private fun DeviceAccessControls(access: DevicePhotoAccess, onAccess: () -> Unit) {
    val context = LocalContext.current
    if(access != DevicePhotoAccess.FULL) {
        Text(stringResource(if(access == DevicePhotoAccess.SELECTED) R.string.photos_device_selected else R.string.photos_device_permission))
        TextButton(onClick=onAccess) { Text(stringResource(R.string.photos_device_grant)) }
    }
    TextButton(onClick={ context.startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
        Uri.fromParts("package", context.packageName, null)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }) {
        Text(stringResource(R.string.photos_device_android_settings))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DevicePhotosScreen(access: DevicePhotoAccess, foreground: Boolean, revision: Int,
    onAccess: () -> Unit, onSettings: () -> Unit, onLeave: (Int) -> Unit) {
    var path by rememberSaveable { mutableStateOf("") }
    var title by rememberSaveable { mutableStateOf("") }
    var changes by remember { mutableIntStateOf(0) }
    val context = LocalContext.current
    // Debounce bursts (camera writes, imports). The observer exists only while this
    // catalog is open and foregrounded; a new foreground always creates a fresh catalog.
    DisposableEffect(context, foreground, access) {
        val handler = Handler(Looper.getMainLooper())
        val update = Runnable { changes++ }
        val observer = object : ContentObserver(handler) {
            override fun onChange(selfChange: Boolean) {
                handler.removeCallbacks(update)
                handler.postDelayed(update, 500)
            }
        }
        val observing = foreground && access != DevicePhotoAccess.NONE
        if(observing) context.contentResolver.registerContentObserver(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, true, observer)
        onDispose {
            if(observing) context.contentResolver.unregisterContentObserver(observer)
            handler.removeCallbacks(update)
        }
    }
    val scope = rememberCoroutineScope()
    val active = foreground && access != DevicePhotoAccess.NONE
    val local = remember(active, revision, changes) {
        if(active) PhotosController(scope, DevicePhotosService(context.contentResolver), DevicePhotosService.SESSION,
            initialQuery=PhotoQuery(path=path)) else null
    }
    val images = remember(local) {
        local?.let { ImageLoader.Builder(context).diskCache(null)
            .memoryCache { MemoryCache.Builder(context).maxSizeBytes(16 * 1024 * 1024).build() }.build() }
    }
    DisposableEffect(local, images) { onDispose { local?.close(); images?.memoryCache?.clear(); images?.shutdown() } }
    val state = local?.state?.collectAsStateWithLifecycle()?.value
    LaunchedEffect(state?.query?.path, state?.loading) {
        if(state != null) {
            path = state.query.path
            title = (local.service as DevicePhotosService).folderName(path).orEmpty()
        }
    }
    fun back() { if(path.isEmpty()) onLeave(1) else { path=""; title=""; local?.open(PhotoQuery()) } }
    BackHandler(state?.selected == null) { back() }
    Scaffold(topBar={ TopAppBar(title={ Column {
        Text(if(path.isEmpty()) stringResource(R.string.photos_device) else title.ifBlank { stringResource(R.string.photos_folders) },
            maxLines=1, overflow=TextOverflow.Ellipsis)
        if(path.isNotEmpty()) Text(stringResource(R.string.photos_device), style=MaterialTheme.typography.labelSmall)
    } }, navigationIcon={ IconButton(onClick={ back() }) {
        Icon(painterResource(R.drawable.ic_back), stringResource(R.string.photos_back))
    } }, actions={ IconButton(onClick=onSettings) {
        Icon(painterResource(R.drawable.ic_more_horiz), stringResource(R.string.photos_settings))
    } }) }, bottomBar={ NavigationBar {
        listOf(R.string.photos_title to R.drawable.ic_photos, R.string.photos_folders to R.drawable.ic_folder,
            R.string.photos_search to R.drawable.ic_search).forEachIndexed { index, (label, icon) ->
            NavigationBarItem(selected=index == 1, onClick={ onLeave(index) },
                icon={ Icon(painterResource(icon), null) }, label={ Text(stringResource(label)) })
        }
    } }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            if(access == DevicePhotoAccess.NONE) Column(Modifier.padding(24.dp)) { DeviceAccessControls(access, onAccess) }
            else if(local != null && images != null && state != null) key(local) {
                if(access == DevicePhotoAccess.SELECTED) Text(stringResource(R.string.photos_device_selected),
                    Modifier.fillMaxWidth().clickable(onClick=onAccess).padding(12.dp), style=MaterialTheme.typography.bodySmall)
                Box(Modifier.fillMaxWidth().height(3.dp)) {
                    if(state.loading || state.loadingSections.isNotEmpty()) LinearProgressIndicator(Modifier.fillMaxSize())
                }
                state.error?.let { error ->
                    Row(Modifier.padding(12.dp), verticalAlignment=Alignment.CenterVertically) {
                        Text(uiStrings()(error), Modifier.weight(1f))
                        TextButton(onClick={ changes++ }) { Text(stringResource(R.string.photos_retry)) }
                    }
                }
                PhotoGallery(local, images, state)
                state.selected?.let { PhotoViewer(local, images, state.media, it) }
            }
        }
    }
}
