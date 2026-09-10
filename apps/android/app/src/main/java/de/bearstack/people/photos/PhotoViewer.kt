package de.bearstack.people.photos

import de.bearstack.people.text.*
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.rememberTransformableState
import androidx.compose.foundation.gestures.transformable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
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
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay
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
    val initial=remember(controller) { photos.indexOfFirst { it.path==path }.coerceAtLeast(0) }
    val pager=rememberPagerState(initialPage=initial,pageCount={photos.size})
    val scope=rememberCoroutineScope()
    val locale=LocalConfiguration.current.locales[0]
    var infoOpen by remember { mutableStateOf(false) }
    var settingsOpen by remember {mutableStateOf(false)}
    var playing by remember(frame) {mutableStateOf(frame)}
    var controls by remember(frame) {mutableStateOf(!frame)}
    var zoomed by remember { mutableStateOf(false) }
    var readyPath by remember {mutableStateOf<String?>(null)}
    var mediaPlayingPath by remember {mutableStateOf<String?>(null)}
    var localSettings by remember {mutableStateOf(PlaybackSettings())}
    val settings=controller.playback?.state?.collectAsStateWithLifecycle()?.value ?: localSettings
    val seconds=(if(frame) settings.frameSeconds.takeIf {it>0} ?: controller.session.frameSeconds
        else settings.seconds.takeIf {it>0} ?: controller.session.slideshowSeconds).coerceIn(3,300)
    val visibleKey by remember(pager) {derivedStateOf {
        pager.layoutInfo.visiblePagesInfo.firstOrNull {it.index==pager.currentPage}?.key
    }}
    val current=photos.firstOrNull {it.path==visibleKey} ?: photos[pager.currentPage.coerceAtMost(photos.lastIndex)]
    val currentIndex=photos.indexOfFirst {it.path==current.path}
    var mediaEnded by remember(current.path) {mutableStateOf(false)}
    var playbackCycle by remember {mutableIntStateOf(0)}
    var pendingPath by remember {mutableStateOf<String?>(null)}
    var moving by remember {mutableStateOf(false)}
    suspend fun move(direction: Int, repeat: Boolean = false) {
        if(moving || pendingPath!=null || pager.isScrollInProgress) return
        moving=true
        try {
            val next=if(standalone) photos.getOrNull(currentIndex+direction) else controller.neighbour(current.path,direction,repeat)
            if(next?.path==current.path) {mediaEnded=false;playbackCycle++}
            else if(next!=null) pendingPath=next.path
            else {playing=false;controls=true}
        } finally {moving=false}
    }
    LaunchedEffect(pendingPath,photos) {
        val target=pendingPath ?: return@LaunchedEffect
        val index=photos.indexOfFirst {it.path==target}
        if(index>=0) {pager.animateScrollToPage(index);pendingPath=null}
    }
    val lifecycle=LocalLifecycleOwner.current.lifecycle
    var foreground by remember(lifecycle) {mutableStateOf(lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED))}
    DisposableEffect(lifecycle) {
        val observer=LifecycleEventObserver {_,_ -> foreground=lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)}
        lifecycle.addObserver(observer)
        onDispose {lifecycle.removeObserver(observer)}
    }
    LaunchedEffect(current.path,photos) {
        if(!standalone) {
            controller.viewerAt(current.path)
            if(currentIndex<8 && catalog.mediaPages.hasPrevious) controller.previous("media")
            else if(currentIndex>=photos.size-8) controller.more("media")
        }
    }
    val adjacent=photos.getOrNull(currentIndex+1) ?: if(!catalog.hasNext && !catalog.mediaPages.hasPrevious && settings.repeat) photos.firstOrNull() else null
    LaunchedEffect(adjacent?.path,adjacent?.version,foreground) {
        if(foreground) controller.prefetch(adjacent,images)
    }
    LaunchedEffect(playing,current.path,seconds,foreground,infoOpen,settingsOpen,zoomed,readyPath,mediaEnded,playbackCycle,pager.isScrollInProgress) {
        if(!playing || !foreground || infoOpen || settingsOpen || zoomed || standalone || pager.isScrollInProgress) return@LaunchedEffect
        if(current.type=="image") {
            if(readyPath!=current.path) return@LaunchedEffect
            delay(seconds*1000L)
        } else if(!mediaEnded) return@LaunchedEffect
        move(1,settings.repeat)
    }
    LaunchedEffect(controls,frame,playing,infoOpen,settingsOpen) {
        if(frame && controls && playing && !infoOpen && !settingsOpen) {delay(5000);controls=false}
    }
    Dialog(onDismissRequest=onClose,properties=DialogProperties(usePlatformDefaultWidth=false,decorFitsSystemWindows=false)) {
        de.bearstack.people.ui.BearStackTheme(dark=true) {
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
                            PhotoMediaPlayer(photo,controller,autoPlay=playing,foreground=foreground && !infoOpen && !settingsOpen && !pager.isScrollInProgress,
                                replay=playbackCycle,onControlsShown={controls=true},onEnded={mediaEnded=true},onPlaying={
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
                                    if(current.type=="image") PhotoShareAction(current,controller.service,onShare={playing=false;controls=true})
                                    controller.downloads?.let {DownloadPhotoAction(current,it)}
                                    IconButton(onClick={infoOpen=true}) {Icon(painterResource(R.drawable.ic_info),stringResource(R.string.photos_info))}
                                    if(!standalone) IconButton(onClick={settingsOpen=true}) {Icon(painterResource(R.drawable.ic_more_horiz),stringResource(if(frame) R.string.photos_frame_settings else R.string.photos_slideshow_settings))}
                                })
                            controller.downloads?.let {PhotoDownloadStatus(it)}
                            if(!standalone && "media" in catalog.loadingSections) LinearProgressIndicator(Modifier.fillMaxWidth())
                            if(!standalone) catalog.pageErrors["media"]?.let {error ->
                                Row(Modifier.fillMaxWidth().padding(horizontal=12.dp),verticalAlignment=Alignment.CenterVertically) {
                                    Text(text(error.message),Modifier.weight(1f),style=MaterialTheme.typography.bodySmall)
                                    TextButton(onClick={controller.retryPage("media")}) {Text(stringResource(R.string.photos_retry))}
                                }
                            }
                        }
                        Row(Modifier.align(Alignment.BottomCenter).fillMaxWidth().then(if(frame) Modifier.safeDrawingPadding() else Modifier)
                            .background(Color.Black.copy(alpha=.8f)).padding(horizontal=12.dp,vertical=8.dp),horizontalArrangement=Arrangement.SpaceBetween,
                            verticalAlignment=Alignment.CenterVertically) {
                            IconButton(onClick={scope.launch {move(-1)}},enabled=!moving && pendingPath==null && !pager.isScrollInProgress && (currentIndex>0 || (!standalone && catalog.mediaPages.hasPrevious))) {
                                Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_previous))
                            }
                            if(!standalone) IconButton(onClick={mediaEnded=false;playing=!playing}) {
                                Icon(painterResource(if(playing) R.drawable.ic_pause else R.drawable.ic_play),stringResource(if(playing) R.string.photos_pause else R.string.photos_play))
                            }
                            Text(stringResource(R.string.photos_of,if(standalone) currentIndex+1 else catalog.mediaPages.position(current.path),if(standalone) photos.size else catalog.total),style=MaterialTheme.typography.labelSmall)
                            IconButton(onClick={scope.launch {move(1)}},enabled=!moving && pendingPath==null && !pager.isScrollInProgress && (currentIndex<photos.lastIndex || (!standalone && catalog.hasNext))) {
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
                if(infoOpen) PhotoInfoSheet(current,controller.service) {infoOpen=false}
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
