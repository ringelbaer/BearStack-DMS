package de.bearstack.people

import android.graphics.Bitmap
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.PhotosController
import de.bearstack.people.photos.PhotosScreen
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.io.IOException

class GalleryPagingTest {
    @get:Rule val compose=createComposeRule()
    private val session=PhotoSession("paging",true,240,240,1280,2048,3,3)
    private fun photo(index: Int)=Photo("image-$index","image-$index","image","image/jpeg","1",
        "2026-09-09T10:00:00Z",null,100,80,80)
    private inner class Service(private val image: File): PhotosService {
        var failPage=0
        var pageGate: CompletableDeferred<Unit>? = null
        val requests=mutableListOf<Pair<String,Int>>()
        override suspend fun session()=session
        override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
            requests+=section to page
            delay(50)
            if(page>1 || section.isNotEmpty()) pageGate?.await()
            if(page==failPage) throw IOException("offline")
            val folders=!query.recursive && query.path.isEmpty()
            val blogs=query.path=="Texts"
            val media=if(!folders && !blogs) List(96) {photo((page-1)*96+it)} else emptyList()
            return PhotoPage(query.path,"",page,if(media.isEmpty()) 0 else 480,!media.isEmpty() && page<5,
                if(folders) 120 else 0,folders && page<5,blogs && page<5,media,
                if(folders) List(24) {PhotoFolder("folder-${(page-1)*24+it}","Folder ${(page-1)*24+it}",null,2,false,0,listOf(photo(0),photo(1)))} else emptyList(),
                if(blogs) List(20) {PhotoBlog("post-${(page-1)*20+it}","Post ${(page-1)*20+it}",null,"2026-09-09T10:00:00Z")} else emptyList())
        }
        override suspend fun info(path: String)=photo(path.removePrefix("image-").toInt())
        override suspend fun blog(path: String)=PhotoBlog(path,path,null,"2026-09-09T10:00:00Z",text="Text",html="<p>Text</p>")
        override fun thumbnail(photo: Photo,size: Int)=image.toURI().toString()
        override fun original(photo: Photo)=image.toURI().toString()
    }
    private fun screen(test: (PhotosController,Service)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val image=File(app.cacheDir,"paging-test.jpg")
        Bitmap.createBitmap(80,80,Bitmap.Config.ARGB_8888).apply {
            eraseColor(android.graphics.Color.rgb(72,130,105))
            image.outputStream().use {compress(Bitmap.CompressFormat.JPEG,80,it)}
            recycle()
        }
        val images=ImageLoader.Builder(app).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        val api=Service(image)
        lateinit var controller:PhotosController
        var gallery by mutableStateOf(true)
        compose.runOnUiThread {controller=PhotosController(owner,api,session)}
        try {
            compose.setGermanContent {MaterialTheme {
                if(gallery) PhotosScreen(controller,images,true,{gallery=false},{})
                else TextButton(onClick={gallery=true}) {Text("Zur Galerie")}
            }}
            compose.waitUntil(10_000) {!controller.state.value.loading}
            test(controller,api)
        } catch(e: Throwable) {
            val state=controller.state.value
            val captions=runCatching {compose.onAllNodes(hasText("480",substring=true)).fetchSemanticsNodes().map {it.config[androidx.compose.ui.semantics.SemanticsProperties.Text]}}.getOrNull()
            throw AssertionError("Selected=${state.selected}, captions=$captions. Pages ${state.mediaPages.firstPage}..${state.mediaPages.lastPage}, loading=${state.loadingSections}, errors=${state.pageErrors}, requests=${api.requests.takeLast(12)}",e)
        } finally {compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()};image.delete()}
    }
    private fun scroll(key: String)=compose.onNodeWithTag("photo-gallery").performScrollToKey(key)

    @Test fun scrollingDuringASlowRequestNeverRewindsOrEvictsTheNewViewport()=screen {controller,api ->
        for(page in 2..3) {
            compose.runOnUiThread {controller.more("media")}
            compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==page}
        }
        api.pageGate=CompletableDeferred()
        scroll("photo:image-287")
        compose.waitUntil(10_000) {"media" in controller.state.value.loadingSections}
        scroll("photo:image-150")
        val before=compose.onNodeWithContentDescription("image-150").fetchSemanticsNode().boundsInRoot.top
        api.pageGate!!.complete(Unit)
        compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==4}
        assertEquals(before,compose.onNodeWithContentDescription("image-150").fetchSemanticsNode().boundsInRoot.top,1f)
        api.pageGate=CompletableDeferred()
        scroll("photo:image-383")
        compose.waitUntil(10_000) {"media" in controller.state.value.loadingSections}
        scroll("photo:image-96")
        val earlier=compose.onNodeWithContentDescription("image-96").fetchSemanticsNode().boundsInRoot.top
        api.pageGate!!.complete(Unit)
        compose.waitUntil(10_000) {controller.state.value.loadingSections.isEmpty()}
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        assertEquals(4,controller.state.value.mediaPages.lastPage)
        assertEquals(earlier,compose.onNodeWithContentDescription("image-96").fetchSemanticsNode().boundsInRoot.top,1f)
    }

    @Test fun addingAndRemovingPagesKeepsTheVisiblePhotoAtTheSamePixelPosition()=screen {controller,api ->
        for(page in 2..3) {
            compose.runOnUiThread {controller.more("media")}
            compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==page}
        }
        api.pageGate=CompletableDeferred()
        scroll("photo:image-287")
        compose.waitUntil(10_000) {"media" in controller.state.value.loadingSections}
        val before=compose.onNodeWithContentDescription("image-287").fetchSemanticsNode().boundsInRoot.top
        api.pageGate!!.complete(Unit)
        compose.waitUntil(10_000) {controller.state.value.mediaPages.firstPage==2}
        val after=compose.onNodeWithContentDescription("image-287").fetchSemanticsNode().boundsInRoot.top
        assertEquals(before,after,1f)
        api.pageGate=CompletableDeferred()
        scroll("previous-media")
        compose.waitUntil(10_000) {"media" in controller.state.value.loadingSections}
        val earlierBefore=compose.onNodeWithContentDescription("image-96").fetchSemanticsNode().boundsInRoot.top
        api.pageGate!!.complete(Unit)
        compose.waitUntil(10_000) {controller.state.value.mediaPages.firstPage==1}
        val earlierAfter=compose.onNodeWithContentDescription("image-96").fetchSemanticsNode().boundsInRoot.top
        assertEquals(earlierBefore,earlierAfter,1f)
    }

    @Test fun viewerKeepsItsVisiblePhotoWhenBackgroundLoadingEvictsAnEarlierPage()=screen {controller,_ ->
        for(page in 2..3) {
            compose.runOnUiThread {controller.more("media")}
            compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==page}
        }
        // Select through the same controller action as a tile tap, without
        // scrolling the grid to its loading boundary before opening the viewer.
        compose.runOnUiThread {controller.select("image-280")}
        compose.waitUntil(10_000) {controller.state.value.mediaPages.firstPage==2}
        compose.onNodeWithText("281 von 480").assertIsDisplayed()
        for(position in 282..289) {
            compose.onNodeWithContentDescription("Nächstes Foto").performClick()
            compose.onNodeWithText("$position von 480").assertIsDisplayed()
        }
        assertEquals("image-288",controller.state.value.selected)
        assertEquals(288,controller.state.value.media.size)
        compose.onNodeWithContentDescription("Schließen").performClick()
        compose.onNodeWithContentDescription("image-288").assertIsDisplayed()
    }

    @Test fun evictedPhotosReloadBackwardsAndViewerKeepsThePhotoAndAbsolutePosition()=screen {controller,_ ->
        for(page in 1..4) {
            scroll("photo:image-${page*96-1}")
            compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==page+1}
            compose.onNodeWithContentDescription("image-${page*96-1}").assertIsDisplayed()
            assertTrue(controller.state.value.media.size<=288)
        }
        assertEquals(3,controller.state.value.mediaPages.firstPage)
        compose.onNodeWithContentDescription("image-383").performClick()
        compose.onNodeWithText("384 von 480").assertIsDisplayed()
        compose.onNodeWithContentDescription("Nächstes Foto").performClick()
        compose.onNodeWithText("385 von 480").assertIsDisplayed()
        compose.onNodeWithContentDescription("Vorheriges Foto").performClick()
        compose.onNodeWithText("384 von 480").assertIsDisplayed()
        compose.onNodeWithContentDescription("Schließen").performClick()
        compose.onNodeWithContentDescription("image-383").assertIsDisplayed()
        scroll("previous-media")
        compose.waitUntil(10_000) {controller.state.value.mediaPages.firstPage==2}
        compose.onNodeWithContentDescription("image-192").assertIsDisplayed()
        compose.waitForIdle()
        assertEquals(2,controller.state.value.mediaPages.firstPage)
        scroll("previous-media")
        compose.waitUntil(10_000) {controller.state.value.mediaPages.firstPage==1}
        compose.onNodeWithContentDescription("image-96").assertIsDisplayed()
        assertEquals(288,controller.state.value.media.size)
    }

    @Test fun failedAppendRetriesInPlaceAndGalleryPositionSurvivesLeavingTheScreen()=screen {controller,api ->
        api.failPage=2
        scroll("photo:image-95")
        compose.waitUntil(10_000) {"media" in controller.state.value.pageErrors}
        compose.onNodeWithContentDescription("image-95").assertIsDisplayed()
        assertEquals(96,controller.state.value.media.size)
        api.failPage=0
        compose.onNodeWithText("Erneut versuchen").performScrollTo().assertIsDisplayed().performClick()
        compose.waitUntil(10_000) {controller.state.value.mediaPages.lastPage==2}
        compose.onNodeWithContentDescription("image-95").assertIsDisplayed()
        assertEquals(listOf(1,2,2),api.requests.map {it.second})
        scroll("photo:image-150")
        compose.onNodeWithContentDescription("image-150").assertIsDisplayed()
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Personen verwalten").performClick()
        compose.onNodeWithText("Zur Galerie").performClick()
        compose.onNodeWithContentDescription("image-150").assertIsDisplayed()
        assertEquals(3,api.requests.size)
    }

    @Test fun foldersAndTextPostsAlsoEvictAndReloadTheirOwnPages()=screen {controller,_ ->
        compose.onNodeWithText("Ordner").performClick()
        compose.waitUntil(10_000) {!controller.state.value.loading}
        for(page in 1..4) {
            scroll("folder:folder-${page*24-1}")
            compose.waitUntil(10_000) {controller.state.value.folderPages.lastPage==page+1}
            compose.onNodeWithText("Folder ${page*24-1}").assertIsDisplayed()
            assertTrue(controller.state.value.folders.size<=72)
        }
        scroll("previous-folders")
        compose.waitUntil(10_000) {controller.state.value.folderPages.firstPage==2}
        compose.onNodeWithText("Folder 48").assertIsDisplayed()
        compose.runOnUiThread {controller.open(PhotoQuery(path="Texts"))}
        compose.waitUntil(10_000) {!controller.state.value.loading}
        for(page in 1..4) {
            scroll("blog:post-${page*20-1}")
            compose.waitUntil(10_000) {controller.state.value.blogPages.lastPage==page+1}
            compose.onNodeWithText("Post ${page*20-1}").assertIsDisplayed()
            assertTrue(controller.state.value.blogs.size<=60)
        }
        scroll("previous-blogs")
        compose.waitUntil(10_000) {controller.state.value.blogPages.firstPage==2}
        compose.onNodeWithText("Post 40").assertIsDisplayed()
    }
}
