package de.bearstack.people

import de.bearstack.people.data.local.QueueState
import de.bearstack.people.data.remote.Receipt
import de.bearstack.people.people.*
import org.junit.Assert.*
import org.junit.Test

class QueueAndGestureTest {
    @Test fun manualMergeRemovesAllSelectedQueueEntriesAndReoffersCombinedGroup() {
        val before=QueueState("scope",100,"pass",current=2,page=4,remaining="1,3,4",detached="2,5",
            skipped="1,6",skipHistory="1:0,6:4",resume="2:0,7:0",stagedIgnores="3:0,8:0")
        val receipt=Receipt("op","merge_groups",1,1,0,10,0,0)
        val after=before.afterReceipt(receipt,setOf(1,2,3))
        assertEquals(0L,after.current);assertEquals(0,after.page)
        assertEquals("4",after.remaining);assertEquals("5,1",after.detached)
        assertEquals("6",after.skipped);assertEquals("6:4",after.skipHistory)
        assertEquals("7:0",after.resume);assertEquals("8:0",after.stagedIgnores)
        assertEquals("5",before.afterReceipt(receipt.copy(action="name_groups"),setOf(1,2,3)).detached)
    }
    @Test fun detachedGroupsKeepOrderAndCurrentGroupUntilCompletion() {
        var state = QueueState("scope",100,"pass",current=7,page=4)
        for(id in listOf(101L,102L,103L)) state=state.afterReceipt(Receipt("$id","detach",7,0,id,1,0,0))
        assertEquals(7L,state.current)
        assertEquals(listOf(101L,102L,103L),state.detached.ids())
        state=state.afterReceipt(Receipt("name","name",7,0,0,8,1,0))
        assertEquals(0L,state.current)
        assertEquals(0,state.page)
        assertEquals(listOf(101L,102L,103L),state.detached.ids())
    }
    @Test fun swipeRejectsTapShortReverseDiagonalAndChangedDirection() {
        assertNull(swipeAction(-20f,0f,true,96f))
        assertEquals(SwipeAction.Back,swipeAction(120f,0f,true,96f))
        assertNull(swipeAction(-120f,-110f,true,96f))
        assertNull(swipeAction(-10f,-200f,true,96f))
        assertNull(swipeAction(-200f,-10f,false,96f))
        assertEquals(SwipeAction.Ignore,swipeAction(10f,-140f,false,96f))
        assertEquals(SwipeAction.Skip,swipeAction(-140f,10f,true,96f))
    }
}
