package de.bearstack.people.photos

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.*
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.sample
import kotlin.math.roundToInt

@OptIn(kotlinx.coroutines.FlowPreview::class)
@Composable internal fun GalleryFastScroller(grid: LazyGridState,state: PhotosState,bottomPadding: androidx.compose.ui.unit.Dp,
    onSeek: (Int)->Unit) {
    val count=galleryItemCount(state)
    if(count<2) return
    val seek by rememberUpdatedState(onSeek)
    val latestCount by rememberUpdatedState(count)
    var dragging by remember {mutableStateOf(false)}
    var dragFraction by remember {mutableFloatStateOf(0f)}
    var visible by remember {mutableStateOf(false)}
    val fraction by remember(grid,state,count) {derivedStateOf {
        val items=grid.layoutInfo.visibleItemsInfo
        val position=items.firstNotNullOfOrNull {galleryItemPosition(state,it.key.toString())} ?: 0
        val atEnd=!grid.canScrollForward && items.any {galleryItemPosition(state,it.key.toString())==count-1}
        if(atEnd) 1f else position.toFloat()/(count-1)
    }}
    val currentFraction by rememberUpdatedState(fraction)
    val label=stringResource(R.string.photos_fast_scroll)
    LaunchedEffect(grid.isScrollInProgress,dragging,state.seekLoading) {
        if(grid.isScrollInProgress || dragging || state.seekLoading) visible=true
        else {delay(1500);visible=false}
    }
    // Bound requests while the finger moves; release always sends the final target.
    LaunchedEffect(Unit) {
        snapshotFlow {if(dragging) dragFraction else null}.filterNotNull().sample(100).collect {
            if(dragging) seek((it*(latestCount-1)).roundToInt())
        }
    }
    if(!visible) return
    BoxWithConstraints(Modifier.fillMaxSize().padding(bottom=bottomPadding)) {
        val height=with(LocalDensity.current) {(maxHeight-56.dp).coerceAtLeast(1.dp).toPx()}
        val shown=if(dragging) dragFraction else fraction.coerceIn(0f,1f)
        Box(Modifier.align(Alignment.TopEnd).offset {IntOffset(0,(shown*height).roundToInt())}
            .size(width=48.dp,height=56.dp).testTag("gallery-fast-scroll")
            .background(MaterialTheme.colorScheme.secondaryContainer,RoundedCornerShape(topStart=28.dp,bottomStart=28.dp))
            .semantics {
                contentDescription=label
                progressBarRangeInfo=ProgressBarRangeInfo(shown,0f..1f)
                setProgress {if(it.isFinite()) {seek((it.coerceIn(0f,1f)*(count-1)).roundToInt());true} else false}
            }
            .pointerInput(height) {
                detectVerticalDragGestures(onDragStart={dragFraction=currentFraction.coerceIn(0f,1f);dragging=true},
                    onDragEnd={seek((dragFraction*(latestCount-1)).roundToInt());dragging=false},
                    onDragCancel={dragging=false}) {change,amount ->
                    change.consume();dragFraction=(dragFraction+amount/height).coerceIn(0f,1f)
                }
            },contentAlignment=Alignment.Center) {
            Icon(painterResource(R.drawable.ic_fast_scroll),null)
        }
    }
}
