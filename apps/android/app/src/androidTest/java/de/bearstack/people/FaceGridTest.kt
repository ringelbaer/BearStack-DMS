package de.bearstack.people

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.SwipeAction
import de.bearstack.people.ui.FaceGrid
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class FaceGridTest {
    @get:Rule val compose=createComposeRule()
    @Test fun oneFaceHasNoDetachAndLargeTextRetainsAccessibleActions() {
        compose.setContent { MaterialTheme {
            val density=LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density,2f)) {
                FaceGrid(Person(1,"",1,1,10,listOf(10)),true,null,{_,_->null},{},{},{},{})
            }
        } }
        compose.onNodeWithContentDescription("Gesicht 1").assertIsDisplayed()
        compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen").assertCountEquals(0)
    }
    @Test fun gridNeverShowsMoreThanTheCurrentFourFacePage() {
        var count by mutableLongStateOf(4)
        compose.setContent { MaterialTheme { FaceGrid(Person(1,"",1,count,10,listOf(10,11,12,13)),true,null,{_,_->null},{},{},{},{}) } }
        for(total in listOf(4L,5L,600L)) {
            compose.runOnIdle {count=total}
            compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen").assertCountEquals(4)
            compose.onNodeWithTag("face-13").assertIsDisplayed()
        }
    }
    @Test fun holdingReleaseAndCancelDoNotSwipeOrDetach() {
        var held: Long?=null;var swipes=0;var detaches=0
        compose.setContent { MaterialTheme { FaceGrid(Person(1,"",1,4,10,listOf(10,11,12,13)),true,null,{_,_->null},
            {detaches++},{held=it},{},{swipes++}) } }
        compose.onNodeWithTag("face-10").performTouchInput { down(center) }
        compose.mainClock.advanceTimeBy(800)
        compose.runOnIdle {assertEquals(10L,held)}
        compose.onNodeWithTag("face-10").performTouchInput { moveBy(Offset(0f,-150f));up() }
        compose.runOnIdle {assertNull(held);assertEquals(0,swipes);assertEquals(0,detaches)}
        compose.onNodeWithTag("face-10").performTouchInput { down(center) }
        compose.mainClock.advanceTimeBy(800)
        compose.onNodeWithTag("face-10").performTouchInput { cancel() }
        compose.runOnIdle {assertNull(held)}
    }
    @Test fun explicitDetachAndDirectionalSwipesAreExclusive() {
        var detached: Long?=null;val swipes=mutableListOf<SwipeAction>()
        compose.setContent { MaterialTheme { FaceGrid(Person(1,"",1,4,10,listOf(10,11,12,13)),true,null,{_,_->null},
            {detached=it},{},{},{swipes+=it}) } }
        compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen")[0].performClick()
        compose.runOnIdle {assertEquals(10L,detached);assertTrue(swipes.isEmpty())}
        compose.onNodeWithTag("face-grid").performTouchInput { swipeLeft() }
        compose.runOnIdle {assertEquals(listOf(SwipeAction.Skip),swipes)}
    }
}
