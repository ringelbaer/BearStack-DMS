package de.bearstack.people.photos

import de.bearstack.people.text.*
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import coil.ImageLoader
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun FolderMap(controller: PhotosController, images: ImageLoader, query: PhotoQuery, tileImages: ImageLoader? = null, onClose: () -> Unit) {
    val text=uiStrings()
    var data by remember(query) {mutableStateOf<PhotoMapData?>(null)}
    var initial by remember(query) {mutableStateOf<PhotoMapBounds?>(null)}
    var error by remember {mutableStateOf<UiText?>(null)}
    var loading by remember {mutableStateOf(false)}
    var retry by remember {mutableIntStateOf(0)}
    var selected by remember {mutableStateOf<Photo?>(null)}
    var cluster by remember {mutableStateOf<PhotoMapMarker?>(null)}
    val scope=rememberCoroutineScope()
    val tracks=remember(controller,query) {MapTracks(scope,controller.service,if(query.query.isBlank()) query.path else "")}
    val photoRoute=remember(controller,query) {MapPhotoRoute(scope,controller.service,query)}
    val route by photoRoute.state.collectAsState()
    DisposableEffect(photoRoute) {onDispose {photoRoute.close()}}
    DisposableEffect(tracks) {onDispose {tracks.close()}}
    val layers by tracks.state.collectAsState()
    var layerSheet by remember {mutableStateOf(false)}
    LaunchedEffect(layers.focus,route.data?.geometry?.bounds) {
        if(initial==null) initial=layers.focus?.bounds ?: route.data?.geometry?.bounds
    }
    val unavailable=UiText(R.string.photos_map_index_pending)
    val loadError=UiText(R.string.photos_map_error)
    fun message(e: Exception)=if(e is ApiFailure && e.code=="map_index_not_ready") unavailable else loadError
    LaunchedEffect(query,retry) {
        loading=true;error=null
        try {
            val result=controller.service.map(query)
            data=result
            initial=result.bounds ?: tracks.state.value.focus?.bounds
        } catch(e: CancellationException) {throw e}
        catch(e: Exception) {error=message(e)}
        finally {loading=false}
    }
    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false)) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.safeDrawingPadding()) {
                TopAppBar(title={Text(stringResource(R.string.photos_map))},navigationIcon={
                    IconButton(onClick=onClose) {Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back))}
                },actions={TextButton(onClick={layerSheet=true}) {Text(stringResource(R.string.photos_map_layers))}})
                Text(stringResource(R.string.photos_map_help),Modifier.padding(horizontal=16.dp,vertical=8.dp),style=MaterialTheme.typography.bodySmall)
                if(loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                error?.let {
                    Row(Modifier.padding(horizontal=16.dp),horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                        Text(text(it),Modifier.weight(1f));TextButton(onClick={initial=null;retry++}) {Text(stringResource(R.string.photos_retry))}
                    }
                }
                if(layers.loading.isNotEmpty() || route.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                route.error?.let {
                    Row(Modifier.padding(horizontal=16.dp),verticalAlignment=androidx.compose.ui.Alignment.CenterVertically) {
                        Text(text(it),Modifier.weight(1f))
                        TextButton(onClick=photoRoute::refresh) {Text(stringResource(R.string.photos_retry))}
                    }
                }
                if(layers.errors.isNotEmpty()) {
                    Row(Modifier.padding(horizontal=16.dp),verticalAlignment=androidx.compose.ui.Alignment.CenterVertically) {
                        Text(layers.errors.entries.joinToString("\n") {(path,message)-> "${layers.selected[path]?.name.orEmpty()}: ${text(message)}"},Modifier.weight(1f),maxLines=3)
                        TextButton(onClick=tracks::refresh) {Text(stringResource(R.string.photos_retry))}
                    }
                }
                if(layers.geometry.values.any {it.simplified || it.omittedSegments>0} || route.data?.geometry?.simplified==true)
                    Text(stringResource(R.string.photos_tracks_simplified),Modifier.padding(horizontal=16.dp),style=MaterialTheme.typography.bodySmall)
                val bounds=unionMapBounds(listOfNotNull(initial,route.data?.geometry?.bounds)+layers.geometry.values.mapNotNull {it.bounds})
                if(bounds!=null) {
                    Text(pluralStringResource(R.plurals.photos_map_count,data?.total ?: 0,data?.total ?: 0),Modifier.padding(16.dp),style=MaterialTheme.typography.labelMedium)
                    PhotoMap(bounds,data?.markers.orEmpty(),Modifier.fillMaxWidth().weight(1f),onMarker={marker ->
                        if(marker.count>1) cluster=marker else scope.launch {
                            try { selected=controller.service.info(marker.path) }
                            catch(e: CancellationException) {throw e}
                            catch(e: Exception) {error=message(e)}
                        }
                    },onViewport={viewport ->
                        tracks.viewport(viewport)
                        photoRoute.viewport(viewport)
                        loading=true
                        try {data=controller.service.map(query,viewport);error=null}
                        catch(e: CancellationException) {throw e}
                        catch(e: Exception) {error=message(e)}
                        finally {loading=false}
                    },tracks=layers.geometry.values.toList()+listOfNotNull(route.data?.geometry),focus=layers.focus,tileImages=tileImages)
                } else if(!loading && error==null) Text(stringResource(R.string.photos_map_empty),Modifier.padding(24.dp))
            }
        }
        if(layerSheet) MapTrackSheet(tracks,photoRoute) {layerSheet=false}
        selected?.let {photo ->PhotoViewer(controller,images,listOf(photo),photo.path,onClose={selected=null},standalone=true)}
        cluster?.let {marker ->key(marker) {MapPhotoSelection(controller,images,query,marker) {cluster=null}}}
    }
}
