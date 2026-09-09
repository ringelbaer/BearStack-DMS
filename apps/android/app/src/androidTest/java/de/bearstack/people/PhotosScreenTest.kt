package de.bearstack.people

import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Color
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import coil.ImageLoader
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.File
import java.util.Locale

class PhotosScreenTest {
    @get:Rule val compose=createComposeRule()
    private fun screen(locale: Locale, showMapSelection: Boolean = false, retryEmptyBlog: Boolean = false, test: (PhotosController,PhotosService)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(locale)})
        val file=File(app.cacheDir,"gallery-test.jpg")
        val bitmap=Bitmap.createBitmap(40,40,Bitmap.Config.ARGB_8888).apply {eraseColor(Color.rgb(82,141,106))}
        file.outputStream().use {bitmap.compress(Bitmap.CompressFormat.JPEG,90,it)};bitmap.recycle()
        val images=ImageLoader.Builder(context).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        val photos=listOf("first.jpg","second.jpg").map {Photo(it,it,"image","image/jpeg","1","2026-09-09T10:00:00Z",null,1024,400,300)}
        val api=object:PhotosService {
            var blogAttempts=0
            override suspend fun session()=PhotoSession("gallery-test",false,240,240,1280,2048,5,8)
            override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
                val folders=if(!query.recursive && query.path.isEmpty()) listOf(PhotoFolder("Holiday","Holiday",null,2,false,0,photos)) else emptyList()
                val blogs=if(query.path=="Holiday") listOf(PhotoBlog("Holiday/story.md","story.md",null,"2026-09-09T10:00:00Z")) else emptyList()
                return PhotoPage(query.path,"",1,2,false,folders.size,false,false,
                    if(query.query.isNotEmpty()) photos.filter {it.name.contains(query.query)} else if(folders.isEmpty()) photos else emptyList(),folders,blogs)
            }
            override suspend fun info(path: String)=photos.first {it.path==path}.copy(camera="Test Camera")
            override suspend fun blog(path: String): PhotoBlog {
                if(retryEmptyBlog) {
                    if(blogAttempts++==0) throw ApiFailure(404,"not_found",de.bearstack.people.text.UiText(R.string.error_missing))
                    return PhotoBlog(path,"story.md",null,"2026-09-09T10:00:00Z")
                }
                return PhotoBlog(path,"story.md",null,"2026-09-09T10:00:00Z","Story","<h2>Story</h2><p>Travel notes.</p>")
            }
            override suspend fun mapMedia(query: PhotoQuery,bounds: PhotoMapBounds,page: Int)=PhotoMapPage(2,page,page==1,listOf(photos[page-1]))
            override fun thumbnail(photo: Photo,size: Int)=file.toURI().toString()
            override fun original(photo: Photo)=file.toURI().toString()
        }
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,api,PhotoSession("gallery-test",false,240,240,1280,2048,5,8))}
        try {
            compose.setContent {CompositionLocalProvider(LocalContext provides context,LocalConfiguration provides context.resources.configuration) {
                MaterialTheme {
                    if(showMapSelection) MapPhotoSelection(controller,images,PhotoQuery(),PhotoMapMarker(52.5,13.4,2)) {}
                    else PhotosScreen(controller,images,false,{fail("reader reached people editing")},{})
                }
            }}
            compose.waitUntil(10_000) {!controller.state.value.loading}
            test(controller,api)
        } finally {compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()};file.delete()}
    }
    @Test fun textFailureOffersRetryInsideTheDialogAndEmptyTextFinishesLoading()=screen(Locale.ENGLISH,retryEmptyBlog=true) {controller,_ ->
        compose.onNodeWithText("Folders").performClick()
        compose.onNodeWithText("Holiday").performClick()
        compose.onNodeWithText("story.md").performClick()
        compose.waitUntil(10_000) {controller.state.value.blogError!=null}
        compose.onNode(hasText("This content is no longer available.") and hasAnyAncestor(isDialog())).assertIsDisplayed()
        compose.onNode(hasText("Try again") and hasAnyAncestor(isDialog())).performClick()
        compose.waitUntil(10_000) {!controller.state.value.blogLoading && controller.state.value.blogError==null}
        compose.onNodeWithText("This post contains no text.").assertIsDisplayed()
        assertNull(controller.state.value.error)
    }
    @Test fun denseMapLocationsReplacePagesAndOpenTheSharedViewer()=screen(Locale.ENGLISH,showMapSelection=true) {_,_ ->
        compose.waitUntil(10_000) {compose.onAllNodesWithContentDescription("first.jpg").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithText("Next page").performClick()
        compose.waitUntil(10_000) {compose.onAllNodesWithContentDescription("second.jpg").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("first.jpg").assertDoesNotExist()
        compose.onNodeWithText("Page 2").assertIsDisplayed()
        compose.onNodeWithText("Next page").assertIsNotEnabled()
        compose.onNodeWithContentDescription("second.jpg").performClick()
        compose.onNodeWithText("1 of 1").assertIsDisplayed()
        compose.onNodeWithContentDescription("Close").performClick()
        compose.onNodeWithText("Previous page").performClick()
        compose.waitUntil(10_000) {compose.onAllNodesWithContentDescription("first.jpg").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("second.jpg").assertDoesNotExist()
    }
    @Test fun englishGalleryOpensViewerAndInfoAndHidesPeopleEditingForReaders() = screen(Locale.ENGLISH) {_,_ ->
        compose.onNodeWithText("BearStack Photos").assertIsDisplayed()
        compose.onNodeWithContentDescription("first.jpg").performClick()
        compose.onNodeWithContentDescription("Next photo").assertIsDisplayed().performClick()
        compose.onNodeWithText("2 of 2").assertIsDisplayed()
        compose.onNodeWithContentDescription("Information").performClick()
        compose.waitUntil(10_000) {compose.onAllNodesWithText("Test Camera").fetchSemanticsNodes().isNotEmpty()}
        compose.onAllNodesWithText("second.jpg").onFirst().assertIsDisplayed()
    }
    @Test fun germanFoldersTextsAndSearchUseNativeNavigation() = screen(Locale.GERMAN) {controller,_ ->
        compose.onNodeWithText("Ordner").performClick()
        compose.onNodeWithText("Holiday").assertIsDisplayed().performClick()
        compose.onNodeWithText("Geschichten & Notizen").assertIsDisplayed()
        compose.onNodeWithText("story.md").performClick()
        compose.waitUntil(10_000) {controller.state.value.blog?.html?.isNotEmpty()==true}
        compose.onNodeWithContentDescription("Zurück").performClick()
        compose.onNodeWithText("Suchen").performClick()
        compose.onNode(hasSetTextAction()).performTextInput("second")
        compose.onNode(hasSetTextAction()).performImeAction()
        compose.waitUntil(10_000) {!controller.state.value.loading && controller.state.value.media.size==1}
        compose.onNodeWithContentDescription("second.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("first.jpg").assertDoesNotExist()
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Personen verwalten").assertDoesNotExist()
    }
    @Test fun slideshowSettingsAdvanceOnlyAfterPlaybackStarts() = screen(Locale.ENGLISH) {_,_ ->
        compose.onNodeWithContentDescription("first.jpg").performClick()
        compose.onNodeWithContentDescription("Slideshow settings").performClick()
        compose.onNodeWithText("5 seconds").performClick()
        compose.onNodeWithText("3 seconds").performClick()
        compose.onNodeWithText("Save").performClick()
        compose.onNodeWithText("1 of 2").assertIsDisplayed()
        compose.onNodeWithContentDescription("Start slideshow").performClick()
        compose.waitUntil(8_000) {compose.onAllNodesWithText("2 of 2").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("Pause slideshow").performClick()
        compose.onNodeWithContentDescription("Start slideshow").assertIsDisplayed()
    }
}
