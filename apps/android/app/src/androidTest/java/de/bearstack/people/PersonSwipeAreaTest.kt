package de.bearstack.people

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.dp
import de.bearstack.people.people.SwipeAction
import de.bearstack.people.ui.PersonSwipeArea
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test

class PersonSwipeAreaTest {
    @get:Rule val compose=createComposeRule()

    @Test fun emptyBackgroundAndPaddingAcceptBothDirectionsOnce() {
        val actions=mutableListOf<SwipeAction>()
        compose.setContent { MaterialTheme { PersonSwipeArea(1,true,{actions+=it},Modifier.fillMaxSize()) {
            Text("Person")
        } } }
        // Both gestures start in empty viewport space, below all content.
        compose.onNodeWithTag("person-swipe-area").performTouchInput {
            swipe(Offset(width*.9f,height*.8f),Offset(width*.1f,height*.8f))
        }
        compose.onNodeWithTag("person-swipe-area").performTouchInput {
            swipe(Offset(2f,height*.85f),Offset(2f,height*.15f))
        }
        compose.onNodeWithTag("person-swipe-area").performTouchInput {swipeRight()}
        compose.runOnIdle {assertEquals(listOf(SwipeAction.Skip,SwipeAction.Ignore,SwipeAction.Back),actions)}
    }

    @Test fun tapsShortDragsCancellationAndDisabledGesturesNeverSubmit() {
        var enabled by mutableStateOf(true)
        var clicks=0;var swipes=0
        compose.setContent { MaterialTheme { PersonSwipeArea(1,enabled,{swipes++},Modifier.fillMaxSize()) {
            Button(onClick={clicks++}) {Text("Aktion")}
        } } }
        compose.onNodeWithText("Aktion").performClick()
        compose.onNodeWithTag("person-swipe-area").performTouchInput {
            swipe(center,center+Offset(-20f,0f))
        }
        compose.runOnIdle {assertEquals("tap or short drag",0,swipes)}
        compose.onNodeWithTag("person-swipe-area").performTouchInput {
            down(center);moveBy(Offset(-300f,0f));cancel()
        }
        compose.runOnIdle {assertEquals("cancelled drag",0,swipes)}
        compose.onNodeWithTag("person-swipe-area").performTouchInput {
            down(center);moveBy(Offset(-120f,0f));moveTo(center+Offset(300f,0f));up()
        }
        compose.runOnIdle {assertEquals("direction reversal",0,swipes)}
        compose.runOnIdle {enabled=false}
        compose.onNodeWithTag("person-swipe-area").performTouchInput {swipeLeft();swipeUp();swipeRight()}
        compose.runOnIdle {assertEquals(1,clicks);assertEquals(0,swipes)}
    }

    @Test fun overflowingContentScrollsBeforeAnUpwardSwipeCanIgnore() {
        val actions=mutableListOf<SwipeAction>()
        compose.setContent { MaterialTheme { PersonSwipeArea(1,true,{actions+=it},Modifier.fillMaxSize()) {
            Spacer(Modifier.height(1500.dp))
            Text("Ende",Modifier.testTag("end"))
        } } }
        compose.onNodeWithTag("person-swipe-area").performTouchInput {swipeUp()}
        compose.runOnIdle {assertTrue(actions.isEmpty())}
        compose.onNodeWithTag("end").performScrollTo()
        compose.onNodeWithTag("person-swipe-area").performTouchInput {swipeUp()}
        compose.runOnIdle {assertEquals(listOf(SwipeAction.Ignore),actions)}
    }
}
