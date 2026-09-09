package de.bearstack.people.photos

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.rememberTransformableState
import androidx.compose.foundation.gestures.transformable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.CachePolicy
import coil.request.ImageRequest
import de.bearstack.people.R
import de.bearstack.people.data.remote.PhotoMapBounds
import de.bearstack.people.data.remote.PhotoMapMarker
import de.bearstack.people.data.remote.PhotoTrackGeometry
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlin.math.pow
import kotlin.math.roundToInt

private val cameraSaver=listSaver<MapCamera,Double>(save={listOf(it.x,it.y,it.zoom)},restore={MapCamera(it[0],it[1],it[2])})

@OptIn(FlowPreview::class)
@Composable
internal fun PhotoMap(bounds: PhotoMapBounds, markers: List<PhotoMapMarker>, modifier: Modifier = Modifier,
    onMarker: (PhotoMapMarker) -> Unit = {}, onViewport: suspend (PhotoMapBounds) -> Unit = {},
    tileImages: coil.ImageLoader? = null, tracks: List<PhotoTrackGeometry> = emptyList(), focus: MapFocus? = null) {
    val context=LocalContext.current
    val density=LocalDensity.current.density.toDouble()
    val tilePixels=256*density
    val images=tileImages ?: remember(context) {MapTiles.images(context)}
    var size by remember {mutableStateOf(IntSize.Zero)}
    var camera by rememberSaveable(stateSaver=cameraSaver) {mutableStateOf(MapCamera())}
    var positioned by rememberSaveable {mutableStateOf(false)}
    var retry by remember {mutableIntStateOf(0)}
    val failures=remember {mutableStateMapOf<String,Boolean>()}
    val viewportCallback by rememberUpdatedState(onViewport)
    val transform=rememberTransformableState { zoom, pan, _ ->
        camera=camera.move(pan.x.toDouble(),pan.y.toDouble(),zoom.toDouble(),tilePixels)
    }
    LaunchedEffect(size,bounds) {
        if(!positioned && size.width>0 && size.height>0) {
            camera=fitMap(bounds,size.width.toDouble(),size.height.toDouble(),tilePixels)
            positioned=true
        }
    }
    LaunchedEffect(focus,size) {
        if(focus!=null && size.width>0 && size.height>0) {
            camera=fitMap(focus.bounds,size.width.toDouble(),size.height.toDouble(),tilePixels)
            positioned=true
        }
    }
    LaunchedEffect(tilePixels) {
        snapshotFlow { if(positioned && size.width>0 && size.height>0) camera.bounds(size.width.toDouble(),size.height.toDouble(),tilePixels) else null }
            .debounce(300).distinctUntilChanged().collectLatest { it?.let {viewportCallback(it)} }
    }
    val label=stringResource(R.string.photos_map)
    val east=stringResource(R.string.photos_map_east)
    val west=stringResource(R.string.photos_map_west)
    val north=stringResource(R.string.photos_map_north)
    val south=stringResource(R.string.photos_map_south)
    val zoomIn=stringResource(R.string.photos_map_zoom_in)
    val zoomOut=stringResource(R.string.photos_map_zoom_out)
    val fit=stringResource(R.string.photos_map_fit)
    fun pan(dx: Double,dy: Double): Boolean {camera=camera.move(dx,dy,1.0,tilePixels);return true}
    Box(modifier.clipToBounds().background(MaterialTheme.colorScheme.surfaceContainer)
        .onSizeChanged {size=it}.transformable(transform).semantics {
            contentDescription=label
            customActions=listOf(
                CustomAccessibilityAction(east) {pan(-size.width*.3,0.0)},CustomAccessibilityAction(west) {pan(size.width*.3,0.0)},
                CustomAccessibilityAction(north) {pan(0.0,size.height*.3)},CustomAccessibilityAction(south) {pan(0.0,-size.height*.3)})
        }) {
        val tiles=if(positioned) visibleMapTiles(camera,size.width.toDouble(),size.height.toDouble(),tilePixels) else emptyList()
        val visibleUrls=tiles.map {it.url}.toSet()
        LaunchedEffect(visibleUrls) {failures.keys.retainAll(visibleUrls)}
        tiles.forEach { tile ->
            key(tile.zoom,tile.column,tile.y) {
                val request=remember(tile.url,retry) {ImageRequest.Builder(context).data(tile.url).size(256,256)
                    // HTTP cache performs freshness checks, including after returning to the map.
                    .memoryCachePolicy(CachePolicy.DISABLED).diskCachePolicy(CachePolicy.DISABLED).build()}
                AsyncImage(request,imageLoader=images,contentDescription=null,contentScale=ContentScale.FillBounds,
                    onError={failures[tile.url]=true},onSuccess={failures.remove(tile.url)},
                    modifier=Modifier.offset {IntOffset(tile.left.roundToInt(),tile.top.roundToInt())}
                        .requiredSize((tile.size/density).dp))
            }
        }
        if(positioned) MapTrackLines(tracks,camera,tilePixels)
        val world=tilePixels*2.0.pow(camera.zoom)
        markers.forEach { marker ->
            val point=mapProject(marker.latitude,marker.longitude)
            val dx=((point.x-camera.x+.5)%1+1)%1-.5
            val offset=Offset((size.width/2+dx*world).toFloat(),(size.height/2+(point.y-camera.y)*world).toFloat())
            if(positioned && offset.x>=0 && offset.x<=size.width && offset.y>=0 && offset.y<=size.height) {
                val description=stringResource(if(marker.count==1) R.string.photos_map_marker_one else R.string.photos_map_marker,marker.count)
                FilledTonalButton(onClick={
                    val extent=marker.bounds
                    val coincident=extent!=null && extent.north-extent.south<1e-7 && kotlin.math.abs(extent.east-extent.west)<1e-7
                    if((marker.count==1 && marker.path.isNotBlank()) || (marker.count>1 && (coincident || camera.zoom>=18))) onMarker(marker)
                    else camera=MapCamera(point.x,point.y,(camera.zoom+2).coerceAtMost(18.0))
                },shape=CircleShape,contentPadding=PaddingValues(0.dp),
                    modifier=Modifier.offset {IntOffset((offset.x-24*density).roundToInt(),(offset.y-24*density).roundToInt())}
                        .size(48.dp).semantics {contentDescription=description}) {
                    Text(if(marker.count==1) "●" else "${marker.count}",style=MaterialTheme.typography.labelMedium,maxLines=1)
                }
            }
        }
        Column(Modifier.align(Alignment.TopEnd).padding(8.dp),verticalArrangement=Arrangement.spacedBy(4.dp)) {
            Surface(shape=CircleShape,shadowElevation=2.dp) {
                IconButton(onClick={camera=camera.move(0.0,0.0,2.0,tilePixels)},enabled=camera.zoom<18,
                    modifier=Modifier.semantics {contentDescription=zoomIn}) {Text("+",style=MaterialTheme.typography.headlineSmall)}
            }
            Surface(shape=CircleShape,shadowElevation=2.dp) {
                IconButton(onClick={camera=camera.move(0.0,0.0,.5,tilePixels)},enabled=camera.zoom>2,
                    modifier=Modifier.semantics {contentDescription=zoomOut}) {Text("−",style=MaterialTheme.typography.headlineSmall)}
            }
            Surface(shape=CircleShape,shadowElevation=2.dp) {
                IconButton(onClick={camera=fitMap(bounds,size.width.toDouble(),size.height.toDouble(),tilePixels)},
                    modifier=Modifier.semantics {contentDescription=fit}) {Text("⊙",style=MaterialTheme.typography.headlineSmall)}
            }
        }
        if(failures.keys.any {it in visibleUrls}) Surface(Modifier.align(Alignment.TopStart).padding(8.dp),shape=MaterialTheme.shapes.medium) {
            TextButton(onClick={retry++;failures.clear()}) {Text(stringResource(R.string.photos_map_retry))}
        }
        val uri=LocalUriHandler.current
        Surface(Modifier.align(Alignment.BottomEnd),color=Color.White.copy(alpha=.95f),contentColor=Color.Black) {
            TextButton(onClick={uri.openUri("https://www.openstreetmap.org/copyright")},contentPadding=PaddingValues(horizontal=8.dp),
                colors=ButtonDefaults.textButtonColors(contentColor=Color.Black)) {
                Text("© OpenStreetMap contributors",style=MaterialTheme.typography.labelSmall)
            }
        }
    }
}
