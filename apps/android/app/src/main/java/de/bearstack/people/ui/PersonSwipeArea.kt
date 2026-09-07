package de.bearstack.people.ui

import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import de.bearstack.people.people.SwipeAction
import de.bearstack.people.people.swipeAction
import kotlin.math.abs

/** One gesture owner for the grid, spacing and the remaining viewport. */
@Composable
fun PersonSwipeArea(gestureKey: Any?, enabled: Boolean, onSwipe: (SwipeAction) -> Unit,
    modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    val scroll = rememberScrollState()
    val threshold = with(LocalDensity.current) { 96.dp.toPx() }
    val active by rememberUpdatedState(enabled)
    val currentSwipe by rememberUpdatedState(onSwipe)
    LaunchedEffect(gestureKey) { scroll.scrollTo(0) }
    Box(modifier.testTag("person-swipe-area").pointerInput(gestureKey, threshold) {
        awaitEachGesture {
            val down = awaitFirstDown(requireUnconsumed=false, pass=PointerEventPass.Initial)
            var blocked = !active
            var horizontal: Boolean? = null
            var claimed = false
            var right = false
            var dx = 0f
            var dy = 0f
            do {
                // Decide before the child scroll container. Taps remain unconsumed;
                // claimed drags cancel child clicks and cannot also scroll the page.
                val event = awaitPointerEvent(PointerEventPass.Initial)
                val change = event.changes.firstOrNull { it.id == down.id }
                if (change == null) { blocked = true; break }
                // Compose represents cancellation with an already consumed release.
                if (change.isConsumed || !active || event.changes.count { it.pressed } > 1) blocked = true
                dx = change.position.x - down.position.x
                dy = change.position.y - down.position.y
                if (!blocked && horizontal == null && maxOf(abs(dx),abs(dy)) > viewConfiguration.touchSlop) {
                    horizontal = abs(dx) > abs(dy)
                    right = dx > 0
                    // When large text overflows, an upward drag scrolls first.
                    // It must never turn into an ignore halfway through a gesture.
                    claimed = horizontal || dy < 0 && !scroll.canScrollForward
                    if (!claimed) blocked = true
                }
                if (claimed) change.consume()
                if (!change.pressed) break
            } while (true)
            if (!blocked && claimed && active && (horizontal!=true || (dx>0)==right)) horizontal?.let {
                swipeAction(dx,dy,it,threshold)?.let(currentSwipe)
            }
        }
    }) {
        Column(Modifier.fillMaxSize().verticalScroll(scroll).padding(16.dp),
            verticalArrangement=Arrangement.spacedBy(16.dp), content=content)
    }
}
