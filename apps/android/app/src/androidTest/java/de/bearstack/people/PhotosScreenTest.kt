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
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.junit4.v2.createComposeRule
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
    @Test fun longPressStartsSelectionAndShortTapsToggleWithoutOpeningViewer() = screen(Locale.ENGLISH) {controller,_ ->
        compose.onNodeWithContentDescription("first.jpg").performTouchInput {longClick()}
        compose.onNodeWithText("1 / 100 selected").assertIsDisplayed()
        compose.onNodeWithContentDescription("second.jpg").performClick()
        compose.onNodeWithText("2 / 100 selected").assertIsDisplayed()
        compose.onNodeWithContentDescription("first.jpg").performClick()
        assertEquals(setOf("second.jpg"),controller.state.value.selection.keys)
        compose.onNodeWithTag("photo-viewer-image").assertDoesNotExist()
        compose.onNodeWithContentDescription("Delete").assertDoesNotExist()
        compose.onNodeWithContentDescription("Clear selection").performClick()
        compose.onNodeWithContentDescription("second.jpg").performClick()
        compose.onNodeWithTag("photo-viewer-image").assertIsDisplayed()
    }
    @Test fun mapThumbnailSelectionOffersShareAndSaveButNeverDelete() = screen(Locale.ENGLISH,showMapSelection=true) {_,_ ->
        compose.onNodeWithContentDescription("first.jpg").performTouchInput {longClick()}
        compose.onNodeWithContentDescription("second.jpg").performClick()
        compose.onNodeWithText("2 / 100 selected").assertIsDisplayed()
        compose.onNodeWithContentDescription("Share").assertIsEnabled()
        compose.onNodeWithContentDescription("Save selection").assertIsEnabled()
        compose.onNodeWithContentDescription("Delete").assertDoesNotExist()
    }
    private fun screen(locale: Locale, showMapSelection: Boolean = false, retryEmptyBlog: Boolean = false, retryInfo: Boolean = false, peopleFolders: Boolean = false, directoryPeople: Boolean = false, peopleCountSort: Boolean = true, frameRandomSort: Boolean = true, photoCount: Int = 2, fontScale: Float = 1f, beforeBrowse: suspend (PhotoQuery)->Unit = {}, test: (PhotosController,PhotosService)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(locale)})
        val file=File(app.cacheDir,"gallery-test.jpg")
        val bitmap=Bitmap.createBitmap(40,40,Bitmap.Config.ARGB_8888).apply {eraseColor(Color.rgb(82,141,106))}
        file.outputStream().use {bitmap.compress(Bitmap.CompressFormat.JPEG,90,it)};bitmap.recycle()
        val images=ImageLoader.Builder(context).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        val photos=(listOf("first.jpg","second.jpg")+(2 until photoCount).map {"photo-$it.jpg"})
            .map {Photo(it,it,"image","image/jpeg","1","2026-09-09T10:00:00Z",null,1024,400,300,folderName=if(it=="first.jpg") "Sommer Urlaub" else "Fotos")}
        val api=object:PhotosService {
            var blogAttempts=0
            var infoAttempts=0
            override suspend fun session()=PhotoSession("gallery-test",false,240,240,1280,2048,5,8,peopleCountSort,frameRandomSort)
            override suspend fun browse(query: PhotoQuery,page: Int,section: String): PhotoPage {
                beforeBrowse(query)
                if(directoryPeople && query.path.startsWith(".people/f-")) {
                    val leaf=query.path.endsWith("/1")
                    val folders=if(leaf) emptyList() else listOf(PhotoFolder(".people/f-SG9saWRheQ/1","Ada",null,1,false,0,listOf(photos[0].copy(faceId=1)),true))
                    return PhotoPage(query.path,if(leaf) ".people/f-SG9saWRheQ" else "Holiday",1,if(leaf) 1 else 0,false,folders.size,false,false,
                        if(leaf) photos.take(1) else emptyList(),folders,emptyList(),if(leaf) "Ada" else "Personen im Ordner")
                }
                if(peopleFolders && !query.recursive) {
                    val portraits=List(8) { photos[0].copy(name="Gesicht $it",faceId=it+1L) }
                    val folders=when(query.path) {
                        "" -> listOf(PhotoFolder(".people","Personen",null,0,false,8,portraits,true))
                        ".people" -> listOf(PhotoFolder(".people/all","Alle",null,0,false,8,portraits,true),PhotoFolder(".people/t-ZmFtaWxpZQ","Familie",null,0,false,1,portraits.take(1),true))
                        ".people/all", ".people/t-ZmFtaWxpZQ" -> listOf(PhotoFolder("${query.path}/1","Zoe",null,2,false,0,portraits.take(1),true))
                        else -> emptyList()
                    }
                    val name=when(query.path) {".people" -> "Personen"; ".people/all" -> "Alle"; ".people/t-ZmFtaWxpZQ" -> "Familie"; else -> "Zoe"}
                    return PhotoPage(query.path,query.path.substringBeforeLast('/',""),1,if(folders.isEmpty()) 2 else 0,false,folders.size,false,false,
                        if(folders.isEmpty()) if(query.sort=="ascending_date") photos else photos.reversed() else emptyList(),folders,emptyList(),name)
                }
                val folders=if(!query.recursive && query.path.isEmpty()) listOf(PhotoFolder("Holiday","Holiday",null,2,false,0,photos)) else emptyList()
                val blogs=if(query.path=="Holiday") listOf(PhotoBlog("Holiday/story.md","story.md",null,"2026-09-09T10:00:00Z")) else emptyList()
                return PhotoPage(query.path,"",1,photos.size,false,folders.size,false,false,
                    if(query.query.isNotEmpty()) photos.filter {it.name.contains(query.query)} else if(folders.isEmpty()) if(query.sort=="random") photos.reversed() else photos else emptyList(),folders,blogs,peoplePath=if(directoryPeople && query.path=="Holiday") ".people/f-SG9saWRheQ" else "")
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
        compose.runOnUiThread {controller=PhotosController(owner,api,PhotoSession("gallery-test",false,240,240,1280,2048,5,8,peopleCountSort,frameRandomSort))}
        try {
            compose.setContent {
                val registry = checkNotNull(LocalActivityResultRegistryOwner.current)
                CompositionLocalProvider(LocalActivityResultRegistryOwner provides registry, LocalContext provides context,LocalResources provides context.resources,LocalConfiguration provides context.resources.configuration,
                    LocalDensity provides Density(LocalDensity.current.density,fontScale)) {
                MaterialTheme {
                    if(showMapSelection) MapPhotoSelection(controller,images,PhotoQuery(),PhotoMapMarker(52.5,13.4,2)) {}
                    else PhotosScreen(controller,images,false,{fail("reader reached people editing")},{})
                }
            }}
            compose.waitUntil(10_000) {!controller.state.value.loading}
            test(controller,api)
        } finally {compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()};file.delete()}
    }
    @Test fun galleryScrollsBehindFloatingNavigation() = checkFloatingNavigation(Locale.ENGLISH,1f)

    @Test fun floatingNavigationKeepsLastRowReachableWithLargeFont() = checkFloatingNavigation(Locale.GERMAN,2f)

    private fun checkFloatingNavigation(locale: Locale,fontScale: Float) = screen(locale,photoCount=60,fontScale=fontScale) {controller,_ ->
        val navigation=compose.onNodeWithTag("gallery-navigation")
        val gallery=compose.onNodeWithTag("photo-gallery")
        val labels=if(locale==Locale.GERMAN) listOf("Fotos","Ordner","Suchen") else listOf("Photos","Folders","Search")
        compose.onRoot().captureToImage().asAndroidBitmap().let { shot ->
            File(InstrumentationRegistry.getInstrumentation().targetContext.cacheDir,"gallery-navigation-$fontScale.png")
                .outputStream().use {shot.compress(Bitmap.CompressFormat.PNG,100,it)}
        }
        val bounds=navigation.getUnclippedBoundsInRoot()
        if(fontScale==1f) assertTrue("Compact bar",bounds.bottom-bounds.top<=57.dp)
        labels.forEachIndexed { index,label ->
            val icon=compose.onNodeWithTag("gallery-navigation-icon-$index",useUnmergedTree=true).getUnclippedBoundsInRoot()
            val labelNode=compose.onNodeWithText(label,useUnmergedTree=true)
            val labelBounds=labelNode.getUnclippedBoundsInRoot()
            labelNode.performSemanticsAction(SemanticsActions.GetTextLayoutResult) { action ->
                val layout=mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
                action(layout)
                assertEquals("Navigation label should stay on one line",1,layout.single().lineCount)
            }
            assertTrue("Icon must sit left of label",icon.right<=labelBounds.left)
            assertEquals(18f,(icon.right-icon.left).value,.5f)
            compose.onNodeWithText(label).assertIsDisplayed().performClick()
            compose.waitUntil { !controller.state.value.loading && controller.state.value.tab==index }
            compose.onNodeWithText(label).assertIsSelected()
            val viewport=gallery.getUnclippedBoundsInRoot()
            assertTrue("The grid must extend behind and below navigation",viewport.bottom>bounds.bottom)
            assertTrue("Navigation must float with side margins",bounds.left>viewport.left && bounds.right<viewport.right)
            assertEquals(bounds,navigation.getUnclippedBoundsInRoot())
        }
        // The empty search contains all 60 photos and one date heading. Scroll
        // to the end so the last row clears the overlay, including large text.
        gallery.performScrollToIndex(60)
        val last=compose.onNodeWithContentDescription("photo-59.jpg")
        last.assertIsDisplayed()
        assertTrue("Last row must be fully above navigation",last.getUnclippedBoundsInRoot().bottom<=bounds.top)
        assertEquals(bounds,navigation.getUnclippedBoundsInRoot())
        last.performClick()
        compose.waitUntil {controller.state.value.selected=="photo-59.jpg"}
    }
    @Test fun peopleTitlesStayReadableThroughoutSlowForwardBackAndSortRequests() {
        var gate=CompletableDeferred<Unit>()
        screen(Locale.GERMAN,peopleFolders=true,beforeBrowse={if(it.path.isNotEmpty()) gate.await()}) {controller,_ ->
            compose.onNodeWithText("Ordner").performClick()
            fun loadingTitle(title: String, raw: String) {
                compose.waitUntil {controller.state.value.loading}
                compose.onNodeWithText(title).assertIsDisplayed()
                compose.onNodeWithText(raw).assertDoesNotExist()
                gate.complete(Unit)
                compose.waitUntil { !controller.state.value.loading }
                gate=CompletableDeferred()
            }
            compose.onNodeWithText("Personen").performClick();loadingTitle("Personen",".people")
            compose.onNodeWithText("Alle").performClick();loadingTitle("Alle","all")
            compose.onNodeWithText("Zoe").performClick();loadingTitle("Zoe","1")
            compose.onNodeWithContentDescription("Sortieren").performClick()
            compose.onNodeWithText("Datum: Älteste zuerst").performClick();loadingTitle("Zoe","1")
            compose.onNodeWithContentDescription("Zurück").performClick();loadingTitle("Alle","all")
            compose.onNodeWithContentDescription("Zurück").performClick();loadingTitle("Personen",".people")
        }
    }
    @Test fun photoTapTogglesAllControlsWithoutInterferingWithZoomOrPaging()=screen(Locale.ENGLISH) {controller,_ ->
        compose.onNodeWithContentDescription("first.jpg").performClick()
        val photo=compose.onNodeWithTag("photo-viewer-image")
        compose.waitUntil {photo.fetchSemanticsNode().config.getOrElse(SemanticsActions.CustomActions) {emptyList()}.isNotEmpty()}
        compose.onNodeWithText("Zoom in").assertDoesNotExist()
        compose.onNodeWithText("Fit photo").assertDoesNotExist()
        val initialBounds=photo.fetchSemanticsNode().boundsInRoot
        photo.performTouchInput {click(center)}
        compose.onNodeWithContentDescription("Close").assertDoesNotExist()
        compose.onNodeWithContentDescription("Share").assertDoesNotExist()
        compose.onNodeWithContentDescription("Information").assertDoesNotExist()
        compose.onNodeWithContentDescription("Next photo").assertDoesNotExist()
        compose.onNodeWithContentDescription("Start slideshow").assertDoesNotExist()
        assertEquals(initialBounds,photo.fetchSemanticsNode().boundsInRoot)
        photo.performTouchInput {swipeLeft()}
        compose.waitUntil {controller.state.value.selected=="second.jpg"}
        compose.onNodeWithContentDescription("Close").assertDoesNotExist()
        photo.performTouchInput {click(center)}
        compose.onNodeWithContentDescription("Close").assertIsDisplayed()
        compose.onNodeWithText("2 of 2").assertIsDisplayed()
        photo.performTouchInput {pinch(start0=center-Offset(40f,0f),end0=center-Offset(120f,0f),start1=center+Offset(40f,0f),end1=center+Offset(120f,0f))}
        compose.onNodeWithContentDescription("Close").assertIsDisplayed()
        val actions=photo.fetchSemanticsNode().config[SemanticsActions.CustomActions]
        assertEquals("Fit photo",actions.single().label)
        photo.performTouchInput {swipeRight()}
        assertEquals("second.jpg",controller.state.value.selected)
        // TalkBack keeps a zoom/reset action without an extra visible button.
        compose.runOnIdle {assertTrue(actions.single().action())}
        photo.performClick()
        compose.onNodeWithContentDescription("Close").assertDoesNotExist()
        photo.performClick()
        compose.onNodeWithContentDescription("Close").assertIsDisplayed().performClick()
        compose.onNodeWithContentDescription("second.jpg").assertIsDisplayed()
    }
    @Test fun sortingHasItsOwnMenuAndAdaptsToTheCurrentFolder()=screen(Locale.GERMAN,peopleFolders=true) {controller,_ ->
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Datum: Neueste zuerst").assertDoesNotExist()
        compose.onNodeWithText("Name: A–Z").assertDoesNotExist()
        compose.onNodeWithText("Einstellungen").assertIsDisplayed()
        androidx.test.espresso.Espresso.pressBack()
        compose.onNodeWithContentDescription("Sortieren").performClick()
        compose.onNodeWithText("Datum: Neueste zuerst").assertIsSelected()
        compose.onNodeWithText("Name: A–Z").assertDoesNotExist()
        compose.onNodeWithText("Anzahl Bilder: Absteigend").assertDoesNotExist()
        compose.onNodeWithText("Datum: Älteste zuerst").performClick()
        compose.waitUntil(10_000) {!controller.state.value.loading && controller.state.value.query.sort=="ascending_date"}
        compose.onNodeWithText("Ordner").performClick()
        compose.onNodeWithContentDescription("Sortieren").performClick()
        compose.onNodeWithText("Name: A–Z").assertIsDisplayed()
        compose.onNodeWithText("Anzahl Bilder: Absteigend").assertDoesNotExist()
        androidx.test.espresso.Espresso.pressBack()
        compose.onNodeWithText("Personen").performClick()
        compose.onNodeWithText("Familie").performClick()
        compose.onNodeWithContentDescription("Sortieren").performClick()
        compose.onNodeWithText("Datum: Neueste zuerst").assertDoesNotExist()
        compose.onNodeWithText("Anzahl Bilder: Absteigend").performClick()
        compose.waitUntil(10_000) {!controller.state.value.loading && controller.state.value.query.sort=="descending_count"}
        compose.onNodeWithContentDescription("Sortieren").performClick()
        compose.onNodeWithText("Anzahl Bilder: Absteigend").assertIsSelected()
        compose.onNodeWithText("Anzahl Bilder: Aufsteigend").performClick()
        compose.waitUntil(10_000) {!controller.state.value.loading && controller.state.value.query.sort=="ascending_count"}
        compose.onNodeWithText("Zoe").performClick()
        assertEquals("descending_date",controller.state.value.query.sort)
    }
    @Test fun olderServersKeepPeopleNameSorting()=screen(Locale.ENGLISH,peopleFolders=true,peopleCountSort=false) {_,_ ->
        compose.onNodeWithText("Folders").performClick()
        compose.onNodeWithText("Personen").performClick()
        compose.onNodeWithText("Familie").performClick()
        compose.onNodeWithContentDescription("Sort").performClick()
        compose.onNodeWithText("Name: A–Z").assertIsDisplayed()
        compose.onNodeWithText("Photo count: Descending").assertDoesNotExist()
    }
    @Test fun readerOpensPeopleForAFolderAndReturnsToThatFolder()=screen(Locale.GERMAN,directoryPeople=true) {controller,_ ->
        compose.onNodeWithText("Ordner").performClick()
        compose.onNodeWithText("Holiday").performClick()
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Personen im Ordner").performClick()
        compose.onNodeWithText("Ada").performClick()
        compose.onNodeWithContentDescription("first.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("second.jpg").assertDoesNotExist()
        compose.onNodeWithContentDescription("Zurück").performClick()
        compose.onNodeWithText("Ada").assertIsDisplayed()
        compose.onNodeWithContentDescription("Zurück").performClick()
        compose.waitUntil(10_000) {controller.state.value.query.path=="Holiday" && !controller.state.value.loading}
        compose.onNodeWithText("Holiday").assertIsDisplayed()
    }
    @Test fun readerBrowsesPeopleTagsAndPhotosAndReturnsThroughParents()=screen(Locale.GERMAN,peopleFolders=true) {controller,_ ->
        compose.onNodeWithText("Ordner").performClick()
        compose.onNodeWithText("Personen").assertIsDisplayed()
        for(i in 0..7) compose.onNodeWithContentDescription("Gesicht $i",useUnmergedTree=true).assertExists()
        compose.onNodeWithText("Personen").performClick()
        compose.onNodeWithText("Alle").assertIsDisplayed()
        compose.onNodeWithText("Familie").performClick()
        compose.onNodeWithText("Zoe").performClick()
        compose.onNodeWithContentDescription("first.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("second.jpg").assertIsDisplayed()
        compose.onNodeWithContentDescription("Sortieren").performClick()
        compose.onNodeWithText("Datum: Älteste zuerst").performClick()
        compose.waitUntil(10_000) {!controller.state.value.loading && controller.state.value.query.sort=="ascending_date"}
        assertEquals("first.jpg",controller.state.value.media.first().path)
        compose.onNodeWithContentDescription("Zurück").performClick()
        compose.waitUntil(10_000) {controller.state.value.name=="Familie"}
        compose.onNodeWithContentDescription("Zurück").performClick()
        compose.waitUntil(10_000) {controller.state.value.name=="Personen"}
        compose.onNodeWithText("Alle").assertIsDisplayed()
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
    @Test fun frameCaptionChoiceUsesEachPhotosFormattedFolderAndCanBeCancelled() = screen(Locale.ENGLISH) {controller,_ ->
        compose.onNodeWithContentDescription("More options").performClick()
        compose.onNodeWithText("Start photo frame").performClick()
        compose.waitUntil {controller.state.value.frame && controller.state.value.selected!=null}
        compose.onNodeWithTag("photo-viewer-image").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Folder name").performScrollTo().performClick()
        compose.onNodeWithText("Cancel").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("File name").performScrollTo().assertIsSelected()
        compose.onNodeWithText("Folder name").performScrollTo().performClick()
        compose.onNodeWithText("Save").performClick()
        compose.onNodeWithTag("photo-viewer-image").performClick()
        compose.onNodeWithText("Sommer Urlaub").assertIsDisplayed()
        compose.onNodeWithText("first.jpg").assertDoesNotExist()
        compose.onNodeWithTag("photo-viewer-image").performTouchInput {swipeLeft()}
        compose.waitUntil {controller.state.value.selected=="second.jpg"}
        compose.onNodeWithText("Fotos").assertIsDisplayed()
    }
    @Test fun frameRandomOrderCanBeCancelledAndRestoresTheGallery() = screen(Locale.ENGLISH,photoCount=3) {controller,_ ->
        val original=controller.state.value.query
        compose.onNodeWithContentDescription("More options").performClick()
        compose.onNodeWithText("Start photo frame").performClick()
        compose.waitUntil {controller.state.value.selected=="first.jpg"}
        compose.onNodeWithTag("photo-viewer-image").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Random order").performScrollTo().assertIsOff().performClick()
        compose.onNodeWithText("Cancel").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Random order").performScrollTo().assertIsOff().performClick()
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil {controller.state.value.query.sort=="random" && controller.state.value.selected=="photo-2.jpg"}
        compose.onNodeWithTag("photo-viewer-image").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Random order").performScrollTo().assertIsOn()
        compose.onNodeWithText("Cancel").performClick()
        compose.onNode(hasContentDescription("Close") and hasAnyAncestor(isDialog())).performClick()
        compose.waitUntil {!controller.state.value.frame}
        assertEquals(original,controller.state.value.query)
        compose.onNodeWithContentDescription("More options").performClick()
        compose.onNodeWithText("Start photo frame").performClick()
        compose.waitUntil {controller.state.value.query.sort=="random" && controller.state.value.selected=="photo-2.jpg"}
    }
    @Test fun frameRandomOrderExplainsOlderServers() = screen(Locale.ENGLISH,frameRandomSort=false) {controller,_ ->
        compose.onNodeWithContentDescription("More options").performClick()
        compose.onNodeWithText("Start photo frame").performClick()
        compose.waitUntil {controller.state.value.selected!=null}
        compose.onNodeWithTag("photo-viewer-image").performClick()
        compose.onNodeWithContentDescription("Photo frame settings").performClick()
        compose.onNodeWithText("Random order").performScrollTo().assertIsNotEnabled().assertIsOff()
        compose.onNodeWithText("Random order requires BearStack 0.69.0 or later.").performScrollTo().assertIsDisplayed()
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
