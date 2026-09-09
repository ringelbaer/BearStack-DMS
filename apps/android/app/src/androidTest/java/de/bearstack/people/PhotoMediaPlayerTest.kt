package de.bearstack.people

import android.util.Base64
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
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

class PhotoMediaPlayerTest {
    @get:Rule val compose=createComposeRule()
    @Test fun pinnedVideoPlaybackPausesInBackgroundAndKeepsCredentialsOnTheConfiguredServer() {
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
                return MockResponse().setHeader("Content-Type","application/json").setBody("""{"path":"","parent":"","page":1,"total":0,"has_next":false,"folder_total":0,"folder_has_next":false,"blog_has_next":false,"media":[],"folders":[],"blogs":[]}""")
            }
        }
        server.start()
        val address=server.url("/prefix/").toString()
        val client=Connections.client(Profile(address,"reader","secret",Base64.encodeToString(certificate.certificate.encoded,Base64.NO_WRAP)))
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,PhotosApi(client,address),PhotoSession("media",false,240,240,1280,2048,5,8))}
        val active=mutableStateOf(true)
        val playing=AtomicBoolean(false)
        val ended=AtomicBoolean(false)
        try {
            compose.setContent {MaterialTheme {
                PhotoMediaPlayer(Photo("clip.mp4","clip.mp4","video","video/mp4","1","2026-09-09T10:00:00Z",null,bytes.size.toLong(),160,90),
                    controller,true,active.value,{ended.set(true)},{playing.set(it)})
            }}
            compose.waitUntil(15_000) {playing.get()}
            compose.runOnUiThread {active.value=false}
            compose.waitUntil(5_000) {!playing.get()}
            assertFalse(ended.get())
            compose.runOnUiThread {active.value=true}
            compose.waitUntil(15_000) {ended.get()}
            assertTrue(originalRequested.get());assertFalse(invalidRequest.get())
        } finally {
            compose.runOnUiThread {controller.close();owner.cancel()}
            runBlocking {Connections.close(client)}
            server.close()
        }
    }
}
