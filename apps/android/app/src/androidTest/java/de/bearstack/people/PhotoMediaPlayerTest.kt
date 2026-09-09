package de.bearstack.people

import android.util.Base64
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.*
import androidx.media3.common.Player
import androidx.media3.ui.PlayerView
import android.view.View
import android.view.ViewGroup
import android.view.inspector.WindowInspector
import androidx.test.filters.SdkSuppress
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.connection.Connections
import de.bearstack.people.connection.Profile
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import okhttp3.Credentials
import okhttp3.mockwebserver.*
import okhttp3.tls.HandshakeCertificates
import okhttp3.tls.HeldCertificate
import okio.Buffer
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.Locale

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
class PhotoMediaPlayerTest {
    @get:Rule val compose=createComposeRule()
    @Test fun pinnedVideoPlaybackPausesInBackgroundAndKeepsCredentialsOnTheConfiguredServer()=video {controller,photo,_,_,_ ->
        val active=mutableStateOf(true)
        val playing=AtomicBoolean(false)
        val ended=AtomicBoolean(false)
        compose.setContent {MaterialTheme {
            PhotoMediaPlayer(photo,controller,true,active.value,{ended.set(true)},{playing.set(it)})
        }}
        compose.waitUntil(15_000) {playing.get()}
        compose.runOnUiThread {active.value=false}
        compose.waitUntil(5_000) {!playing.get()}
        assertFalse(ended.get())
        compose.runOnUiThread {active.value=true}
        compose.waitUntil(15_000) {ended.get()}
    }
    @Test @SdkSuppress(minSdkVersion=29)
    fun singleVideoFrameRepeatsThenStopsAndReturnsToGallery()=video {controller,_,images,requested,_ ->
        compose.setLocalizedContent(Locale.ENGLISH) {MaterialTheme {PhotosScreen(controller,images,false,{},{})}}
        compose.waitUntil(10_000) {!controller.state.value.loading}
        compose.onNodeWithContentDescription("More options").performClick()
        compose.onNodeWithText("Start photo frame").performClick()
        compose.waitUntil(15_000) {requested.get()}
        val ends=AtomicInteger(0)
        lateinit var player:Player
        lateinit var view:PlayerView
        compose.runOnIdle {
            fun players(root:View):List<PlayerView> = when(root) {
                is PlayerView -> listOf(root)
                is ViewGroup -> (0 until root.childCount).flatMap {players(root.getChildAt(it))}
                else -> emptyList()
            }
            view=WindowInspector.getGlobalWindowViews().flatMap(::players).single()
            assertTrue(view.isShown)
            player=view.player!!
            player.addListener(object:Player.Listener {
                override fun onPlaybackStateChanged(state:Int) {if(state==Player.STATE_ENDED) ends.incrementAndGet()}
            })
        }
        compose.waitUntil(20_000) {ends.get()>=2}
        // Showing the native player controls also exposes the frame actions.
        compose.runOnUiThread {view.showController()}
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Repeat at the end").performClick()
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil(15_000) {compose.onAllNodesWithContentDescription("Start slideshow").fetchSemanticsNodes().isNotEmpty()}
        compose.runOnUiThread {assertFalse(player.isPlaying);assertEquals(Player.STATE_ENDED,player.playbackState)}
        compose.onNodeWithContentDescription("Close").performClick()
        compose.onNodeWithContentDescription("clip.mp4").assertIsDisplayed()
        assertFalse(controller.state.value.frame)
    }
    private fun video(test:(PhotosController,Photo,ImageLoader,AtomicBoolean,AtomicBoolean)->Unit) {
        val bytes=InstrumentationRegistry.getInstrumentation().context.assets.open("gallery-test.mp4").use {it.readBytes()}
        val certificate=HeldCertificate.Builder().addSubjectAlternativeName("localhost").build()
        val server=MockWebServer()
        server.useHttps(HandshakeCertificates.Builder().heldCertificate(certificate).build().sslSocketFactory(),false)
        val originalRequested=AtomicBoolean(false)
        val invalidRequest=AtomicBoolean(false)
        server.dispatcher=object:Dispatcher() {
            override fun dispatch(request:RecordedRequest):MockResponse {
                if(request.getHeader("Authorization")!=Credentials.basic("reader","secret") || !request.path.orEmpty().startsWith("/prefix/api/photos/v1/")) {
                    invalidRequest.set(true);return MockResponse().setResponseCode(403)
                }
                if(request.path.orEmpty().startsWith("/prefix/api/photos/v1/media?")) {
                    originalRequested.set(true)
                    return MockResponse().setHeader("Content-Type","video/mp4").setBody(Buffer().write(bytes))
                }
                return MockResponse().setHeader("Content-Type","application/json").setBody("""{"path":"","parent":"","page":1,"total":1,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[{"path":"clip.mp4","name":"clip.mp4","type":"video","mime":"video/mp4","version":"1","modified":"2026-09-09T10:00:00Z","bytes":1024,"width":160,"height":90}],"folders":[],"blogs":[]}""")
            }
        }
        server.start()
        val address=server.url("/prefix/").toString()
        val client=Connections.client(Profile(address,"reader","secret",Base64.encodeToString(certificate.certificate.encoded,Base64.NO_WRAP)))
        val images=ImageLoader.Builder(InstrumentationRegistry.getInstrumentation().targetContext).okHttpClient(client).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,PhotosApi(client,address),PhotoSession("media",false,240,240,1280,2048,5,8))}
        try {
            test(controller,Photo("clip.mp4","clip.mp4","video","video/mp4","1","2026-09-09T10:00:00Z",null,bytes.size.toLong(),160,90),
                images,originalRequested,invalidRequest)
            assertTrue(originalRequested.get());assertFalse(invalidRequest.get())
        } finally {
            compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()}
            runBlocking {Connections.close(client)}
            server.close()
        }
    }
}
