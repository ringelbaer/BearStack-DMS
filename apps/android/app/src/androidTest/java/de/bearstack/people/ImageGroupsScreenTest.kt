package de.bearstack.people

import android.content.res.Configuration
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
    private class Service(val photos: List<Photo>): PhotosService {
        var created: Pair<List<String>,String>?=null
        var changed: String?=null
        override suspend fun imageGroup(id: Long)=ImageGroup(id,4,listOf(ImageGroupMember(1,"first.jpg","Fotos / first.jpg",true,false),ImageGroupMember(2,"second.jpg","Fotos / second.jpg",false,false)))
        override suspend fun updateImageGroup(group: ImageGroup,action: String,entity: Long,paths: List<String>): Boolean {assertEquals(4L,group.revision);changed=action;return false}
        override suspend fun session()=PhotoSession("test",true,240,240,1280,2048,5,8,imageGroups=true,folderPosition=true)
        override suspend fun browse(query: PhotoQuery,page: Int,section: String)=PhotoPage(query.path,"",page,photos.size,false,0,false,false,photos,emptyList(),emptyList())
        override suspend fun info(path: String)=photos.first {it.path==path}
        override suspend fun blog(path: String)=error("unused")
        override fun thumbnail(photo: Photo,size: Int)=""
        override fun original(photo: Photo)=""
        override suspend fun createImageGroup(paths: List<String>,primary: String): Long {created=paths to primary;return 1}
    }
    private fun screen(info: Boolean=false, group: Boolean=false, editor: Boolean=true, test: (Service,()->Photo?)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext
        val context=app.createConfigurationContext(Configuration(app.resources.configuration).apply {setLocale(Locale.ENGLISH)})
        val service=Service(listOf(photo("first.jpg"),photo("second.jpg")))
        val owner=CoroutineScope(SupervisorJob()+Dispatchers.Main.immediate)
        lateinit var controller: PhotosController
        var folder:Photo?=null
        compose.runOnUiThread {controller=PhotosController(owner,service,PhotoSession("test",editor,240,240,1280,2048,5,8,imageGroups=true,folderPosition=true))}
        try {
            compose.setContent {CompositionLocalProvider(LocalContext provides context,LocalResources provides context.resources,LocalConfiguration provides context.resources.configuration) {
                MaterialTheme {
                    if(group) ImageGroupDialog(controller,1,null,null,onClose={},onChanged={})
                    else if(info) PhotoInfoSheet(service.photos.first(),service,controller,onFolder={folder=it}) {}
                    else PhotoGroupSelectionAction(controller,service.photos)
                }
            }}
            compose.waitUntil { !controller.state.value.loading }
            test(service,{folder})
        } finally {compose.runOnUiThread {controller.close();owner.cancel()}}
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
    @Test fun infoUsesServerDisplayPathAndLinksTheActualPhoto()=screen(info=true) {_,folder ->
        compose.onNodeWithText("Fotos / first.jpg").assertIsDisplayed()
        compose.onNodeWithText("Show in gallery folder").performScrollTo().performClick()
        assertEquals("first.jpg",folder()?.path)
    }
}
