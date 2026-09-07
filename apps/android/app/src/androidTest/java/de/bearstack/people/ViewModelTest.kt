package de.bearstack.people

import android.app.Application
import androidx.lifecycle.ViewModelStore
import androidx.room.Room
import androidx.test.platform.app.InstrumentationRegistry
import de.bearstack.people.data.local.LabelingDatabase
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.PeopleViewModel
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test

class ViewModelTest {
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
            api.actionDelay=6000
            withContext(Dispatchers.Main){vm.startNaming();vm.nameChanged("Anna");vm.submitName()}
            until {api.commits>=1};api.actionDelay=0
            until {api.commits==3};idle(vm)
            until {vm.state.value.stats.any {it.action=="ignore" && it.groups==2L}}
            assertEquals(6L,vm.state.value.stats.first {it.action=="ignore"}.faces)
            assertEquals(1L,vm.state.value.stats.first {it.action=="name"}.faces)
            assertNull(vm.state.value.person)
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
