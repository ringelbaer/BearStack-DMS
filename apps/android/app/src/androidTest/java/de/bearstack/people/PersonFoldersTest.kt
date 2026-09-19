package de.bearstack.people

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.*
import de.bearstack.people.people.*
import de.bearstack.people.ui.PeopleApp
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import java.io.IOException

private class FolderService(val base: FakeService = FakeService()) : LabelingService by base {
    var supported=true
    var failPage=false
    var conflict=false
    var loseResponse=false
    var commits=0
    var requests=0
    val bodies=mutableListOf<JSONObject>()
    val reads=mutableListOf<Int>()
    var revision=1L
    val folders=mutableListOf(folder())
    override suspend fun session()=base.session.copy(personFolders=supported)
    override suspend fun personFolders(id: Long,page: Int): PersonFolderPage {
        reads+=page
        if(failPage) throw IOException("offline")
        return PersonFolderPage(id,base.people[id]?.name.orEmpty(),revision,page,folders.size>page*40,folders.drop((page-1)*40).take(40))
    }
    override suspend fun action(id: Long,body: String): Receipt {
        requests++
        val o=JSONObject(body);bodies+=o
        val operation=o.getString("operation_id")
        base.receipts[operation]?.let {return it}
        if(conflict || o.getLong("revision")!=revision) {conflict=false;revision++;throw ApiFailure(409,"conflict","stale")}
        val f=folders.first {it.directory==o.getString("directory")}
        val action=o.getString("action")
        folders.remove(f)
        if(action=="folder_exclude") folders.add(0,f.copy(excluded=true,count=0,preview=f.preview.copy(faces=emptyList())))
        revision++;commits++
        val r=Receipt(operation,action,id,o.optLong("target_id"),0,f.count,0,100,revision)
        base.receipts[operation]=r
        if(loseResponse) {loseResponse=false;throw IOException("lost response")}
        return r
    }
    companion object {
        fun folder(directory: String="20240102_Family_Trip/Nested_Folder",label: String="Fotos / 02.01.2024 · Family Trip / Nested Folder",face: Long=10)=
            PersonFolder(directory,label,501,false,Person(1,"",1,501,face,listOf(face,face+1),
                facePaths=mapOf(face to "$label / portrait.jpg"),faceBounds=mapOf(face to FaceBounds(.1f,.2f,.3f,.4f)),originalKeys=mapOf(face to "a".repeat(64))))
    }
}

class PersonFoldersTest {
    @get:Rule val compose=createComposeRule()
    private fun idle(vm:PeopleViewModel)=compose.waitUntil(10_000) {vm.state.value.connected && !vm.state.value.busy}
    private fun screen(scale:Float=1f, setup:(FolderService)->Unit={}, test:(PeopleViewModel,FolderService,LabelingDatabase)->Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val api=FolderService();setup(api)
        val store=ViewModelStore();lateinit var vm:PeopleViewModel
        compose.runOnUiThread {vm=PeopleViewModel(app,db,api,runBlocking {api.session()});store.put("test",vm)}
        try {
            compose.setGermanContent {
                val density=LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density,scale)) {PeopleApp(vm)}
            }
            idle(vm)
            compose.onNodeWithContentDescription("Weitere Optionen").performClick()
            if(api.supported) {compose.onNodeWithText("Ordner-Pfade prüfen").performClick();idle(vm)}
            test(vm,api,db)
        } finally {compose.runOnUiThread {store.clear()}}
    }
    @Test fun olderServerDisablesEntry()=screen(setup={it.supported=false}) {vm,api,_ ->
        compose.onNodeWithText("Ordner-Pfade prüfen").assertIsNotEnabled()
        assertFalse(vm.state.value.folderReview);assertTrue(api.reads.isEmpty())
    }
    @Test fun formattedPathsHoldZoomAccessiblePreviewAndBack()=screen {vm,api,_ ->
        compose.onNodeWithText("Fotos / 02.01.2024 · Family Trip / Nested Folder").assertExists()
        compose.onNodeWithText("20240102_Family_Trip/Nested_Folder").assertDoesNotExist()
        compose.onNodeWithContentDescription("Gesicht 2").assertExists()
        assertTrue(vm.originalKey(10)!!.endsWith(":large-preview:"+"a".repeat(64)))
        compose.onNodeWithTag("face-10").performTouchInput {down(center)}
        compose.mainClock.advanceTimeBy(800)
        compose.onNodeWithTag("original-photo-path").assertIsDisplayed()
        compose.onNodeWithTag("face-10").performTouchInput {moveBy(androidx.compose.ui.geometry.Offset(0f,-35f));up()}
        compose.onNodeWithTag("original-photo-path").assertDoesNotExist()
        val actions=compose.onNodeWithTag("face-10").fetchSemanticsNode().config[SemanticsActions.CustomActions]
        compose.runOnIdle {assertTrue(actions.first().action())}
        compose.onNodeWithText("Fotos / 02.01.2024 · Family Trip / Nested Folder / portrait.jpg").assertIsDisplayed()
        compose.onNodeWithText("Vorschau schließen").performClick()
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNode(hasText("Zurück") and isEnabled()).performClick();idle(vm)
        assertFalse(vm.state.value.folderReview);assertEquals(0,api.commits)
    }
    @Test fun excludeAndIncludeRemainAvailableWithoutFacesAtLargeFont()=screen(2f) {vm,api,_ ->
        compose.onNodeWithText("Pfad ausschließen und Gesichter auf unbenannt setzen").performScrollTo().performClick()
        compose.onNodeWithText("Abbrechen").performClick();assertEquals(0,api.commits)
        compose.onNodeWithText("Pfad ausschließen und Gesichter auf unbenannt setzen").performScrollTo().performClick()
        compose.onNodeWithText("Bestätigen").performClick();idle(vm)
        compose.onNodeWithText("Pfad wieder freigeben").performScrollTo().performClick()
        compose.onNodeWithText("Bestätigen").performClick();idle(vm)
        compose.onNodeWithText("Keine Ordner vorhanden.").assertExists()
        assertEquals(listOf("folder_exclude","folder_include"),api.bodies.map {it.getString("action")})
        assertTrue(api.bodies.none {it.has("face_ids")});assertEquals(2,api.commits)
    }
    @Test fun reassignSearchUsesExistingDialogAndTargetRevision()=screen(setup={api ->
        api.base.people[3]=Person(3,"Berta",7,1,30,listOf(30))
    }) {vm,api,_ ->
        compose.onNodeWithText("Alle neu zuweisen").performScrollTo().performClick()
        compose.onNodeWithText("Name").performTextInput("Bert")
        compose.waitUntil(10_000) {vm.state.value.suggestions.isNotEmpty()}
        compose.onNodeWithText("Berta").performClick();idle(vm)
        assertEquals(1,api.commits);assertFalse(vm.state.value.naming)
        val body=api.bodies.single()
        assertEquals("folder_move",body.getString("action"));assertEquals(3L,body.getLong("target_id"));assertEquals(7L,body.getLong("target_revision"))
        assertEquals("20240102_Family_Trip/Nested_Folder",body.getString("directory"))
    }
    @Test fun newNameAndConflictsRequireFreshDecision()=screen {vm,api,_ ->
        api.conflict=true
        compose.onNodeWithText("Alle neu zuweisen").performScrollTo().performClick()
        compose.onNodeWithText("Name").performTextInput("New Person")
        compose.onNodeWithText("Speichern").performClick();idle(vm)
        assertFalse(vm.state.value.naming);assertNull(vm.state.value.folderSelection);assertEquals(0,api.commits)
        compose.runOnUiThread {vm.requestFolderAction(vm.state.value.folderPage!!.folders.first(),"move");vm.nameChanged("New Person");vm.submitName()};idle(vm)
        assertEquals(1,api.commits);assertEquals("New Person",api.bodies.last().getString("name"));assertFalse(vm.state.value.naming)
    }
    @Test fun lostResponseIsResolvedOnceFromDurableReceipt()=screen {vm,api,db ->
        api.loseResponse=true
        compose.onNodeWithText("Alle ignorieren").performScrollTo().performClick()
        compose.onNodeWithText("Bestätigen").performClick();idle(vm)
        assertTrue(vm.state.value.unresolved);assertEquals(1,api.commits)
        compose.runOnUiThread {vm.closePersonFolders();vm.confirmFolderAction()}
        assertTrue(vm.state.value.folderReview);assertEquals(1,api.requests)
        // A new repository instance uses the same persisted operation after restart.
        runBlocking {
            val restored=PeopleRepository(db,api,api.session())
            assertNotNull(restored.pending());assertNotNull(restored.resolve());assertNull(restored.pending())
        }
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertFalse(vm.state.value.unresolved);assertEquals(1,api.requests);assertEquals(1,api.commits)
    }
    @Test fun namedPersonNavigationAndExcludedOnlyEntryStayReachable()=screen(setup={api ->
        api.base.upper=3
        api.base.people[3]=Person(3,"Ada",1,1,30,listOf(30))
    }) {vm,api,_ ->
        compose.runOnUiThread {vm.closePersonFolders()};idle(vm)
        compose.runOnUiThread {vm.openDirectory()};idle(vm)
        compose.onNodeWithText("Ada").performClick();idle(vm)
        compose.onNodeWithContentDescription("Weitere Optionen").performClick()
        compose.onNodeWithText("Ordner-Pfade prüfen").performClick();idle(vm)
        assertEquals(3L,vm.state.value.folderSource!!.id)
        compose.runOnUiThread {vm.closePersonFolders()};idle(vm)
        api.base.people[3]=api.base.people.getValue(3).copy(count=0,faceId=0,faces=emptyList())
        compose.runOnUiThread {vm.openDirectory()};idle(vm)
        compose.onNodeWithText("Ausgeschlossene Pfade").assertExists()
        compose.onNodeWithText("Ada").performClick();idle(vm)
        assertTrue(vm.state.value.folderReview);assertEquals(3L,vm.state.value.folderSource!!.id)
    }
    @Test fun refreshFailureAfterCommitNeverSendsAnotherWriteAndRootIsExplicit()=screen(setup={api ->
        api.folders.clear();api.folders+=FolderService.folder("","Fotos")
    }) {vm,api,_ ->
        api.failPage=true
        compose.onNodeWithText("Alle auf unbenannt setzen").performScrollTo().performClick()
        compose.onNodeWithText("Bestätigen").performClick();idle(vm)
        assertEquals(1,api.commits);assertFalse(vm.state.value.unresolved);assertFalse(vm.state.value.folderReady)
        assertEquals("",api.bodies.single().getString("directory"))
        api.failPage=false
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertEquals(1,api.requests);assertTrue(vm.state.value.folderReady)
        compose.onNodeWithText("Keine Ordner vorhanden.").assertExists()
    }
    @Test fun failedNextPageDisablesWritesAndRetryIsReadOnly()=screen(setup={api ->
        api.folders.clear()
        repeat(41) {i->api.folders+=FolderService.folder("$i",if(i==0) "Fotos" else "Fotos / Folder $i",100L+i*2)}
    }) {vm,api,_ ->
        assertEquals(40,vm.state.value.folderPage!!.folders.size)
        api.failPage=true
        compose.runOnUiThread {vm.personFolderPage(2)};idle(vm)
        assertFalse(vm.state.value.folderReady)
        compose.runOnUiThread {vm.requestFolderAction(vm.state.value.folderPage!!.folders.first(),"ignore")}
        assertNull(vm.state.value.folderConfirmation)
        api.failPage=false
        compose.runOnUiThread {vm.retry()};idle(vm)
        assertEquals(2,vm.state.value.folderPage!!.page);assertEquals(1,vm.state.value.folderPage!!.folders.size)
        compose.onNodeWithText("Fotos / Folder 40").assertExists();assertEquals(0,api.commits)
    }
}
