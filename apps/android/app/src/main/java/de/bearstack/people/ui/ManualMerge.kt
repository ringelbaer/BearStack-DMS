package de.bearstack.people.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import de.bearstack.people.R
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.*
import de.bearstack.people.text.uiStrings

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ManualMergeScreen(state: PeopleState, vm: PeopleViewModel, controller: ManualMergeController) {
    val s by controller.state.collectAsStateWithLifecycle()
    val text=uiStrings()
    var held by remember {mutableStateOf<Person?>(null)}
    var dismissed by remember {mutableStateOf(false)}
    var accessible by remember {mutableStateOf(false)}
    var zoom by remember {mutableFloatStateOf(0f)}
    val distance=with(LocalDensity.current) {240.dp.toPx()}
    val drag:(Float)->Unit={zoom=zoomAfterDrag(zoom,it,distance)}
    val enabled=!state.busy && !state.unresolved && held==null
    val browsing=enabled && !s.naming
    val lifecycle=LocalLifecycleOwner.current.lifecycle
    BackHandler {
        if(held!=null) {if(accessible) held=null else dismissed=true}
        else if(s.naming && enabled) controller.closeNaming() else vm.closeManualMerge()
    }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={TopAppBar(title={Text(text(R.string.people_manual_merge),maxLines=1,overflow=TextOverflow.Ellipsis)},
            navigationIcon={TextButton(onClick=vm::closeManualMerge,enabled=browsing) {Text(text(R.string.photos_back))}},
            actions={TextButton(onClick={controller.reset()},enabled=browsing) {Text(text(R.string.common_refresh))}})
        },bottomBar={if(s.selected.isNotEmpty()) Surface(color=MaterialTheme.colorScheme.surfaceContainer,tonalElevation=2.dp) {
            Column(Modifier.fillMaxWidth().windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Horizontal+WindowInsetsSides.Bottom))
                .padding(horizontal=16.dp,vertical=8.dp),verticalArrangement=Arrangement.spacedBy(4.dp)) {
                Row(verticalAlignment=Alignment.CenterVertically) {
                    Text(text(R.string.people_manual_selected,s.selected.size),Modifier.weight(1f))
                    TextButton(onClick=controller::clearSelection,enabled=browsing) {Text(text(R.string.people_manual_clear))}
                }
                if(s.selected.size==60) Text(text(R.string.people_manual_merge_limit),style=MaterialTheme.typography.bodySmall)
                if(s.selected.size>=2) {
                    Text(if(s.retainedName.isEmpty()) text(R.string.people_manual_stays_unnamed)
                        else text(R.string.people_manual_keeps_name,s.retainedName),style=MaterialTheme.typography.bodySmall)
                    Button(onClick={vm.combineManualGroups()},enabled=browsing,modifier=Modifier.fillMaxWidth()) {Text(text(R.string.people_manual_combine))}
                    OutlinedButton(onClick=controller::startNaming,enabled=browsing,modifier=Modifier.fillMaxWidth()) {Text(text(R.string.people_manual_combine_name))}
                }
            }
        }}) {padding ->
            Column(Modifier.fillMaxSize().padding(padding)) {
                Row(Modifier.fillMaxWidth().padding(horizontal=16.dp),verticalAlignment=Alignment.CenterVertically) {
                    Text(text(R.string.people_manual_include_named),Modifier.weight(1f))
                    Switch(s.includeNamed,{controller.reset(it)},enabled=browsing,
                        modifier=Modifier.semantics {contentDescription=text(R.string.people_manual_include_named)})
                }
                Text(text(R.string.people_manual_help),style=MaterialTheme.typography.bodySmall,modifier=Modifier.padding(horizontal=16.dp,vertical=4.dp))
                Box(Modifier.fillMaxWidth().height(4.dp)) {if(s.loading || state.busy) LinearProgressIndicator(Modifier.fillMaxSize())}
                (state.error ?: s.error)?.let {error ->
                    Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically) {
                        Text(text(error),color=MaterialTheme.colorScheme.error,modifier=Modifier.weight(1f).semantics {liveRegion=LiveRegionMode.Polite})
                        TextButton(onClick={if(state.error!=null) vm.retry() else controller.retry()},enabled=!state.busy && held==null) {
                            Text(text(if(state.unresolved) R.string.people_check_pending else R.string.photos_retry))
                        }
                    }
                }
                key(s.epoch) {
                    val grid=rememberLazyGridState()
                    val latest by rememberUpdatedState(s)
                    val canBrowse by rememberUpdatedState(browsing)
                    LaunchedEffect(grid,lifecycle) {
                        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
                            snapshotFlow {Triple(grid.layoutInfo.visibleItemsInfo.firstOrNull()?.index ?: 0,
                                grid.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: 0,latest to canBrowse)}.collect {view ->
                                if(view.third.second) controller.visible(view.first,view.second)
                            }
                        }
                    }
                    LazyVerticalGrid(GridCells.Adaptive(150.dp),state=grid,modifier=Modifier.fillMaxSize().testTag("manual-merge-grid"),
                        contentPadding=PaddingValues(12.dp),horizontalArrangement=Arrangement.spacedBy(8.dp),verticalArrangement=Arrangement.spacedBy(8.dp)) {
                        items(s.count,key={it}) {index ->
                            val person=s.person(index)
                            if(person==null) {
                                if(index/20 !in s.pages) Box(Modifier.fillMaxWidth().aspectRatio(1f).background(MaterialTheme.colorScheme.surfaceContainer))
                                else Spacer(Modifier.height(0.dp))
                            } else {
                                val selected=s.selected.any {it.id==person.id}
                                Surface(shape=RoundedCornerShape(16.dp),border=if(selected) BorderStroke(3.dp,MaterialTheme.colorScheme.primary) else null,
                                    modifier=Modifier.testTag("manual-group-${person.id}").semantics(mergeDescendants=true) {this.selected=selected}) {
                                    Column(Modifier.padding(4.dp)) {
                                        FaceGrid(person.copy(faces=listOf(person.faceId)),browsing,vm.images,vm::image,onDetach={},
                                            onHold={held=if(it==null) null else person;dismissed=false;zoom=0f},
                                            onZoom={held=person;dismissed=false;zoom=0f;accessible=true},onZoomDrag=drag,
                                            allowDetach=false,showPaths=false,onTap={controller.select(person)})
                                        Text((if(selected) "✓ " else "")+person.name.ifBlank {text(R.string.people_unnamed)},maxLines=1,overflow=TextOverflow.Ellipsis,
                                            modifier=Modifier.fillMaxWidth().clickable(enabled=browsing) {controller.select(person)}.padding(4.dp))
                                        Text(text(R.string.people_face_count_id,text.faces(person.count),person.id),style=MaterialTheme.typography.bodySmall,
                                            modifier=Modifier.padding(horizontal=4.dp,vertical=2.dp))
                                    }
                                }
                            }
                        }
                        if(s.count==0 && !s.loading && s.error==null) item(span={GridItemSpan(maxLineSpan)}) {
                            Text(text(R.string.people_manual_empty),Modifier.padding(24.dp))
                        }
                    }
                }
            }
        }
        held?.takeUnless {dismissed}?.let {person ->
            Column(Modifier.fillMaxSize().testTag("manual-merge-preview").background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),horizontalAlignment=Alignment.CenterHorizontally) {
                vm.images?.let {images ->
                    OriginalPhoto(vm.original(person.faceId),images,person.faceBounds[person.faceId],zoom,cacheKey=vm.originalKey(person.faceId),
                        onZoom={zoom=it},onDrag=drag,modifier=Modifier.weight(1f).fillMaxWidth(),onNewTouch=if(accessible) null else {{dismissed=true}})
                }
                person.facePaths[person.faceId]?.let {Text(text.photoPath(it),color=Color.White,modifier=Modifier.heightIn(max=96.dp).verticalScroll(rememberScrollState()))}
                if(accessible) Button(onClick={held=null;accessible=false}) {Text(text(R.string.people_close_preview))}
            }
        }
    }
    if(s.naming) AlertDialog(onDismissRequest={if(enabled) controller.closeNaming()},title={Text(text(R.string.people_manual_combine_name))},
        text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            Text(text(R.string.people_manual_selected,s.selected.size))
            OutlinedTextField(s.name,controller::nameChanged,label={Text(text(R.string.people_name))},singleLine=true,enabled=enabled)
            if(s.duplicateName) Text(text(R.string.people_name_exists))
            state.error?.let {Text(text(it),color=MaterialTheme.colorScheme.error)}
            if(state.unresolved) TextButton(onClick=vm::retry,enabled=!state.busy) {Text(text(R.string.people_check_pending))}
        }},confirmButton={TextButton(onClick={vm.combineManualGroups(true)},enabled=enabled && s.name.isNotBlank()) {
            Text(text(if(s.duplicateName) R.string.people_name_separately else R.string.people_manual_combine_name))
        }},dismissButton={TextButton(onClick=controller::closeNaming,enabled=enabled) {Text(text(R.string.photos_cancel))}})
}
