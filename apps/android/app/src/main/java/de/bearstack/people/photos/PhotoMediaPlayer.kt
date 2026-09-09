package de.bearstack.people.photos

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.ProgressiveMediaSource
import androidx.media3.ui.PlayerView
import de.bearstack.people.R
import de.bearstack.people.data.remote.Photo
import de.bearstack.people.data.remote.PhotosApi

// Only the visible page owns a decoder. Original media uses the same pinned,
// origin-restricted HTTP client as photos; no separate player credentials.
@androidx.annotation.OptIn(UnstableApi::class)
@Composable internal fun PhotoMediaPlayer(photo: Photo, controller: PhotosController, autoPlay: Boolean,
    foreground: Boolean, onEnded: () -> Unit, onPlaying: (Boolean) -> Unit) {
    val context=LocalContext.current
    val client=(controller.service as? PhotosApi)?.streamingClient
    if(client==null) {Text(stringResource(R.string.photos_media_error));return}
    val ended by rememberUpdatedState(onEnded)
    val playing by rememberUpdatedState(onPlaying)
    var error by remember(photo.path) {mutableStateOf(false)}
    val player=remember(photo.path,photo.version,client) {
        ExoPlayer.Builder(context).setLoadControl(DefaultLoadControl.Builder()
            .setBufferDurationsMs(5000,15000,1000,2000).setTargetBufferBytes(16*1024*1024)
            .setPrioritizeTimeOverSizeThresholds(false).build()).build().apply {
                val source=ProgressiveMediaSource.Factory(OkHttpDataSource.Factory(client)).createMediaSource(
                    MediaItem.Builder().setUri(controller.service.original(photo)).setMimeType(photo.mime).build())
                setMediaSource(source)
                addListener(object:Player.Listener {
                    override fun onPlaybackStateChanged(state: Int) {if(state==Player.STATE_ENDED) ended()}
                    override fun onIsPlayingChanged(isPlaying: Boolean) {playing(isPlaying)}
                    override fun onPlayerError(exception: PlaybackException) {error=true}
                })
                prepare()
            }
    }
    LaunchedEffect(autoPlay,foreground) {
        if(!foreground) player.pause()
        else if(autoPlay) {if(player.playbackState==Player.STATE_ENDED) player.seekTo(0);player.play()}
        else player.pause()
    }
    DisposableEffect(player) {onDispose {player.release();playing(false)}}
    Box(Modifier.fillMaxSize(),contentAlignment=Alignment.Center) {
        AndroidView(factory={PlayerView(it).apply {this.player=player;setShowBuffering(PlayerView.SHOW_BUFFERING_WHEN_PLAYING)}},
            modifier=Modifier.fillMaxSize(),onRelease={it.player=null})
        if(error) Surface {
            Column(horizontalAlignment=Alignment.CenterHorizontally) {
                Text(stringResource(R.string.photos_media_error))
                TextButton(onClick={error=false;player.prepare()}) {Text(stringResource(R.string.photos_retry))}
            }
        }
    }
}
