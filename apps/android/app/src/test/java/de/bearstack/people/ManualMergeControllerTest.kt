package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.people.ManualMergeController
import kotlinx.coroutines.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ManualMergeControllerTest {
    private class Fake : LabelingService {
        var sessionValue=Session("instance","dataset","account",2000,manualMerge=true)
        var intercept:suspend (Boolean)->Unit = {}
        val requests=mutableListOf<Triple<Long,Long,Boolean>>()
        override suspend fun session()=sessionValue
        override suspend fun mergeGroups(after:Long,upper:Long,includeNamed:Boolean):Candidates {
            requests+=Triple(after,upper,includeNamed);intercept(includeNamed)
            val people=(after+1..upper).asSequence().filter {includeNamed || it%2==1L}.take(21)
                .map {Person(it,if(it%2==0L) "Named $it" else "",1,1,it*10,listOf(it*10))}.toList()
            return Candidates(people.take(20),people.take(20).lastOrNull()?.id ?: after,people.size>20)
        }
        override suspend fun candidates(after:Long,upper:Long):Candidates=error("unused")
        override suspend fun person(id:Long,offset:Int):Person=error("unused")
        override suspend fun suggestions(q:String,exact:Boolean):List<Person> = error("unused")
        override suspend fun action(id:Long,body:String):Receipt=error("unused")
        override suspend fun receipt(operation:String,dataset:String):Receipt=error("unused")
    }
    @Test fun longScrollKeepsOnlyNearbyPagesAndReloadsOldIntervals()=runTest {
        val api=Fake();val controller=ManualMergeController(this,api,api.sessionValue.scope)
        controller.reset();runCurrent()
        val selected=controller.state.value.person(0)!!
        controller.select(selected)
        repeat(12) {
            val count=controller.state.value.count
            controller.visible(count-5,count-1);runCurrent()
            assertTrue(controller.state.value.pages.size<=3)
            assertEquals(listOf(selected),controller.state.value.selected)
        }
        assertTrue(controller.state.value.count>200)
        assertFalse(0 in controller.state.value.pages)
        controller.visible(0,4);runCurrent()
        assertEquals(selected,controller.state.value.person(0))
        assertEquals(0L,api.requests.last().first)
        assertEquals(39L,api.requests.last().second)
        controller.cancel()
    }
    @Test fun filterChangeCancelsLateResponseAndClearsSelection()=runTest {
        val api=Fake();val controller=ManualMergeController(this,api,api.sessionValue.scope)
        controller.reset();runCurrent();controller.select(controller.state.value.person(0)!!)
        val gate=CompletableDeferred<Unit>()
        api.intercept={named -> if(!named) withContext(NonCancellable){gate.await()}}
        controller.visible(15,19);runCurrent();assertTrue(controller.state.value.loading)
        controller.reset(true);runCurrent()
        assertTrue(controller.state.value.selected.isEmpty())
        assertEquals((1L..20L).toList(),controller.state.value.pages[0]!!.map {it.id})
        gate.complete(Unit);runCurrent()
        assertTrue(controller.state.value.includeNamed)
        assertEquals(20,controller.state.value.count)
        controller.cancel()
    }
    @Test fun failedPageRetryKeepsCursorAndSelectionAndUnsupportedServerDoesNotBrowse()=runTest {
        val api=Fake();val controller=ManualMergeController(this,api,api.sessionValue.scope)
        controller.reset();runCurrent();controller.select(controller.state.value.person(0)!!)
        api.intercept={throw java.io.IOException("offline")}
        controller.visible(15,19);runCurrent()
        assertNotNull(controller.state.value.error);assertEquals(20,controller.state.value.count)
        val failed=api.requests.last()
        api.intercept={};controller.retry();runCurrent()
        assertEquals(failed,api.requests.last());assertEquals(40,controller.state.value.count)
        assertEquals(1,controller.state.value.selected.size)
        api.sessionValue=api.sessionValue.copy(manualMerge=false)
        val calls=api.requests.size
        controller.reset();runCurrent()
        assertEquals(calls,api.requests.size)
        assertEquals(R.string.people_manual_merge_version,controller.state.value.error!!.resource)
        controller.cancel()
    }
    @Test fun selectionIsBoundedAndNamingUsesFirstNamedSelection()=runTest {
        val api=Fake();val controller=ManualMergeController(this,api,api.sessionValue.scope)
        controller.reset(true);runCurrent()
        repeat(61){i -> controller.select(Person(i+1L,if(i==1) "Anna" else "",1,1,i+10L))}
        assertEquals(60,controller.state.value.selected.size)
        assertEquals("Anna",controller.state.value.retainedName)
        controller.startNaming();assertEquals("Anna",controller.state.value.name)
        controller.duplicateName();controller.nameChanged("Berta")
        assertFalse(controller.state.value.duplicateName)
        controller.closeNaming();assertEquals(60,controller.state.value.selected.size)
        controller.clearSelection();assertTrue(controller.state.value.selected.isEmpty())
        controller.cancel()
    }
    @Test fun interruptedInitialLoadRestartsAndRefreshRetainsScrollEpoch()=runTest {
        val api=Fake();val controller=ManualMergeController(this,api,api.sessionValue.scope)
        controller.reset();controller.cancel();runCurrent()
        controller.visible(0,0);runCurrent()
        assertEquals(20,controller.state.value.count)
        controller.visible(15,19);runCurrent()
        controller.visible(25,29);runCurrent()
        val epoch=controller.state.value.epoch
        controller.select(controller.state.value.person(25)!!)
        controller.refresh();runCurrent()
        assertEquals(epoch,controller.state.value.epoch)
        assertTrue(controller.state.value.selected.isEmpty())
        assertTrue(1 in controller.state.value.pages)
        controller.cancel()
    }
}
