package de.bearstack.people

import android.app.Application
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.FaceMatch
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test

class ViewModelTest {
    @Test fun delayedDirectorySearchDoesNotOverwriteNewQueryAndStreamingRejectsChangedRevision() = runBlocking {
        val api=FakeService().apply {
            upper=50
            people[40]=Person(40,"Anna",1,100,400,(400L..499L).toList())
            people[50]=Person(50,"Berta",1,1,500,listOf(500))
        }
        model(api) {vm ->
            withContext(Dispatchers.Main) {vm.openDirectory()};idle(vm)
            api.directoryDelay=600
            withContext(Dispatchers.Main) {vm.namedQueryChanged("Ann")}
            until {api.directoryQueries.contains("Ann")}
            withContext(Dispatchers.Main) {vm.namedQueryChanged("Berta")}
            until {!vm.state.value.busy && vm.state.value.loadedNamedQuery=="Berta"}
            assertEquals(listOf(50L),vm.state.value.namedPeople.map {it.id})
            withContext(Dispatchers.Main) {vm.openPerson(api.people.getValue(40))};idle(vm)
            assertEquals(40,vm.state.value.selectedPerson!!.faces.size)
            withContext(Dispatchers.Main) {vm.morePersonFaces()};idle(vm)
            assertEquals(80,vm.state.value.selectedPerson!!.faces.size)
            withContext(Dispatchers.Main) {vm.requestUnassign(450);vm.morePersonFaces()};idle(vm)
            assertEquals(80,vm.state.value.selectedPerson!!.faces.size);assertEquals(0,api.commits)
            withContext(Dispatchers.Main) {vm.cancelUnassign()}
            api.people[40]=api.people.getValue(40).copy(revision=2)
            withContext(Dispatchers.Main) {vm.morePersonFaces()};idle(vm)
            assertEquals(2L,vm.state.value.selectedPerson!!.revision)
            assertEquals(40,vm.state.value.selectedPerson!!.faces.size)
            assertNotNull(vm.state.value.error);assertEquals(0,api.commits)
        }
    }
    @Test fun managementPreservesQueueAndResolvesLostFavoriteResponseBeforeNextDecision() = runBlocking {
        val api=FakeService().apply { upper=30; people[30]=Person(30,"Anna",1,1,300,listOf(300)) }
        model(api) { vm ->
            withContext(Dispatchers.Main) {vm.openDirectory()};idle(vm)
            assertEquals(listOf(30L),vm.state.value.namedPeople.map {it.id})
            withContext(Dispatchers.Main) {vm.openPerson(vm.state.value.namedPeople.single())};idle(vm)
            api.loseResponse=true
            withContext(Dispatchers.Main) {vm.favorite(300)};idle(vm)
            assertTrue(vm.state.value.unresolved);assertEquals(1,api.commits)
            withContext(Dispatchers.Main) {vm.unassign(300);vm.closeDirectory()};idle(vm)
            assertTrue(vm.state.value.directory);assertEquals(1,api.commits)
            withContext(Dispatchers.Main) {vm.retry()};idle(vm)
            assertFalse(vm.state.value.unresolved);assertEquals(setOf(300L),vm.state.value.selectedPerson!!.favorites)
            assertEquals(1,api.commits)
            withContext(Dispatchers.Main) {vm.startNaming();vm.nameChanged("Anna Neu");vm.submitName()};idle(vm)
            assertEquals("Anna Neu",vm.state.value.selectedPerson!!.name)
            assertEquals("Anna Neu",vm.state.value.namedPeople.single().name)
            assertEquals(1L,vm.state.value.person!!.id)
            withContext(Dispatchers.Main) {vm.unassign(300)};idle(vm)
            assertNull(vm.state.value.selectedPerson);assertTrue(vm.state.value.namedPeople.isEmpty())
            withContext(Dispatchers.Main) {vm.closeDirectory()};idle(vm)
            assertEquals(1L,vm.state.value.person!!.id)
            withContext(Dispatchers.Main) {vm.skip()};idle(vm)
            assertEquals(31L,vm.state.value.person!!.id)
        }
    }
    @Test fun managementConflictRequiresFreshDecisionAndListsPastTwentyPeople() = runBlocking {
        val api=FakeService().apply {
            upper=50
            for(id in 10L..50L) people[id]=Person(id,"Anna $id",1,1,id*10,listOf(id*10))
        }
        model(api) {vm ->
            withContext(Dispatchers.Main) {vm.openDirectory()};idle(vm)
            assertEquals(20,vm.state.value.namedPeople.size);assertTrue(vm.state.value.namedHasNext)
            withContext(Dispatchers.Main) {vm.moreNamedPeople()};idle(vm)
            withContext(Dispatchers.Main) {vm.moreNamedPeople()};idle(vm)
            assertEquals(41,vm.state.value.namedPeople.size);assertFalse(vm.state.value.namedHasNext)
            withContext(Dispatchers.Main) {vm.openPerson(vm.state.value.namedPeople.first())};idle(vm)
            api.people[10]=api.people.getValue(10).copy(name="Webänderung",revision=2)
            withContext(Dispatchers.Main) {vm.unassign(100)};idle(vm)
            assertEquals(0,api.commits);assertFalse(vm.state.value.unresolved)
            assertEquals("Webänderung",vm.state.value.selectedPerson!!.name)
            assertNotNull(vm.state.value.error)
            withContext(Dispatchers.Main) {vm.unassign(100)};idle(vm)
            assertEquals(1,api.commits);assertNull(vm.state.value.selectedPerson)
        }
    }
    @Test fun faceSearchCancelsOnTypingClosingAndBackgroundAndNeverWrites() = runBlocking {
        val api=FakeService().apply { matchDelay=2000; matches=listOf(FaceMatch(9,"Anna",1,90)) }
        model(api) { vm ->
            withContext(Dispatchers.Main) {vm.startNaming();vm.findFaceMatches()}
            until {api.matchedFaces.size==1}
            withContext(Dispatchers.Main) {vm.nameChanged("Other")}
            until {api.cancelledMatches==1}
            assertTrue(vm.state.value.faceMatches.isEmpty());assertFalse(vm.state.value.faceSearching)
            withContext(Dispatchers.Main) {vm.findFaceMatches()}
            until {api.matchedFaces.size==2}
            withContext(Dispatchers.Main) {vm.closeNaming()}
            until {api.cancelledMatches==2}
            withContext(Dispatchers.Main) {vm.startNaming();vm.findFaceMatches()}
            until {api.matchedFaces.size==3}
            withContext(Dispatchers.Main) {vm.background()}
            until {api.cancelledMatches==3}
            assertEquals(0,api.commits)
            assertTrue(vm.state.value.faceMatches.isEmpty())
        }
    }
    @Test fun faceSearchUsesDisplayedPageAndRechecksTargetBeforeAssigning() = runBlocking {
        val api=FakeService().apply {people[9]=Person(9,"Anna",7,1,90,listOf(90));matches=listOf(FaceMatch(9,"Anna",1,90))}
        model(api) { vm ->
            withContext(Dispatchers.Main) {vm.page(1)};idle(vm)
            withContext(Dispatchers.Main) {vm.startNaming();vm.findFaceMatches()}
            until {vm.state.value.faceSearchDone}
            assertEquals(listOf(14L),api.matchedFaces);assertEquals(0,api.commits)
            api.people[9]=api.people.getValue(9).copy(name="Changed",revision=8)
            withContext(Dispatchers.Main) {vm.assignFaceMatch(vm.state.value.faceMatches.single())};idle(vm)
            assertEquals(0,api.commits);assertNotNull(vm.state.value.error)
            api.people[9]=api.people.getValue(9).copy(name="Anna")
            withContext(Dispatchers.Main) {vm.startNaming();vm.findFaceMatches()}
            until {vm.state.value.faceSearchDone}
            withContext(Dispatchers.Main) {vm.assignFaceMatch(vm.state.value.faceMatches.single())};idle(vm)
            assertEquals(1,api.commits);assertEquals("assign",api.receipts.values.single().action)
            assertFalse(vm.state.value.naming)
        }
    }
    private suspend fun model(api: FakeService, test: suspend (PeopleViewModel) -> Unit) {
        val app=InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as Application
        val db=Room.inMemoryDatabaseBuilder(app,LabelingDatabase::class.java).build()
        val store=ViewModelStore()
        val vm=withContext(Dispatchers.Main) { PeopleViewModel(app,db,api,api.session).also {store.put("test",it)} }
        try { idle(vm);test(vm) } finally {withContext(Dispatchers.Main){store.clear()}}
    }
    private suspend fun idle(vm: PeopleViewModel) { withTimeout(10_000) {while(vm.state.value.busy)delay(10)} }
    private suspend fun until(predicate: () -> Boolean) {withTimeout(10_000){while(!predicate())delay(10)}}
    @Test fun skippedCardCanBeRecoveredWhenNextLoadFailsAndAtEndOfPass() = runBlocking {
        val api=FakeService()
        model(api) { vm ->
            api.failPerson=2
            withContext(Dispatchers.Main){vm.skip()};idle(vm)
            assertNull(vm.state.value.person);assertTrue(vm.state.value.canGoBack)
            withContext(Dispatchers.Main){vm.back()};idle(vm)
            assertEquals(1L,vm.state.value.person!!.id);assertFalse(vm.state.value.canGoBack)
            api.failPerson=null
            withContext(Dispatchers.Main){vm.skip()};idle(vm)
            withContext(Dispatchers.Main){vm.skip()};idle(vm)
            assertNull(vm.state.value.person);assertTrue(vm.state.value.canGoBack)
            withContext(Dispatchers.Main){vm.back()};idle(vm)
            assertEquals(2L,vm.state.value.person!!.id);assertEquals(1,vm.state.value.skipped)
        }
    }
    @Test fun undoAndBackgroundRestoreUnsentGroupsWithoutBlockingOtherCards() = runBlocking {
        val api=FakeService()
        model(api) { vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            assertEquals(listOf(1L),vm.state.value.undoIgnores)
            assertEquals(2L,vm.state.value.person!!.id)
            withContext(Dispatchers.Main){vm.skip()};idle(vm)
            assertNull(vm.state.value.person)
            withContext(Dispatchers.Main){vm.undoIgnore()}
            until {vm.state.value.person?.id==1L && !vm.state.value.busy}
            assertTrue(vm.state.value.undoIgnores.isEmpty());assertEquals(0,api.commits)
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.background()}
            until {vm.state.value.person?.id==1L && !vm.state.value.busy}
            assertTrue(vm.state.value.undoIgnores.isEmpty());assertEquals(0,api.commits)
        }
        assertEquals(0,api.commits)
    }
    @Test fun ignoreCountsConfirmedSourceAndKeepsNextCardAfterFiveSeconds() = runBlocking {
        val api=FakeService()
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            assertEquals(2L,vm.state.value.person!!.id)
            delay(200);assertEquals(0,api.commits)
            until {api.commits==1};idle(vm)
            assertEquals(2L,vm.state.value.person!!.id)
            until {vm.state.value.stats.any {it.action=="ignore"}}
            assertEquals(5L,vm.state.value.stats.first {it.action=="ignore"}.faces)
        }
    }
    @Test fun severalIgnoresWaitForNamingWithoutLosingAnyCommit() = runBlocking {
        val api=FakeService();api.people[3]=Person(3,"",1,1,30,listOf(30))
        // Make the third group available in the existing small candidate page.
        api.upper=3
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            assertEquals(listOf(1L,2L),vm.state.value.undoIgnores)
            assertEquals(3L,vm.state.value.person!!.id)
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("Ann")}
            until {vm.state.value.undoIgnores.isEmpty()}
            assertFalse(vm.state.value.busy);assertTrue(vm.state.value.naming)
            assertEquals(0,api.commits);assertEquals("Ann",vm.state.value.name)
            api.actionDelay=6000
            withContext(Dispatchers.Main){vm.nameChanged("Anna");vm.submitName()}
            until {api.commits>=1};api.actionDelay=0
            until {api.commits==3};idle(vm)
            until {vm.state.value.stats.any {it.action=="ignore" && it.groups==2L}}
            assertEquals(6L,vm.state.value.stats.first {it.action=="ignore"}.faces)
            assertEquals(1L,vm.state.value.stats.first {it.action=="name"}.faces)
            assertNull(vm.state.value.person)
        }
    }
    @Test fun expiredIgnoreWaitsForDuplicateDialogCancellation() = runBlocking {
        val api=FakeService().apply {people[9]=Person(9,"Anna",1,1,90,listOf(90))}
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("Anna");vm.submitName()};idle(vm)
            assertEquals("Anna",vm.state.value.duplicates.single().name)
            until {vm.state.value.undoIgnores.isEmpty()}
            assertFalse(vm.state.value.busy);assertTrue(vm.state.value.naming)
            assertEquals("Anna",vm.state.value.duplicates.single().name)
            assertEquals(0,api.commits)
            withContext(Dispatchers.Main){vm.closeNaming()}
            until {api.commits==1};idle(vm)
            assertEquals(2L,vm.state.value.person!!.id)
            assertEquals(1L,api.receipts.values.single().source)
            assertEquals("ignore",api.receipts.values.single().action)
        }
    }
    @Test fun backgroundRestoresExpiredIgnoreWaitingForOpenDialog() = runBlocking {
        val api=FakeService()
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("Entwurf")}
            until {vm.state.value.undoIgnores.isEmpty()}
            assertEquals(0,api.commits)
            withContext(Dispatchers.Main){vm.background()};idle(vm)
            withContext(Dispatchers.Main){vm.foreground();vm.closeNaming();vm.skip()};idle(vm)
            assertEquals(1L,vm.state.value.person!!.id)
            assertEquals(0,api.commits)
            assertFalse(vm.state.value.unresolved)
        }
    }
    @Test fun lastToastUndoKeepsOlderIgnoreAndLostResponseDoesNotDoubleCount() = runBlocking {
        val api=FakeService()
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.ignore()};idle(vm)
            withContext(Dispatchers.Main){vm.undoIgnore()}
            until {vm.state.value.person?.id==2L && !vm.state.value.busy}
            assertEquals(listOf(1L),vm.state.value.undoIgnores)
            api.loseResponse=true
            until {vm.state.value.unresolved};assertEquals(1,api.commits)
            assertEquals(2L,vm.state.value.person!!.id)
            withContext(Dispatchers.Main){vm.retry()};idle(vm)
            assertEquals(1,api.commits);assertEquals(2L,vm.state.value.person!!.id)
            until {vm.state.value.stats.any {it.action=="ignore"}}
            assertEquals(1L,vm.state.value.stats.single().groups)
        }
    }
    @Test fun confirmedCardCannotBeSkippedAgainWhenLoadingNextGroupFails() = runBlocking {
        val api=FakeService()
        model(api) {vm ->
            api.failPerson=2
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("Neue Person");vm.submitName()};idle(vm)
            assertEquals(1,api.commits)
            assertNull(vm.state.value.person)
            assertNotNull(vm.state.value.error)
            withContext(Dispatchers.Main){vm.skip()};idle(vm)
            api.failPerson=null
            withContext(Dispatchers.Main){vm.retry()};idle(vm)
            assertEquals(2L,vm.state.value.person!!.id)
            assertTrue(vm.state.value.stats.none {it.action=="skip"})
        }
    }
    @Test fun autocompleteDebouncesCancelsAndDuplicateRequiresExplicitChoice() = runBlocking {
        val api=FakeService();api.people[9]=Person(9,"Anna",1,1,90,listOf(90))
        model(api) {vm ->
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("An");vm.nameChanged("Ann")}
            delay(100);assertTrue(api.queries.isEmpty())
            until {vm.state.value.suggestions.isNotEmpty()}
            assertEquals(listOf("Ann"),api.queries)
            api.slowQuery="old"
            withContext(Dispatchers.Main){vm.nameChanged("old")}
            until {"old" in api.queries}
            withContext(Dispatchers.Main){vm.nameChanged("Anna")}
            until {vm.state.value.suggestions.isNotEmpty()}
            assertEquals(1,api.cancelledQueries)
            withContext(Dispatchers.Main){vm.submitName()};idle(vm)
            assertEquals(0,api.commits);assertEquals("Anna",vm.state.value.duplicates.single().name)
            withContext(Dispatchers.Main){vm.submitName(true)};idle(vm)
            assertEquals(1,api.commits);assertFalse(vm.state.value.naming)
        }
    }
}
