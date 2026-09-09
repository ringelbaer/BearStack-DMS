package de.bearstack.people.photos

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.*
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MapPhotoSelection(controller: PhotosController,images: ImageLoader,query: PhotoQuery,marker: PhotoMapMarker,onClose: ()->Unit) {
    val extent=marker.bounds ?: PhotoMapBounds(marker.latitude,marker.longitude,marker.latitude,marker.longitude)
    // Expand a degenerate point by less than a millimetre to obtain valid query
    // bounds; the same small padding keeps floating point edge points included.
    val bounds=remember(extent) {expandMapBounds(extent)}
    var page by remember {mutableIntStateOf(1)}
    var result by remember {mutableStateOf<PhotoMapPage?>(null)}
    var loading by remember {mutableStateOf(false)}
    var failed by remember {mutableStateOf(false)}
    var retry by remember {mutableIntStateOf(0)}
    var selected by remember {mutableStateOf<String?>(null)}
    LaunchedEffect(page,retry) {
        loading=true;failed=false
        try {result=controller.service.mapMedia(query,bounds,page)}
        catch(e: CancellationException) {throw e}
        catch(_: Exception) {failed=true}
        finally {loading=false}
    }
    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false)) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.safeDrawingPadding()) {
                TopAppBar(title={Text(pluralStringResource(R.plurals.photos_map_selection,result?.total ?: marker.count,result?.total ?: marker.count))},navigationIcon={
                    IconButton(onClick=onClose) {Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back))}
                })
                if(loading) LinearProgressIndicator(Modifier.fillMaxWidth())
                if(failed) TextButton(onClick={retry++}) {Text(stringResource(R.string.photos_map_error)+" "+stringResource(R.string.photos_retry))}
                key(result?.page) {
                    LazyVerticalGrid(GridCells.Fixed(3),Modifier.weight(1f),horizontalArrangement=Arrangement.spacedBy(2.dp),verticalArrangement=Arrangement.spacedBy(2.dp)) {
                        items(result?.media.orEmpty(),key={it.path}) {photo ->
                            PhotoThumbnail(photo,controller,images,controller.session.thumbnailSize,
                                Modifier.aspectRatio(1f).clickable(enabled=!loading) {selected=photo.path})
                        }
                    }
                }
                Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.SpaceBetween) {
                    TextButton(onClick={page=(result?.page ?: page)-1},enabled=!loading && (result?.page ?: page)>1) {Text(stringResource(R.string.photos_map_previous_page))}
                    Text(stringResource(R.string.photos_map_page,result?.page ?: page),Modifier.padding(16.dp))
                    TextButton(onClick={page=(result?.page ?: page)+1},enabled=!loading && result?.hasNext==true) {Text(stringResource(R.string.photos_map_next_page))}
                }
            }
        }
        selected?.let {path ->PhotoViewer(controller,images,result?.media.orEmpty(),path,onClose={selected=null},standalone=true)}
    }
}
