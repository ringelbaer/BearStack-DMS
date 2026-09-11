package de.bearstack.people.ui

import de.bearstack.people.text.uiStrings
import de.bearstack.people.R
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import de.bearstack.people.people.PeopleState
import de.bearstack.people.people.PeopleViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun MergeReviewScreen(state: PeopleState, vm: PeopleViewModel) {
    val text=uiStrings()
    val suggestion=state.mergeSuggestion
    var held by remember { mutableStateOf<Long?>(null) }
    var heldDismissed by remember { mutableStateOf(false) }
    var accessible by remember { mutableStateOf(false) }
    var zoom by remember { mutableFloatStateOf(0f) }
    var help by remember { mutableStateOf(false) }
    val distance=with(LocalDensity.current) {240.dp.toPx()}
    val drag: (Float)->Unit={zoom=zoomAfterDrag(zoom,it,distance)}
    val dialogEnabled=!state.busy && !state.unresolved
    val enabled=!state.naming && !state.busy && !state.unresolved && held==null && !help
    val lifecycle=LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(suggestion) {held=null;accessible=false;zoom=0f}
    LaunchedEffect(suggestion,lifecycle) {
        val faces=suggestion?.let {it.source.faces+it.target.faces} ?: emptyList()
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {vm.prefetchOriginals(faces)}
    }
    BackHandler {
        if(held!=null) {if(accessible) held=null else heldDismissed=true}
        else if(help) help=false else if(state.naming) vm.closeNaming() else vm.closeMergeReview()
    }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={TopAppBar(title={Text(text(R.string.people_similar_groups),maxLines=1,overflow=TextOverflow.Ellipsis)},
            navigationIcon={TextButton(onClick=vm::closeMergeReview,enabled=enabled) {Text(text(R.string.photos_back))}},
            actions={TextButton(onClick={help=true},enabled=held==null && !state.naming) {Text(text(R.string.common_help))}})
        },bottomBar={
            Surface(color=MaterialTheme.colorScheme.surfaceContainer,tonalElevation=2.dp) {
                Column(Modifier.fillMaxWidth()
                    .windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Horizontal+WindowInsetsSides.Bottom))
                    .padding(horizontal=16.dp,vertical=8.dp),verticalArrangement=Arrangement.spacedBy(4.dp)) {
                    suggestion?.score?.let {score ->
                        Text(text(R.string.people_merge_similarity,score),style=MaterialTheme.typography.bodySmall,
                            color=MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier=Modifier.align(Alignment.CenterHorizontally).testTag("merge-similarity"))
                    }
                    if(state.mergeSideResults.isNotEmpty()) {
                        Button(onClick=vm::nextMerge,enabled=enabled,modifier=Modifier.fillMaxWidth()) {Text(text(R.string.common_next))}
                    } else {
                    Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically) {
                        Button(onClick={vm.decideMerge(true)},enabled=enabled && suggestion!=null,modifier=Modifier.weight(1f)) {Text(text(R.string.people_merge))}
                        if(state.mergeNaming && suggestion?.source?.name=="" && suggestion.target.name.isEmpty()) {
                            IconButton(onClick=vm::startMergeNaming,enabled=enabled,
                                modifier=Modifier.semantics {contentDescription=text(R.string.people_merge_name)}) {
                                Icon(painterResource(R.drawable.ic_edit),contentDescription=null,modifier=Modifier.size(24.dp))
                            }
                        }
                    }
                    OutlinedButton(onClick={vm.decideMerge(false)},enabled=enabled && suggestion!=null,modifier=Modifier.fillMaxWidth()) {Text(text(R.string.people_keep_separate))}
                    }
                }
            }
        }) {padding ->
            Column(Modifier.fillMaxSize().padding(padding).testTag("merge-review")) {
                Box(Modifier.fillMaxWidth().height(4.dp)) {if(state.busy) LinearProgressIndicator(Modifier.fillMaxSize())}
                state.error?.let {error ->
                    Column(Modifier.fillMaxWidth().padding(12.dp)) {
                        Text(text(error),color=MaterialTheme.colorScheme.error,modifier=Modifier.semantics {liveRegion=LiveRegionMode.Polite})
                        Row {
                            TextButton(onClick=vm::retry,enabled=!state.busy && held==null) {Text(if(state.unresolved) text(R.string.people_check_pending) else text(R.string.photos_retry))}
                            TextButton(onClick=vm::switchConnection,enabled=!state.busy && held==null) {Text(text(R.string.connection_title))}
                        }
                    }
                }
                if(suggestion!=null) {
                    Text(text(R.string.people_same_person),style=MaterialTheme.typography.titleLarge,modifier=Modifier.padding(horizontal=16.dp,vertical=8.dp))
                    Row(Modifier.weight(1f).fillMaxWidth().padding(horizontal=12.dp,vertical=8.dp),horizontalArrangement=Arrangement.spacedBy(12.dp)) {
                        listOf(suggestion.source,suggestion.target).forEachIndexed {index,person ->
                            Column(Modifier.weight(1f).fillMaxHeight(),horizontalAlignment=Alignment.CenterHorizontally) {
                                Text(if(index==0) text(R.string.people_first_group) else text(R.string.people_second_group),style=MaterialTheme.typography.labelMedium)
                                Text(person.name.ifBlank {text(R.string.people_unnamed)},maxLines=2,overflow=TextOverflow.Ellipsis,style=MaterialTheme.typography.titleMedium)
                                Text(text.faces(person.count),style=MaterialTheme.typography.bodySmall)
                                if(state.mergeSideActions && person.name.isEmpty()) {
                                    val result=state.mergeSideResults[person.id]
                                    if(result!=null) Text(text(result),style=MaterialTheme.typography.bodySmall,
                                        modifier=Modifier.semantics {liveRegion=LiveRegionMode.Polite})
                                    else {
                                        OutlinedButton(onClick={vm.ignoreMergeSide(person.id)},enabled=enabled,
                                            modifier=Modifier.fillMaxWidth().semantics {contentDescription=text(if(index==0) R.string.people_merge_ignore_first else R.string.people_merge_ignore_second)}) {
                                            Text(text(R.string.people_merge_ignore))
                                        }
                                        IconButton(onClick={vm.startMergeSideNaming(person.id)},enabled=enabled,
                                            modifier=Modifier.semantics {contentDescription=text(if(index==0) R.string.people_merge_name_first else R.string.people_merge_name_second)}) {
                                            Icon(painterResource(R.drawable.ic_edit),contentDescription=null,modifier=Modifier.size(24.dp))
                                        }
                                    }
                                }
                                BoxWithConstraints(Modifier.weight(1f).fillMaxWidth(),contentAlignment=Alignment.Center) {
                                    Box(Modifier.size(minOf(maxWidth,maxHeight))) {
                                        FaceGrid(person,enabled,vm.images,vm::image,onDetach={},
                                            onHold={held=it;heldDismissed=false;zoom=0f},
                                            onZoom={held=it;heldDismissed=false;zoom=0f;accessible=true},
                                            onZoomDrag=drag,allowDetach=false,showPaths=false)
                                    }
                                }
                            }
                        }
                    }
                } else if(!state.busy && state.error==null) {
                    Column(Modifier.padding(24.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                        Text(text(R.string.people_no_similar),Modifier.semantics {liveRegion=LiveRegionMode.Polite},style=MaterialTheme.typography.titleLarge)
                        Text(text(R.string.people_suggestions_help))
                        TextButton(onClick=vm::retry,enabled=enabled) {Text(text(R.string.common_refresh))}
                    }
                }
            }
        }
        held?.takeUnless {heldDismissed}?.let {face ->
            val person=suggestion?.let {if(face in it.source.faces) it.source else it.target}
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                vm.images?.let {images ->
                    OriginalPhoto(vm.original(face),images,person?.faceBounds?.get(face),zoom,cacheKey=vm.originalKey(face),
                        onZoom={zoom=it},onDrag=drag,modifier=Modifier.weight(1f).fillMaxWidth(),
                        onNewTouch=if(accessible) null else {{heldDismissed=true}})
                }
                person?.facePaths?.get(face)?.takeIf {it.isNotEmpty()}?.let {
                    Text(text.photoPath(it),color=Color.White,style=MaterialTheme.typography.bodySmall,
                        modifier=Modifier.fillMaxWidth().heightIn(max=96.dp).verticalScroll(rememberScrollState()).testTag("original-photo-path"))
                }
                if(accessible) Button(onClick={held=null;accessible=false}) {Text(text(R.string.people_close_preview))}
            }
        }
    }
    if(state.naming) NamingDialog(state.copy(directory=false),vm,dialogEnabled)
    if(help) AlertDialog(onDismissRequest={help=false},title={Text(text(R.string.people_merge_help_title))},
        text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(text(R.string.people_merge_help))
            Text(text(R.string.people_merge_next_help))
            Text(text(R.string.people_merge_name_help))
            Text(text(R.string.people_merge_side_help))
            Text(text(R.string.people_merge_preview_help))
        }},confirmButton={TextButton(onClick={help=false}) {Text(text(R.string.photos_close))}})
}
