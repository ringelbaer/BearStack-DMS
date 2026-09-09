package de.bearstack.people

import android.graphics.Bitmap
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.espresso.Espresso.pressBack
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.Locale

class MapTracksScreenTest {
    @get:Rule val compose=createComposeRule()
    private val extent=PhotoMapBounds(52.49,13.38,52.51,13.44)
    private inner class Source: PhotosService {
        val loaded=mutableSetOf<String>()
        val files=listOf("Hike.gpx","Ride.gpx","Broken.gpx").map {PhotoTrackFile(it,it,"2026-09-09T10:00:00Z",100)}
        override suspend fun session()=PhotoSession("tracks",false,240,240,1280,2048,5,8)
        override suspend fun browse(query: PhotoQuery,page: Int,section: String)=PhotoPage("","",page,0,false,0,false,false,emptyList(),emptyList(),emptyList())
        override suspend fun info(path: String): Photo=throw UnsupportedOperationException()
        override suspend fun blog(path: String): PhotoBlog=throw UnsupportedOperationException()
        override fun thumbnail(photo: Photo,size: Int)=""
        override fun original(photo: Photo)=""
        override suspend fun map(query: PhotoQuery,bounds: PhotoMapBounds?)=PhotoMapData(0,null,emptyList())
        override suspend fun tracks(path: String,cursor: String,before: Boolean)=PhotoTrackPage(files,"3","1",false,false,true)
        override suspend fun route(query:PhotoQuery,bounds:PhotoMapBounds?,points:Int):PhotoRouteData {
            loaded+="photo-route"
            return PhotoRouteData(PhotoTrackGeometry("","",extent,listOf(listOf(
                PhotoMapPoint(52.5,13.385),PhotoMapPoint(52.5,13.435))),2,false,0),5,1000)
        }
        override suspend fun track(path: String,bounds: PhotoMapBounds?,points: Int): PhotoTrackGeometry {
            delay(30)
            if(path=="Broken.gpx") throw ApiFailure(422,"invalid_gpx",de.bearstack.people.text.UiText(R.string.photos_track_error))
            loaded+=path
            val lat=if(path=="Hike.gpx") 52.495 else 52.505
            return PhotoTrackGeometry(path,path,extent,listOf(
                listOf(PhotoMapPoint(lat,13.385),PhotoMapPoint(lat,13.400)),
                listOf(PhotoMapPoint(lat,13.420),PhotoMapPoint(lat,13.435))),4,false,0)
        }
    }
    @Test fun germanTrackOnlyMapShowsMultiplePathsAndErrors()=screen(Locale.GERMAN)
    @Test fun englishTrackOnlyMapShowsMultiplePathsAndErrors()=screen(Locale.ENGLISH)
    private fun screen(locale: Locale) {
        val english=locale==Locale.ENGLISH
        val layers=if(english) "Layers" else "Ebenen"
        val map=if(english) "Map" else "Karte"
        val error=if(english) "This track could not be read." else "Dieser Track konnte nicht gelesen werden."
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val tile=Bitmap.createBitmap(256,256,Bitmap.Config.ARGB_8888).apply {eraseColor(0xffecece6.toInt())}
        val bytes=ByteArrayOutputStream().use {tile.compress(Bitmap.CompressFormat.PNG,100,it);it.toByteArray()};tile.recycle()
        val client=OkHttpClient.Builder().addInterceptor {chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1).code(200).message("OK")
                .header("Cache-Control","max-age=60").body(bytes.toResponseBody("image/png".toMediaType())).build()
        }.build()
        val images=ImageLoader.Builder(app).okHttpClient(client).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        val source=Source()
        lateinit var controller: PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,source,PhotoSession("tracks",false,240,240,1280,2048,5,8))}
        try {
            compose.setLocalizedContent(locale) {MaterialTheme {FolderMap(controller,images,PhotoQuery(),tileImages=images,onClose={})}}
            compose.onNodeWithText(layers).performClick()
            compose.onNodeWithText(if(english) "GPX tracks" else "GPX-Tracks").assertIsDisplayed()
            compose.onNodeWithTag("map-layer-list").performScrollToNode(hasText("Hike.gpx"))
            compose.onNodeWithText("Hike.gpx").performClick()
            compose.waitUntil(10_000) {"Hike.gpx" in source.loaded}
            compose.onNodeWithTag("map-layer-list").performScrollToNode(hasText("Ride.gpx"))
            compose.onNodeWithText("Ride.gpx").performClick()
            compose.waitUntil(10_000) {"Ride.gpx" in source.loaded}
            val routeTitle=if(english) "Photo route" else "Fotoroute"
            compose.onNodeWithTag("map-layer-list").performScrollToNode(hasText(routeTitle))
            compose.onNodeWithText(routeTitle).performClick()
            compose.waitUntil(10_000) {"photo-route" in source.loaded}
            pressBack()
            compose.onNodeWithContentDescription(map).assertIsDisplayed()
            compose.waitForIdle()
            val bitmap=compose.onNodeWithContentDescription(map).captureToImage().asAndroidBitmap()
            for(name in listOf("Hike.gpx","Ride.gpx")) {
                val color=trackColor(name).toArgb()
                var longest=0;var lineY=0;var left=0;var right=0
                for(y in 0 until bitmap.height) {
                    var count=0;var first=-1;var last=-1
                    for(x in 0 until bitmap.width) if(bitmap.getPixel(x,y)==color) {
                        if(first<0) first=x
                        last=x;count++
                    }
                    if(count>longest) {longest=count;lineY=y;left=first;right=last}
                }
                assertTrue("Track $name has no visible geometry ($longest pixels)",longest>20)
                assertNotEquals("Track $name bridges the recorded gap",color,bitmap.getPixel((left+right)/2,lineY))
            }
            val routeColor=trackColor(photoRoutePath).toArgb()
            var maxRuns=0
            for(y in 0 until bitmap.height) {
                var runs=0;var previous=false
                for(x in 0 until bitmap.width) {
                    val current=bitmap.getPixel(x,y)==routeColor
                    if(current && !previous) runs++
                    previous=current
                }
                maxRuns=maxOf(maxRuns,runs)
            }
            assertTrue("Photo route is not visibly dashed",maxRuns>=4)
            File(app.cacheDir,"native-gpx-map-${locale.language}.png").outputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}
            bitmap.recycle()
            compose.onNodeWithText(layers).performClick()
            compose.onNodeWithTag("map-layer-list").performScrollToNode(hasText("Broken.gpx"))
            compose.onNodeWithText("Broken.gpx").performClick()
            compose.waitUntil(10_000) {compose.onAllNodesWithText(error).fetchSemanticsNodes().isNotEmpty()}
            compose.onNodeWithText(if(english) "Hide all GPX tracks" else "Alle GPX-Tracks ausblenden").performClick()
            compose.onNodeWithTag("map-layer-list").performScrollToNode(hasText(routeTitle))
            compose.onNodeWithText(routeTitle).performClick()
            pressBack()
            compose.onNodeWithText(layers).assertIsDisplayed()
        } finally {
            compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()}
            client.dispatcher.executorService.shutdown();client.connectionPool.evictAll()
        }
    }
}
