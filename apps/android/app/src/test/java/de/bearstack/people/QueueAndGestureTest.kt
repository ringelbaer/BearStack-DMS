package de.bearstack.people

import de.bearstack.people.data.local.QueueState
import de.bearstack.people.data.remote.Receipt
import de.bearstack.people.people.*
import org.junit.Assert.*
import org.junit.Test

class QueueAndGestureTest {
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
        assertNull(swipeAction(120f,0f,true,96f))
        assertNull(swipeAction(-120f,-110f,true,96f))
        assertNull(swipeAction(-10f,-200f,true,96f))
        assertNull(swipeAction(-200f,-10f,false,96f))
        assertEquals(SwipeAction.Ignore,swipeAction(10f,-140f,false,96f))
        assertEquals(SwipeAction.Skip,swipeAction(-140f,10f,true,96f))
    }
}
