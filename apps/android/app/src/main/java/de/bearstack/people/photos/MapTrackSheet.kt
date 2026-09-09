package de.bearstack.people.photos

import androidx.compose.foundation.background
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import de.bearstack.people.text.uiStrings

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MapTrackSheet(tracks: MapTracks,route: MapPhotoRoute?=null,onClose: ()->Unit) {
    val state by tracks.state.collectAsState()
    val latest by rememberUpdatedState(state)
    val list=rememberLazyListState()
    val text=uiStrings()
    val routeState=route?.state?.collectAsState()?.value
    LaunchedEffect(tracks) {tracks.open()}
    DisposableEffect(tracks) {onDispose {tracks.hideList()}}
    LaunchedEffect(list,tracks) {
        snapshotFlow {
            val files=latest.files
            val visible=list.layoutInfo.visibleItemsInfo.mapNotNull {item ->files.indexOfFirst {it.path==item.key}.takeIf {it>=0}}
            val first=visible.minOrNull();val last=visible.maxOrNull()
            Triple(latest,first,last)
        }.collect {(current,first,last) ->
            val files=current.files
            tracks.visible(if(first!=null && last!=null) files.subList(first,last+1).mapTo(HashSet()) {it.path} else emptySet())
            if(!current.listLoading && current.listError==null && current.pages.isNotEmpty()) {
                if(first!=null && first<4 && current.pages.first().hasPrevious) tracks.load(true)
                else if((last!=null && last>=files.size-4 || files.isEmpty()) && current.pages.last().hasNext) tracks.load(false)
            }
        }
    }
    ModalBottomSheet(onDismissRequest=onClose,sheetState=rememberModalBottomSheetState(skipPartiallyExpanded=true)) {
        Column(Modifier.fillMaxWidth().fillMaxHeight(.85f).padding(horizontal=16.dp)) {
            Text(stringResource(R.string.photos_map_layers),style=MaterialTheme.typography.headlineSmall)
            if(state.selected.isNotEmpty()) {
                LazyRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                    items(state.selected.values.toList(),key={it.path}) {file ->
                        InputChip(selected=true,onClick={tracks.toggle(file)},label={Text(file.name,maxLines=1,overflow=TextOverflow.Ellipsis)},
                            leadingIcon={Box(Modifier.size(12.dp).background(trackColor(file.path),CircleShape))})
                    }
                }
                TextButton(onClick=tracks::clear) {Text(stringResource(R.string.photos_tracks_clear))}
                if(state.errors.isNotEmpty()) TextButton(onClick=tracks::refresh) {Text(stringResource(R.string.photos_retry))}
            }
            if(state.selected.size>=256) Text(stringResource(R.string.photos_tracks_limit),style=MaterialTheme.typography.bodySmall)
            if(!state.ready) {
                Text(stringResource(R.string.photos_tracks_index_pending),style=MaterialTheme.typography.bodySmall)
                TextButton(onClick=tracks::retryList) {Text(stringResource(R.string.photos_retry))}
            }
            Box(Modifier.fillMaxWidth().height(3.dp)) {if(state.listLoading) LinearProgressIndicator(Modifier.fillMaxSize())}
            state.listError?.let {error ->
                Row(verticalAlignment=Alignment.CenterVertically) {
                    Text(text(error),Modifier.weight(1f))
                    TextButton(onClick=tracks::retryList) {Text(stringResource(R.string.photos_retry))}
                }
            }
            if(state.pages.isNotEmpty() && state.files.isEmpty() && !state.listLoading && !state.pages.last().hasNext)
                Text(stringResource(R.string.photos_tracks_empty),Modifier.padding(vertical=16.dp))
            LazyColumn(Modifier.fillMaxWidth().weight(1f).testTag("map-layer-list"),state=list) {
                if(route!=null && routeState!=null) item(key="photo-route") {
                    ListItem(headlineContent={Text(stringResource(R.string.photos_route_title))},
                        supportingContent={Text(stringResource(R.string.photos_route_help))},
                        leadingContent={Checkbox(checked=routeState.enabled,onCheckedChange=null)},
                        trailingContent={if(routeState.loading) CircularProgressIndicator(Modifier.size(20.dp),strokeWidth=2.dp)
                            else if(routeState.enabled) Box(Modifier.size(12.dp).background(trackColor(photoRoutePath),CircleShape))},
                        modifier=Modifier.toggleable(value=routeState.enabled,role=Role.Checkbox) {route.enable(it)})
                    routeState.error?.let {error ->
                        Text(text(error),color=MaterialTheme.colorScheme.error)
                        TextButton(onClick=route::refresh) {Text(stringResource(R.string.photos_retry))}
                    }
                    routeState.data?.let {data ->
                        Text(stringResource(R.string.photos_route_summary,data.totalMedia,data.radiusMeters),style=MaterialTheme.typography.bodySmall)
                    }
                    HorizontalDivider(Modifier.padding(vertical=12.dp))
                }
                item(key="gpx-title") {Text(stringResource(R.string.photos_tracks_title),style=MaterialTheme.typography.titleMedium)}
                item(key="track-help") {Text(stringResource(R.string.photos_tracks_help),Modifier.padding(vertical=8.dp),style=MaterialTheme.typography.bodySmall)}
                items(state.files,key={it.path}) {file ->
                    val selected=file.path in state.selected
                    val enabled=selected || state.selected.size<256
                    ListItem(headlineContent={Text(file.name,maxLines=2,overflow=TextOverflow.Ellipsis)},
                        supportingContent={Text(state.errors[file.path]?.let {text(it)} ?: file.path.substringBeforeLast('/',""),maxLines=2,overflow=TextOverflow.Ellipsis)},
                        leadingContent={Checkbox(checked=selected,onCheckedChange=null,enabled=enabled)},
                        trailingContent={if(file.path in state.loading) CircularProgressIndicator(Modifier.size(20.dp),strokeWidth=2.dp)
                            else if(selected) Box(Modifier.size(12.dp).background(trackColor(file.path),CircleShape))},
                        modifier=Modifier.toggleable(value=selected,enabled=enabled,role=Role.Checkbox) {tracks.toggle(file)})
                }
            }
        }
    }
}
