package de.bearstack.people

import android.content.res.Configuration
import android.graphics.Bitmap
import coil3.ImageLoader
import coil3.EventListener
import coil3.request.ImageRequest
import coil3.request.SuccessResult
import java.io.File
import java.util.concurrent.ConcurrentHashMap
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.remote.*
import de.bearstack.people.photos.*
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.util.Locale

class ImageGroupsScreenTest {
    @get:Rule val compose=createComposeRule()
    private fun photo(path: String)=Photo(path,path,"image","image/jpeg","1","2026-09-09T10:00:00Z",null,1,1,1,displayPath="Fotos / $path")
    private class Service(val photos: List<Photo>, val directory: File): PhotosService {
        val decoded=ConcurrentHashMap.newKeySet<String>()
        var created: Pair<List<String>,String>?=null
        var changed: String?=null
        override suspend fun imageGroup(id: Long)=ImageGroup(id,4,listOf(ImageGroupMember(1,"first.jpg","Fotos / first.jpg",true,false),ImageGroupMember(2,"second.jpg","Fotos / second.jpg",false,false)))
        override suspend fun updateImageGroup(group: ImageGroup,action: String,entity: Long,paths: List<String>): Boolean {assertEquals(4L,group.revision);changed=action;return false}
        override suspend fun session()=PhotoSession("test",true,240,240,1280,2048,5,8,imageGroups=true,folderPosition=true)
        override suspend fun browse(query: PhotoQuery,page: Int,section: String)=PhotoPage(query.path,"",page,photos.size,false,0,false,false,photos,emptyList(),emptyList())
        override suspend fun info(path: String)=photos.first {it.path==path}
        override suspend fun blog(path: String)=error("unused")
        override fun thumbnail(photo: Photo,size: Int)=File(directory,photo.path).toURI().toString()
        override fun original(photo: Photo)=thumbnail(photo,2048)
        override suspend fun createImageGroup(paths: List<String>,primary: String): Long {created=paths to primary;return 1}
    }
    private fun screen(info: Boolean=false, group: Boolean=false, editor: Boolean=true, adding: Boolean=false, test: (Service,()->Photo?)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(Locale.ENGLISH)})
        val directory=File(context.cacheDir,"group-previews-${System.nanoTime()}").apply {mkdirs()}
        val photos=listOf(photo("first.jpg").copy(imageGroupId=if(adding) 1L else 0L),photo("second.jpg"))
        photos.forEachIndexed {index,photo ->
            val bitmap=Bitmap.createBitmap(80,60,Bitmap.Config.ARGB_8888).apply {eraseColor(if(index==0) 0xffff0000.toInt() else 0xff0000ff.toInt())}
            File(directory,photo.path).outputStream().use {bitmap.compress(Bitmap.CompressFormat.PNG,100,it)}
            bitmap.recycle()
        }
        val service=Service(photos,directory)
        val images=ImageLoader.Builder(context).eventListener(object: EventListener() {
            override fun onSuccess(request: ImageRequest,result: SuccessResult) {service.decoded.add(request.data.toString())}
        }).build()
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        lateinit var controller: PhotosController
        var folder:Photo?=null
        compose.runOnUiThread {controller=PhotosController(owner,service,PhotoSession("test",editor,240,240,1280,2048,5,8,imageGroups=true,folderPosition=true))}
        try {
            compose.setContent {CompositionLocalProvider(LocalContext provides context,LocalResources provides context.resources,LocalConfiguration provides context.resources.configuration) {
                MaterialTheme {
                    if(group) ImageGroupDialog(controller,1,null,null,onClose={},onChanged={})
                    else if(info) PhotoInfoSheet(service.photos.first(),service,controller,onFolder={folder=it}) {}
                    else PhotoGroupSelectionAction(controller,service.photos,images)
                }
            }}
            compose.waitUntil { !controller.state.value.loading }
            test(service,{folder})
        } finally {compose.runOnUiThread {controller.close();owner.cancel();images.shutdown()};directory.deleteRecursively()}
    }
    @Test fun dissolveRequiresConfirmationAndUsesCurrentRevision()=screen(group=true) {service,_ ->
        compose.onNodeWithText("Dissolve group").performClick()
        compose.onNodeWithText("Cancel").performClick()
        assertNull(service.changed)
        compose.onNodeWithText("Dissolve group").performClick()
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil {service.changed!=null}
        assertEquals("dissolve",service.changed)
    }
    @Test fun readersCanSeeMembersButCannotChangeGroups()=screen(group=true,editor=false) {_,_ ->
        compose.onNodeWithText("Fotos / first.jpg").assertIsDisplayed()
        compose.onNodeWithText("Dissolve group").assertDoesNotExist()
        compose.onNodeWithText("Remove from group").assertDoesNotExist()
        compose.onNodeWithText("Use as primary").assertDoesNotExist()
    }
    @Test fun cancelDoesNotWriteAndChosenPrimaryIsSubmitted()=screen {service,_ ->
        compose.onNodeWithText("Group").performClick()
        compose.onNodeWithText("Cancel").performClick()
        assertNull(service.created)
        compose.onNodeWithText("Group").performClick()
        compose.onNodeWithText("Fotos / second.jpg").performClick()
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil {service.created!=null}
        assertEquals(listOf("first.jpg","second.jpg") to "second.jpg",service.created)
    }
    @Test fun decodedPreviewsOpenViewerAndDisplayedPhotoBecomesPrimaryOnlyOnSave()=screen {service,_ ->
        compose.onNodeWithText("Group").performClick()
        compose.waitUntil(5000) {service.decoded.size==2}
        compose.onNodeWithTag("group-preview:first.jpg").performClick()
        compose.onNodeWithTag("photo-viewer-image").assertIsDisplayed()
        compose.onNodeWithContentDescription("Next photo").performClick()
        compose.onNodeWithText("2 of 2").assertIsDisplayed()
        compose.onNodeWithText("Use as primary").performClick()
        compose.onNodeWithTag("group-primary:second.jpg").assertIsSelected()
        assertNull(service.created)
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil {service.created!=null}
        assertEquals("second.jpg",service.created!!.second)
    }
    @Test fun closingPreviewKeepsPrimaryAndCancelDoesNotWrite()=screen {service,_ ->
        compose.onNodeWithText("Group").performClick()
        compose.onNodeWithTag("group-preview:second.jpg").performClick()
        compose.onNodeWithContentDescription("Close").performClick()
        compose.onNodeWithTag("group-primary:first.jpg").assertIsSelected()
        compose.onNodeWithText("Cancel").performClick()
        assertNull(service.created)
    }
    @Test fun addingToGroupShowsPreviewsWithoutChangingPrimary()=screen(adding=true) {service,_ ->
        compose.onNodeWithText("Group").performClick()
        compose.waitUntil(5000) {service.decoded.size==2}
        compose.onNodeWithTag("group-preview:second.jpg").performClick()
        compose.onNodeWithTag("photo-viewer-image").assertIsDisplayed()
        compose.onNodeWithText("Use as primary").assertDoesNotExist()
        compose.onNodeWithContentDescription("Close").performClick()
        compose.onNodeWithText("Save").performClick()
        compose.waitUntil {service.changed!=null}
        assertEquals("add",service.changed)
    }
    @Test fun infoUsesServerDisplayPathAndLinksTheActualPhoto()=screen(info=true) {_,folder ->
        compose.onNodeWithText("Fotos / first.jpg").assertIsDisplayed()
        compose.onNodeWithText("Show in gallery folder").performScrollTo().performClick()
        assertEquals("first.jpg",folder()?.path)
    }
}
