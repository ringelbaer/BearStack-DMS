package de.bearstack.people

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.Modifier
import de.bearstack.people.ui.PersonSwipeArea
import androidx.compose.runtime.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.compose.ui.text.TextLayoutResult
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.SwipeAction
import de.bearstack.people.ui.FaceGrid
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class FaceGridTest {
    @get:Rule val compose=createComposeRule()
    @Test fun shortTapSelectsButHoldDragAndCancellationDoNot() {
        var taps=0;var held by mutableStateOf<Long?>(null)
        compose.setGermanContent {MaterialTheme {
            FaceGrid(Person(1,"",1,1,10,listOf(10)),held==null,null,{_,_->null},{},{held=it},{},allowDetach=false,onTap={taps++})
        }}
        val face=compose.onNodeWithTag("face-10")
        face.performTouchInput {click()}
        compose.runOnIdle {assertEquals(1,taps)}
        face.performTouchInput {down(center)};compose.mainClock.advanceTimeBy(300)
        compose.runOnIdle {assertEquals(10L,held)}
        face.performTouchInput {moveBy(Offset(0f,-40f));up()}
        compose.runOnIdle {assertNull(held);assertEquals(1,taps)}
        face.performTouchInput {down(center);cancel()}
        compose.runOnIdle {assertEquals(1,taps)}
    }
    @Test fun fullGalleryPathWrapsBelowTheImageAtLargeFontSize() {
        val path="Fotos / 11.05.2026 · Urlaub / Ein sehr langer Unterordner / IMG_1234.jpg"
        compose.setGermanContent {MaterialTheme {
            val density=LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density,2f)) {
                FaceGrid(Person(1,"",1,1,10,listOf(10),facePaths=mapOf(10L to path)),true,null,{_,_->null},{},{},{})
            }
        }}
        val caption=compose.onNodeWithText(path).assertIsDisplayed()
        caption.performSemanticsAction(SemanticsActions.GetTextLayoutResult) { action ->
            val results=mutableListOf<TextLayoutResult>();assertTrue(action(results))
            assertFalse(results.single().hasVisualOverflow);assertTrue(results.single().lineCount>1)
        }
        assertTrue(caption.fetchSemanticsNode().boundsInRoot.top >= compose.onNodeWithTag("face-10").fetchSemanticsNode().boundsInRoot.bottom)
    }
    @Test fun accessibleOriginalPreviewTargetsTheSelectedFaceWithoutEditing() {
        var preview: Long?=null
        var edits=0
        compose.setGermanContent { MaterialTheme {
            FaceGrid(Person(1,"",1,4,10,listOf(10,11,12,13)),true,null,{_,_->null},
                {edits++},{},{preview=it})
        } }
        val actions = compose.onNodeWithTag("face-12").fetchSemanticsNode().config[SemanticsActions.CustomActions]
        compose.runOnIdle {
            assertTrue(actions.single { it.label=="Originalfoto anzeigen" }.action())
        }
        compose.runOnIdle {assertEquals(12L,preview);assertEquals(0,edits)}
    }
    @Test fun oneFaceHasNoDetachAndLargeTextRetainsAccessibleActions() {
        compose.setGermanContent { MaterialTheme {
            val density=LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density,2f)) {
                FaceGrid(Person(1,"",1,1,10,listOf(10)),true,null,{_,_->null},{},{},{})
            }
        } }
        compose.onNodeWithContentDescription("Gesicht 1").assertIsDisplayed()
        compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen").assertCountEquals(0)
    }
    @Test fun gridNeverShowsMoreThanTheCurrentFourFacePage() {
        var count by mutableLongStateOf(4)
        compose.setGermanContent { MaterialTheme { FaceGrid(Person(1,"",1,count,10,listOf(10,11,12,13)),true,null,{_,_->null},{},{},{}) } }
        for(total in listOf(4L,5L,600L)) {
            compose.runOnIdle {count=total}
            compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen").assertCountEquals(4)
            compose.onNodeWithTag("face-13").assertIsDisplayed()
        }
    }
    @Test fun holdingReleaseAndCancelDoNotSwipeOrDetach() {
        var held by mutableStateOf<Long?>(null);var swipes=0;var detaches=0;var drag=0f
        compose.setGermanContent { MaterialTheme { PersonSwipeArea(1,held==null,{swipes++},Modifier.fillMaxSize()) {
            FaceGrid(Person(1,"",1,4,10,listOf(10,11,12,13)),held==null,null,{_,_->null},
                {detaches++},{held=it},{},onZoomDrag={drag+=it})
        } } }
        compose.onNodeWithTag("face-10").performTouchInput { down(center) }
        compose.mainClock.advanceTimeBy(300)
        compose.runOnIdle {assertEquals(10L,held)}
        compose.onNodeWithTag("face-10").performTouchInput { moveBy(Offset(0f,-300f)) }
        compose.runOnIdle {assertEquals(10L,held);assertEquals(-300f,drag,.01f)}
        compose.onNodeWithTag("face-10").performTouchInput { moveBy(Offset(0f,100f)) }
        compose.runOnIdle {assertEquals(-200f,drag,.01f)}
        compose.onNodeWithTag("face-10").performTouchInput { up() }
        compose.runOnIdle {assertNull(held);assertEquals(0,swipes);assertEquals(0,detaches)}
        compose.onNodeWithTag("face-10").performTouchInput { down(center) }
        compose.mainClock.advanceTimeBy(800)
        compose.onNodeWithTag("face-10").performTouchInput { cancel() }
        compose.runOnIdle {assertNull(held)}
    }
    @Test fun explicitDetachAndDirectionalSwipesAreExclusive() {
        var detached: Long?=null;val swipes=mutableListOf<SwipeAction>()
        compose.setGermanContent { MaterialTheme { PersonSwipeArea(1,true,{swipes+=it},Modifier.fillMaxSize()) {
            FaceGrid(Person(1,"",1,4,10,listOf(10,11,12,13)),true,null,{_,_->null},{detached=it},{},{})
        } } }
        compose.onAllNodesWithContentDescription("Dieses Gesicht einzeln benennen")[0].performClick()
        compose.runOnIdle {assertEquals(10L,detached);assertTrue(swipes.isEmpty())}
        compose.onNodeWithTag("face-grid").performTouchInput { swipeLeft() }
        compose.runOnIdle {assertEquals(listOf(SwipeAction.Skip),swipes)}
    }
}
