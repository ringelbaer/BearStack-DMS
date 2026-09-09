package de.bearstack.people.photos

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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MapPhotoSelection(controller: PhotosController,images: ImageLoader,query: PhotoQuery,marker: PhotoMapMarker,onClose: ()->Unit) {
    val extent=marker.bounds ?: PhotoMapBounds(marker.latitude,marker.longitude,marker.latitude,marker.longitude)
    // Expand a degenerate point by less than a millimetre to obtain valid query
    // bounds; the same small padding keeps floating point edge points included.
    val bounds=remember(extent) {expandMapBounds(extent)}
    val scope=rememberCoroutineScope()
    val context=androidx.compose.ui.platform.LocalContext.current
    val selection=remember(controller,query,bounds) {
        val source=controller.service
        val selectedQuery=query
        val service=object : PhotosService by source {
            override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
                val result=source.mapMedia(selectedQuery,bounds,page)
                return PhotoPage(selectedQuery.path,"",result.page,result.total,result.hasNext,0,false,false,
                    result.media,emptyList(),emptyList())
            }
        }
        PhotosController(scope,service,controller.session,context)
    }
    DisposableEffect(selection) {onDispose {selection.close()}}
    val state by selection.state.collectAsState()
    val count=if(state.loading || state.error!=null) marker.count else state.total
    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false)) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.safeDrawingPadding()) {
                TopAppBar(title={Text(pluralStringResource(R.plurals.photos_map_selection,count,count))},navigationIcon={
                    IconButton(onClick=onClose) {Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back))}
                })
                Box(Modifier.fillMaxWidth().height(3.dp)) {
                    if(state.loading || state.loadingSections.isNotEmpty()) LinearProgressIndicator(Modifier.fillMaxSize())
                }
                if(state.error!=null) TextButton(onClick={selection.open(state.query)}) {
                    Text(stringResource(R.string.photos_map_error)+" "+stringResource(R.string.photos_retry))
                }
                PhotoGallery(selection,images,state,Modifier.fillMaxWidth().weight(1f))
            }
        }
        state.selected?.let {path ->PhotoViewer(selection,images,state.media,path)}
    }
}
