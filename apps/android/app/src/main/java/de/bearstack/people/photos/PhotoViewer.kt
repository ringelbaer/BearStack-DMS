package de.bearstack.people.photos

import de.bearstack.people.text.*
import android.text.format.Formatter
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.rememberTransformableState
import androidx.compose.foundation.gestures.transformable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import coil.ImageLoader
import coil.compose.AsyncImage
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotoMapBounds
import de.bearstack.people.data.remote.PhotoMapMarker
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.compose.ui.window.DialogWindowProvider
import androidx.compose.ui.text.style.TextOverflow

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PhotoViewer(controller: PhotosController, images: ImageLoader, photos: List<Photo>, path: String,
    onClose: () -> Unit = controller::closeViewer, standalone: Boolean = false) {
    val text=uiStrings()
    if(photos.isEmpty()) return
    val catalog by controller.state.collectAsStateWithLifecycle()
    val frame=catalog.frame && !standalone
    val initial=remember(controller,path) { photos.indexOfFirst { it.path==path }.coerceAtLeast(0) }
    val pager=rememberPagerState(initialPage=initial,pageCount={photos.size})
    val scope=rememberCoroutineScope()
    val context=LocalContext.current
    val locale=LocalConfiguration.current.locales[0]
    var infoOpen by remember { mutableStateOf(false) }
    var settingsOpen by remember {mutableStateOf(false)}
    var playing by remember(frame) {mutableStateOf(frame)}
    var controls by remember(frame) {mutableStateOf(!frame)}
    var zoomed by remember { mutableStateOf(false) }
    var readyPath by remember {mutableStateOf<String?>(null)}
    var endedPath by remember {mutableStateOf<String?>(null)}
    var mediaPlayingPath by remember {mutableStateOf<String?>(null)}
    var localSettings by remember {mutableStateOf(PlaybackSettings())}
    val settings=controller.playback?.state?.collectAsStateWithLifecycle()?.value ?: localSettings
    val seconds=(if(frame) settings.frameSeconds.takeIf {it>0} ?: controller.session.frameSeconds
        else settings.seconds.takeIf {it>0} ?: controller.session.slideshowSeconds).coerceIn(3,300)
    val current=photos[pager.currentPage.coerceAtMost(photos.lastIndex)]
    val lifecycle=LocalLifecycleOwner.current.lifecycle
    var foreground by remember(lifecycle) {mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED))}
    DisposableEffect(lifecycle) {
        val observer=LifecycleEventObserver {_,_ -> foreground=lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)}
        lifecycle.addObserver(observer)
        onDispose {lifecycle.removeObserver(observer)}
    }
    LaunchedEffect(pager.currentPage,photos.size) {
        if(!standalone && pager.currentPage>=photos.size-8) controller.more("media")
    }
    val adjacent=photos.getOrNull(pager.currentPage+1) ?: if(!catalog.hasNext && settings.repeat) photos.firstOrNull() else null
    LaunchedEffect(adjacent?.path,adjacent?.version,foreground) {
        if(foreground) controller.prefetch(adjacent,images)
    }
    LaunchedEffect(playing,current.path,seconds,foreground,infoOpen,settingsOpen,zoomed,readyPath,endedPath) {
        if(!playing || !foreground || infoOpen || settingsOpen || zoomed || standalone) return@LaunchedEffect
        if(current.type=="image") {
            if(readyPath!=current.path) return@LaunchedEffect
            delay(seconds*1000L)
        } else if(endedPath!=current.path) return@LaunchedEffect
        val next=pager.currentPage+1
        if(next>=controller.state.value.media.size && controller.state.value.hasNext) {
            controller.more("media")
            controller.state.map {it.media.size>next || !it.hasNext || it.error!=null}.first {it}
        }
        if(next<controller.state.value.media.size) pager.animateScrollToPage(next)
        else if(controller.state.value.error!=null) playing=false
        else if(settings.repeat) {if(pager.currentPage>0) pager.animateScrollToPage(0)}
        else playing=false
    }
    LaunchedEffect(controls,frame,playing,infoOpen,settingsOpen) {
        if(frame && controls && playing && !infoOpen && !settingsOpen) {delay(5000);controls=false}
    }
    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false,decorFitsSystemWindows=false)) {
        MaterialTheme(colorScheme=darkColorScheme(primary=Color(0xff75d2e8))) {
            PlaybackWindow(frame,foreground && (playing || mediaPlayingPath==current.path))
            Surface(Modifier.fillMaxSize(),color=Color.Black) {
                Box(Modifier.fillMaxSize().then(if(frame) Modifier.clickable {controls=!controls} else Modifier.safeDrawingPadding())) {
                    HorizontalPager(pager,key={photos[it].path},userScrollEnabled=!zoomed,
                        modifier=Modifier.fillMaxSize().then(if(frame) Modifier else Modifier.padding(top=64.dp,bottom=64.dp))) { index ->
                        val photo=photos[index]
                        if(photo.type=="image") ZoomablePhoto(photo,controller,images,index==pager.currentPage,frame && settings.frameFill,
                            showZoom=!frame,onReady={if(index==pager.currentPage) readyPath=if(it) photo.path else null}) {
                                if(index==pager.currentPage) zoomed=it
                            }
                        else if(index==pager.currentPage) {
                            LaunchedEffect(photo.path) {zoomed=false}
                            PhotoMediaPlayer(photo,controller,autoPlay=playing && !infoOpen && !settingsOpen,foreground=foreground,
                                onEnded={endedPath=photo.path},onPlaying={
                                    if(it) mediaPlayingPath=photo.path else if(mediaPlayingPath==photo.path) mediaPlayingPath=null
                                })
                        } else PhotoThumbnail(photo,controller,images,controller.session.thumbnailSize,Modifier.fillMaxSize())
                    }
                    if(controls) {
                        Column(Modifier.align(Alignment.TopCenter).fillMaxWidth().then(if(frame) Modifier.safeDrawingPadding() else Modifier)
                            .background(Color.Black.copy(alpha=.8f))) {
                            TopAppBar(title={Text(photoDateLabel(current.date,locale),style=MaterialTheme.typography.titleSmall,maxLines=2,overflow=TextOverflow.Ellipsis)},
                                colors=TopAppBarDefaults.topAppBarColors(containerColor=Color.Transparent),navigationIcon={
                                    IconButton(onClick=onClose) {Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_close))}
                                },actions={
                                    controller.downloads?.let {DownloadPhotoAction(current,it)}
                                    IconButton(onClick={infoOpen=true}) {Icon(painterResource(R.drawable.ic_info),stringResource(R.string.photos_info))}
                                    if(!standalone) IconButton(onClick={settingsOpen=true}) {Icon(painterResource(R.drawable.ic_more_horiz),stringResource(if(frame) R.string.photos_frame_settings else R.string.photos_slideshow_settings))}
                                })
                            controller.downloads?.let {PhotoDownloadStatus(it)}
                        }
                        Row(Modifier.align(Alignment.BottomCenter).fillMaxWidth().then(if(frame) Modifier.safeDrawingPadding() else Modifier)
                            .background(Color.Black.copy(alpha=.8f)).padding(horizontal=12.dp,vertical=8.dp),horizontalArrangement=Arrangement.SpaceBetween,
                            verticalAlignment=Alignment.CenterVertically) {
                            IconButton(onClick={scope.launch {pager.animateScrollToPage(pager.currentPage-1)}},enabled=pager.currentPage>0) {
                                Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_previous))
                            }
                            if(!standalone) IconButton(onClick={endedPath=null;playing=!playing}) {
                                Icon(painterResource(if(playing) R.drawable.ic_pause else R.drawable.ic_play),stringResource(if(playing) R.string.photos_pause else R.string.photos_play))
                            }
                            Text(stringResource(R.string.photos_of,pager.currentPage+1,if(standalone) photos.size else catalog.total),style=MaterialTheme.typography.labelSmall)
                            IconButton(onClick={scope.launch {pager.animateScrollToPage(pager.currentPage+1)}},enabled=pager.currentPage<photos.lastIndex) {
                                Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_next),Modifier.rotate(180f))
                            }
                        }
                    } else if(settings.frameCaptions) {
                        Surface(Modifier.align(Alignment.BottomCenter).safeDrawingPadding().padding(20.dp),color=Color.Black.copy(alpha=.6f)) {
                            Column(Modifier.padding(horizontal=16.dp,vertical=8.dp),horizontalAlignment=Alignment.CenterHorizontally) {
                                Text(current.name,maxLines=1,overflow=TextOverflow.Ellipsis,style=MaterialTheme.typography.titleSmall)
                                Text(photoDateLabel(current.date,locale),style=MaterialTheme.typography.bodySmall)
                            }
                        }
                    }
                }
                if(settingsOpen) PlaybackSettingsDialog(settings,seconds,frame,
                    onSave={localSettings=it;controller.playback?.save(it)},onDismiss={settingsOpen=false})
                if(infoOpen) {
                    var detail by remember(current.path) { mutableStateOf<Photo?>(null) }
                    var error by remember(current.path) { mutableStateOf<UiText?>(null) }
                    LaunchedEffect(current.path) {
                        try { detail=controller.service.info(current.path) }
                        catch(e: CancellationException) {throw e}
                        catch(e: Exception) {error=failureText(e)}
                    }
                    ModalBottomSheet(onDismissRequest={infoOpen=false},sheetState=rememberModalBottomSheetState(skipPartiallyExpanded=true)) {
                        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal=24.dp).padding(bottom=32.dp),verticalArrangement=Arrangement.spacedBy(20.dp)) {
                            Text(photoDateLabel(current.date,locale),style=MaterialTheme.typography.headlineSmall)
                            if(detail==null && error==null) LinearProgressIndicator(Modifier.fillMaxWidth())
                            error?.let {Text(text(it),color=MaterialTheme.colorScheme.error)}
                            val item=detail ?: current
                            Column(verticalArrangement=Arrangement.spacedBy(6.dp)) {
                                Text(stringResource(R.string.photos_file),style=MaterialTheme.typography.labelMedium,color=MaterialTheme.colorScheme.primary)
                                Text(item.name,style=MaterialTheme.typography.titleMedium)
                                Text(stringResource(R.string.photos_details,item.width,item.height,Formatter.formatFileSize(context,item.bytes)))
                                Text(item.path,style=MaterialTheme.typography.bodySmall)
                            }
                            if(item.camera.isNotBlank() || item.lens.isNotBlank()) Column(verticalArrangement=Arrangement.spacedBy(6.dp)) {
                                Text(stringResource(R.string.photos_camera),style=MaterialTheme.typography.labelMedium,color=MaterialTheme.colorScheme.primary)
                                Text(listOf(item.camera,item.lens).filter(String::isNotBlank).joinToString("\n"))
                            }
                            if(item.latitude!=null && item.longitude!=null) Column(verticalArrangement=Arrangement.spacedBy(6.dp)) {
                                Text(stringResource(R.string.photos_location),style=MaterialTheme.typography.labelMedium,color=MaterialTheme.colorScheme.primary)
                                Text(String.format(locale,"%.5f, %.5f",item.latitude,item.longitude))
                                key(item.path) {
                                    PhotoMap(PhotoMapBounds(item.latitude,item.longitude,item.latitude,item.longitude),
                                        listOf(PhotoMapMarker(item.latitude,item.longitude,1)),Modifier.fillMaxWidth().height(240.dp))
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable private fun PlaybackWindow(frame: Boolean, awake: Boolean) {
    val view=LocalView.current
    val window=(view.parent as? DialogWindowProvider)?.window
    DisposableEffect(view,window,frame,awake) {
        val previous=view.keepScreenOn
        view.keepScreenOn=awake
        val insets=window?.let {WindowCompat.getInsetsController(it,view)}
        if(frame) {
            insets?.systemBarsBehavior=WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            insets?.hide(WindowInsetsCompat.Type.systemBars())
        }
        onDispose {view.keepScreenOn=previous;if(frame) insets?.show(WindowInsetsCompat.Type.systemBars())}
    }
}

@Composable private fun ZoomablePhoto(photo: Photo, controller: PhotosController, images: ImageLoader, active: Boolean, fill: Boolean, showZoom: Boolean, onReady: (Boolean)->Unit, onZoomed: (Boolean) -> Unit) {
    var zoom by remember(photo.path) { mutableFloatStateOf(1f) }
    var pan by remember(photo.path) { mutableStateOf(Offset.Zero) }
    var size by remember { mutableStateOf(IntSize.Zero) }
    var loading by remember(photo.path) { mutableStateOf(true) }
    var failed by remember(photo.path) { mutableStateOf(false) }
    var attempt by remember(photo.path) { mutableIntStateOf(0) }
    LaunchedEffect(active) { zoom=1f;pan=Offset.Zero;if(active) onZoomed(false) }
    LaunchedEffect(active,loading,failed) {if(active) onReady(!loading && !failed)}
    val transform=rememberTransformableState { scale,delta,_ ->
        zoom=(zoom*scale).coerceIn(1f,6f)
        val maxX=size.width*(zoom-1)/2f;val maxY=size.height*(zoom-1)/2f
        pan=Offset((pan.x+delta.x).coerceIn(-maxX,maxX),(pan.y+delta.y).coerceIn(-maxY,maxY))
        onZoomed(zoom>1f)
    }
    val context=LocalContext.current
    val request=remember(photo.path,photo.version,attempt,controller,context) {photoPreviewRequest(context,photo,controller.service,controller.session)}
    Box(Modifier.fillMaxSize().clipToBounds().onSizeChanged {size=it}.transformable(transform,canPan={zoom>1f}),contentAlignment=Alignment.Center) {
        AsyncImage(request,stringResource(R.string.photos_original),imageLoader=images,contentScale=if(fill) ContentScale.Crop else ContentScale.Fit,
            onLoading={loading=true;failed=false},onSuccess={loading=false},onError={loading=false;failed=true},
            modifier=Modifier.fillMaxSize().graphicsLayer {scaleX=zoom;scaleY=zoom;translationX=pan.x;translationY=pan.y})
        if(loading) CircularProgressIndicator()
        if(failed) Column(horizontalAlignment=Alignment.CenterHorizontally) {
            Text(stringResource(R.string.photos_image_error))
            TextButton(onClick={attempt++}) {Text(stringResource(R.string.photos_retry))}
        }
        if(showZoom && !loading && !failed) TextButton(onClick={zoom=if(zoom==1f) 2f else 1f;pan=Offset.Zero;onZoomed(zoom>1f)},
            modifier=Modifier.align(Alignment.BottomCenter).background(Color.Black.copy(alpha=.55f))) {
            Text(stringResource(if(zoom==1f) R.string.photos_zoom else R.string.photos_zoom_reset))
        }
    }
}
