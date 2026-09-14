package de.bearstack.people

import de.bearstack.people.data.local.QueueState
import de.bearstack.people.data.remote.Receipt
import de.bearstack.people.people.*
import org.junit.Assert.*
import org.junit.Test

class QueueAndGestureTest {
    @Test fun receiptsPreserveCurrentUntilItsGroupIsCompleted() {
        val before=QueueState("scope",100,"pass",current=7,page=4)
        for(action in listOf("detach","unassign_faces","rename","favorite","reject_merge")) {
            assertEquals(before,before.afterReceipt(Receipt("op",action,7,0,101,1,0,0)))
        }
        val receipt=Receipt("op","name",7,0,0,1,0,0)
        assertEquals(before.copy(current=0,page=0),before.afterReceipt(receipt))
        assertEquals(before,before.afterReceipt(receipt.copy(source=8)))
        assertEquals(before.copy(current=0,page=0),before.afterReceipt(receipt.copy(action="merge_groups"),setOf(7,8)))
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
