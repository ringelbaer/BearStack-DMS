package de.bearstack.people.people

import kotlin.math.abs

enum class SwipeAction { Ignore, Skip }
/** Drag direction is fixed after touch slop; a perpendicular or reverse drag never confirms. */
fun swipeAction(dx: Float, dy: Float, horizontal: Boolean, threshold: Float): SwipeAction? = when {
    horizontal && dx < -threshold && abs(dx) > abs(dy) * 1.3f -> SwipeAction.Skip
    !horizontal && dy < -threshold && abs(dy) > abs(dx) * 1.3f -> SwipeAction.Ignore
    else -> null
}
