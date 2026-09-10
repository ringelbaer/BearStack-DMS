package de.bearstack.people

import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Color
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.graphics.asAndroidBitmap
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
    private fun screen(locale: Locale, showMapSelection: Boolean = false, retryEmptyBlog: Boolean = false, retryInfo: Boolean = false, test: (PhotosController,PhotosService)->Unit) {
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
            var infoAttempts=0
            override suspend fun session()=PhotoSession("gallery-test",false,240,240,1280,2048,5,8)
            override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
                val folders=if(!query.recursive && query.path.isEmpty()) listOf(PhotoFolder("Holiday","Holiday",null,2,false,0,photos)) else emptyList()
                val blogs=if(query.path=="Holiday") listOf(PhotoBlog("Holiday/story.md","story.md",null,"2026-09-09T10:00:00Z")) else emptyList()
                return PhotoPage(query.path,"",1,2,false,folders.size,false,false,
                    if(query.query.isNotEmpty()) photos.filter {it.name.contains(query.query)} else if(folders.isEmpty()) photos else emptyList(),folders,blogs)
            }
            override suspend fun info(path: String): Photo {
                if(retryInfo && infoAttempts++==0) throw ApiFailure(404,"not_found",de.bearstack.people.text.UiText(R.string.error_missing))
                return photos.first {it.path==path}.copy(camera="Test Camera",captured="2024-01-02T00:15:30+14:00",
                    rating=4.5,people=listOf("Alex","Sam"),tags=listOf("Holiday"),keywords=listOf("Sea","Sun"))
            }
            override suspend fun blog(path: String): PhotoBlog {
                if(retryEmptyBlog) {
                    if(blogAttempts++==0) throw ApiFailure(404,"not_found",de.bearstack.people.text.UiText(R.string.error_missing))
                    return PhotoBlog(path,"story.md",null,"2026-09-09T10:00:00Z")
                }
                return PhotoBlog(path,"story.md",null,"2026-09-09T10:00:00Z","Story","<h2>Story</h2><p>Travel notes.</p>")
            }
            override suspend fun mapMedia(query: PhotoQuery,bounds: PhotoMapBounds,page: Int)=PhotoMapPage(2,page,false,photos)
            override fun thumbnail(photo: Photo,size: Int)=file.toURI().toString()
            override fun original(photo: Photo)=file.toURI().toString()
        }
        lateinit var controller:PhotosController
        compose.runOnUiThread {controller=PhotosController(owner,api,PhotoSession("gallery-test",false,240,240,1280,2048,5,8))}
        try {
            compose.setContent {
                val registry = checkNotNull(LocalActivityResultRegistryOwner.current)
                CompositionLocalProvider(LocalActivityResultRegistryOwner provides registry, LocalContext provides context,LocalResources provides context.resources,LocalConfiguration provides context.resources.configuration) {
                MaterialTheme {
                    if(showMapSelection) MapPhotoSelection(controller,images,PhotoQuery(),PhotoMapMarker(52.5,13.4,2)) {}
                    else PhotosScreen(controller,images,false,{fail("reader reached people editing")},{})
                }
            }}
            compose.waitUntil(10_000) {!controller.state.value.loading}
            test(controller,api)
        } finally {compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()};file.delete()}
    }
    @Test fun photoInfoRetriesAndShowsSourceTimeAndMetadataInEnglish()=infoRetry(Locale.ENGLISH)
    @Test fun photoInfoRetriesAndShowsSourceTimeAndMetadataInGerman()=infoRetry(Locale.GERMAN)
    private fun infoRetry(locale: Locale)=screen(locale,retryInfo=true) {_,_ ->
        val german=locale==Locale.GERMAN
        compose.onNodeWithContentDescription("first.jpg").performClick()
        compose.onNodeWithContentDescription(if(german) "Informationen" else "Information").performClick()
        compose.onNodeWithText(if(german) "Erneut versuchen" else "Try again").performClick()
        compose.waitUntil(10_000) {compose.onAllNodesWithText("Test Camera").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithText(if(german) "Aufnahmezeit" else "Capture time").assertIsDisplayed()
        compose.onNodeWithText("UTC+14:00",substring=true).assertIsDisplayed()
        compose.onNodeWithText(if(german) "1,02" else "1.02",substring=true).assertIsDisplayed()
        val bitmap=compose.onNode(hasScrollAction() and hasAnyDescendant(hasText("UTC+14:00",substring=true))).captureToImage().asAndroidBitmap()
        File(InstrumentationRegistry.getInstrumentation().targetContext.cacheDir,"photo-info-${locale.language}.png")
            .outputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}
        compose.onNodeWithText(if(german) "4,5 / 5 Sterne" else "4.5 / 5 stars").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Alex · Sam").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Sea · Sun").performScrollTo().assertIsDisplayed()
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
    @Test fun mapSelectionKeepsPhotosInOneGridAndOpensTheSharedViewer()=screen(Locale.ENGLISH,showMapSelection=true) {_,_ ->
        compose.waitUntil(10_000) {compose.onAllNodesWithContentDescription("second.jpg").fetchSemanticsNodes().isNotEmpty()}
        compose.onNodeWithContentDescription("first.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("second.jpg").assertIsDisplayed()
        compose.onNodeWithText("Next page").assertDoesNotExist()
        compose.onNodeWithText("Previous page").assertDoesNotExist()
        compose.onNodeWithContentDescription("second.jpg").performClick()
        compose.onNodeWithText("2 of 2").assertIsDisplayed()
        compose.onNodeWithContentDescription("Previous photo").performClick()
        compose.onNodeWithText("1 of 2").assertIsDisplayed()
        compose.onNodeWithContentDescription("Close").performClick()
        compose.onNodeWithContentDescription("first.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("second.jpg").assertIsDisplayed()
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
        compose.onNode(hasContentDescription("Zurück") and hasAnyAncestor(isDialog())).performClick()
        assertNull(controller.state.value.blog)
        assertEquals("Holiday",controller.state.value.query.path)
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
