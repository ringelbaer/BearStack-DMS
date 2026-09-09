package de.bearstack.people

import android.content.res.Configuration
import android.graphics.Bitmap
import androidx.compose.foundation.layout.*
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import coil.EventListener
import coil.request.ImageRequest
import coil.request.SuccessResult
import coil.request.ErrorResult
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.PhotoMap
import de.bearstack.people.photos.mapTileClient
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.Locale
import java.util.concurrent.atomic.AtomicReference

class PhotoMapTest {
    @get:Rule val compose=createComposeRule()
    private fun map(locale: Locale) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        // Manual visual acceptance can exercise the public production tile client;
        // the regular regression suite remains independent of OSM availability.
        val liveTiles=InstrumentationRegistry.getArguments().getString("visualLiveMap")=="true"
        val loaded=java.util.concurrent.ConcurrentHashMap.newKeySet<String>()
        val tileError=AtomicReference<Throwable>()
        val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(locale)})
        val bitmap=Bitmap.createBitmap(256,256,Bitmap.Config.ARGB_8888).apply {eraseColor(0xffe3ecd9.toInt())}
        val bytes=ByteArrayOutputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it);it.toByteArray()}
        bitmap.recycle()
        val liveCache=if(liveTiles) Cache(File(app.cacheDir,"live-map-test"),64L*1024*1024) else null
        val proxyPort=InstrumentationRegistry.getArguments().getString("visualProxyPort")?.toInt()
        val client=if(liveCache!=null) mapTileClient(liveCache).newBuilder().apply {
            // An isolated AVD can use a host CONNECT tunnel for this manual test.
            // The tunnel does not terminate TLS or change certificate validation.
            if(proxyPort!=null) proxy(java.net.Proxy(java.net.Proxy.Type.HTTP,java.net.InetSocketAddress("127.0.0.1",proxyPort)))
        }.build() else OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .header("Cache-Control","max-age=60").body(bytes.toResponseBody("image/png".toMediaType())).build()
        }.build()
        val images=ImageLoader.Builder(context).okHttpClient(client).diskCache(null)
            .eventListener(object:EventListener {
                override fun onSuccess(request: ImageRequest,result: SuccessResult) {loaded.add(request.data.toString())}
                override fun onError(request: ImageRequest,result: ErrorResult) {tileError.set(result.throwable)}
            }).build()
        val viewport=AtomicReference<PhotoMapBounds>()
        val selected=AtomicReference<String>()
        val isEnglish=locale==Locale.ENGLISH
        try {
            compose.setContent {CompositionLocalProvider(LocalContext provides context,LocalResources provides context.resources,LocalConfiguration provides context.resources.configuration) {
                MaterialTheme {
                    PhotoMap(PhotoMapBounds(52.50,13.38,52.58,13.48),
                        listOf(PhotoMapMarker(52.52,13.405,1,"berlin.jpg"),PhotoMapMarker(52.55,13.45,12),
                            PhotoMapMarker(52.54,13.40,8,bounds=PhotoMapBounds(52.54,13.40,52.54,13.40))),
                        Modifier.fillMaxWidth().height(480.dp),onMarker={selected.set(it.path.ifBlank {"cluster:${it.count}"})},onViewport={viewport.set(it)},tileImages=images)
                }
            }}
            compose.waitUntil(10_000) {viewport.get()!=null}
            compose.onNodeWithText("© OpenStreetMap contributors").assertIsDisplayed()
            compose.onNodeWithContentDescription(if(isEnglish) "1 item at this location" else "1 Aufnahme an diesem Ort").performClick()
            assertEquals("berlin.jpg",selected.get())
            compose.onNodeWithContentDescription(if(isEnglish) "8 items at this location" else "8 Aufnahmen an diesem Ort").performClick()
            assertEquals("cluster:8",selected.get())
            val initial=viewport.get()
            compose.onNodeWithContentDescription(context.getString(R.string.photos_map_zoom_in)).performClick()
            compose.waitUntil(5_000) {viewport.get()!=initial}
            if(liveTiles) {
                compose.waitUntil(30_000) {loaded.isNotEmpty() || tileError.get()!=null}
                assertTrue("No live tile decoded: ${tileError.get()}",loaded.isNotEmpty())
            }
            assertTrue(viewport.get().north-viewport.get().south<initial.north-initial.south)
            compose.onNodeWithContentDescription(context.getString(R.string.photos_map_fit)).performClick()
            compose.waitUntil(5_000) {viewport.get()==initial}
            compose.onNodeWithContentDescription(if(isEnglish) "12 items at this location" else "12 Aufnahmen an diesem Ort").performClick()
            compose.waitUntil(5_000) {viewport.get()!=initial}
            compose.onRoot().captureToImage().asAndroidBitmap().let {shot ->
                File(app.cacheDir,"map-${locale.language}.png").outputStream().use {shot.compress(Bitmap.CompressFormat.PNG,100,it)}
            }
        } finally {images.shutdown();client.dispatcher.executorService.shutdown();client.connectionPool.evictAll();liveCache?.close()}
    }
    @Test fun englishMapSupportsZoomMarkersAndVisibleAttribution()=map(Locale.ENGLISH)
    @Test fun germanMapSupportsZoomMarkersAndVisibleAttribution()=map(Locale.GERMAN)
}
