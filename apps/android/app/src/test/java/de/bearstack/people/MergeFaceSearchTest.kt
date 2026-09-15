package de.bearstack.people

import de.bearstack.people.data.remote.*
import de.bearstack.people.people.MergeFaceSearch
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.test.*
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException

@OptIn(ExperimentalCoroutinesApi::class)
class MergeFaceSearchTest {
    private val first=Person(1,"",1,2,10,listOf(11))
    private val second=Person(2,"",1,4,20)
    private val pair=MergeSuggestion(7,first,second,.7)
    private val anna=FaceMatch(9,"Anna",3,90)

    @Test fun onlyTwoUnnamedGroupsEnableTheSearchAndFirstMatchesSkipTheSecondWitness()=runTest {
        val calls=mutableListOf<Long>()
        for(candidate in listOf(pair.copy(source=first.copy(name="Named")),
            pair.copy(target=second.copy(name="Named")),
            pair.copy(source=first.copy(name="Named"),target=second.copy(name="Named")),
            pair.copy(source=first.copy(faceId=0,faces=emptyList())))) {
            val search=MergeFaceSearch(this,candidate) {face -> flow {calls+=face;emit(listOf(anna))} }
            search.resume();search.retry();runCurrent()
            assertNull(search.state.value);search.close()
        }
        assertTrue(calls.isEmpty())
        val search=MergeFaceSearch(this,pair) {face -> flow {calls+=face;emit(listOf(anna))} }
        search.resume();search.resume();runCurrent()
        assertEquals(listOf(11L),calls)
        assertEquals(listOf(anna),search.state.value!!.matches)
        search.pause();search.resume();runCurrent()
        assertEquals(listOf(11L),calls)
        search.retry();runCurrent();assertEquals(listOf(11L,11L),calls)
        search.close()
    }

    @Test fun emptyCompletedFirstSearchTriesSecondWitnessAndReusesItsMatches()=runTest {
        val calls=mutableListOf<Long>()
        val gate=CompletableDeferred<Unit>()
        val search=MergeFaceSearch(this,pair.copy(target=second.copy(faces=listOf(21)))) {face -> flow {
            calls+=face
            if(face==11L) {emit(emptyList());gate.await()}
            else emit(listOf(anna))
        } }
        search.resume();runCurrent()
        assertEquals(listOf(11L),calls);assertTrue(search.state.value!!.loading)
        gate.complete(Unit);runCurrent()
        assertEquals(listOf(11L,21L),calls)
        assertEquals(listOf(anna),search.state.value!!.matches)
        assertTrue(search.state.value!!.complete);assertFalse(search.state.value!!.loading)
        search.pause();search.resume();runCurrent()
        assertEquals(listOf(11L,21L),calls)
        search.close()
    }

    @Test fun finalEmptyRankingTriggersFallbackEvenAfterPreliminaryMatches()=runTest {
        val calls=mutableListOf<Long>()
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face
            emit(listOf(anna))
            emit(listOf(FaceMatch(first.id,"Excluded",1,10),FaceMatch(second.id,"Excluded",1,20),
                FaceMatch(99,"   ",1,990)))
        } }
        search.resume();runCurrent()
        assertEquals(listOf(11L,20L),calls)
        assertTrue(search.state.value!!.complete);assertTrue(search.state.value!!.matches.isEmpty())
        search.close()
    }

    @Test fun invalidOrDuplicateSecondWitnessDoesNotStartAnotherRequest()=runTest {
        for(face in listOf(0L,11L)) {
            val calls=mutableListOf<Long>()
            val search=MergeFaceSearch(this,pair.copy(target=second.copy(faceId=face))) {id -> flow {
                calls+=id;emit(emptyList())
            } }
            search.resume();runCurrent()
            assertEquals(listOf(11L),calls);assertTrue(search.state.value!!.complete)
            search.close()
        }
    }

    @Test fun fallbackFailureClearsMatchesAndRetryStartsWithFirstGroup()=runTest {
        val calls=mutableListOf<Long>();var failing=true
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face
            if(face==11L) emit(emptyList()) else {
                emit(listOf(anna))
                if(failing) throw IOException("failed fallback")
            }
        } }
        search.resume();runCurrent()
        assertEquals(listOf(11L,20L),calls)
        assertTrue(search.state.value!!.matches.isEmpty());assertNotNull(search.state.value!!.error)
        search.pause();search.resume();runCurrent();assertEquals(2,calls.size)
        failing=false;search.retry();runCurrent()
        assertEquals(listOf(11L,20L,11L,20L),calls)
        assertEquals(listOf(anna),search.state.value!!.matches);assertTrue(search.state.value!!.complete)
        search.close()
    }

    @Test fun pauseDuringFallbackCancelsItAndResumeRestartsThePair()=runTest {
        val calls=mutableListOf<Long>();var cancelled=0
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face
            if(face==11L) emit(emptyList()) else {
                emit(listOf(anna))
                try {awaitCancellation()} finally {cancelled++}
            }
        } }
        search.resume();runCurrent();search.pause();runCurrent()
        assertEquals(1,cancelled);assertTrue(search.state.value!!.matches.isEmpty())
        search.resume();runCurrent()
        assertEquals(listOf(11L,20L,11L,20L),calls)
        search.close();runCurrent();assertEquals(2,cancelled);assertNull(search.state.value)
    }

    @Test fun oneStreamReplacesTheSharedRankingAndKeepsMemoryBounded()=runTest {
        val gate=CompletableDeferred<Unit>();var active=0;var maximum=0
        val search=MergeFaceSearch(this,pair) {flow {
            active++;maximum=maxOf(maximum,active)
            try {
                emit(listOf(anna));gate.await()
                emit(listOf(FaceMatch(99,"",1,990))+(1L..100L).map {FaceMatch(it,"Person $it",1,it*10)})
            } finally {active--}
        } }
        search.resume();search.retry();runCurrent()
        assertEquals(1,maximum)
        assertEquals(listOf(anna),search.state.value!!.matches)
        assertTrue(search.state.value!!.loading)
        gate.complete(Unit);runCurrent()
        val result=search.state.value!!
        assertEquals(20,result.matches.size);assertEquals(3L,result.matches.first().id)
        assertTrue(result.complete);assertFalse(result.loading)
        search.close();assertEquals(0,active)
    }

    @Test fun pauseClearsPreliminaryResultsAndResumeSearchesOnlyFirstGroupAgain()=runTest {
        val calls=mutableListOf<Long>();var cancelled=0
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face;emit(listOf(anna))
            try {awaitCancellation()} finally {cancelled++}
        } }
        search.resume();runCurrent();search.pause();runCurrent()
        assertEquals(1,cancelled)
        assertTrue(search.state.value!!.matches.isEmpty());assertFalse(search.state.value!!.loading)
        search.retry();runCurrent();assertEquals(1,calls.size)
        search.resume();runCurrent();assertEquals(listOf(11L,11L),calls)
        search.close();runCurrent()
    }

    @Test fun failedStreamClearsPreliminaryMatchesAndWaitsForExplicitRetry()=runTest {
        var failing=true;val calls=mutableListOf<Long>()
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face;emit(listOf(anna))
            if(failing) throw IOException("do not display raw server details")
        } }
        search.resume();runCurrent()
        assertTrue(search.state.value!!.matches.isEmpty());assertNotNull(search.state.value!!.error)
        search.pause();search.resume();runCurrent();assertEquals(1,calls.size)
        failing=false;search.retry();runCurrent()
        assertEquals(2,calls.size);assertNull(search.state.value!!.error)
        assertEquals(listOf(anna),search.state.value!!.matches)
        search.close()
    }

    @Test fun closedPairCannotPublishLateResultsOrRestart()=runTest {
        val gate=CompletableDeferred<Unit>();val calls=mutableListOf<Long>()
        val search=MergeFaceSearch(this,pair) {face -> flow {
            calls+=face
            // Simulate a producer which finishes cleanup even after cancellation.
            withContext(NonCancellable) {gate.await()}
            emit(listOf(anna))
        } }
        search.resume();runCurrent();search.close();gate.complete(Unit);runCurrent()
        search.resume();search.retry();runCurrent()
        assertNull(search.state.value);assertEquals(listOf(11L),calls)
    }
}
