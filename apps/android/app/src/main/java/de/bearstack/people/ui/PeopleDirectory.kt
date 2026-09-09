package de.bearstack.people.ui

import de.bearstack.people.text.*
import de.bearstack.people.R
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.lazy.grid.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.*
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import de.bearstack.people.people.PeopleState
import de.bearstack.people.people.PeopleViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PeopleDirectoryScreen(state: PeopleState, vm: PeopleViewModel) {
    val text=uiStrings()
    val person=state.selectedPerson
    var held by remember { mutableStateOf<Long?>(null) }
    var heldDismissed by remember { mutableStateOf(false) }
    var accessible by remember { mutableStateOf(false) }
    var zoom by remember { mutableFloatStateOf(0f) }
    var browserError by remember(person?.id) { mutableStateOf<UiText?>(null) }
    val context=LocalContext.current
    val zoomDistance=with(LocalDensity.current) { 240.dp.toPx() }
    val zoomDrag: (Float) -> Unit = { zoom=zoomAfterDrag(zoom,it,zoomDistance) }
    val enabled=!state.busy && !state.unresolved && held==null
    val browsing=enabled && !state.naming && state.removeFace==null
    val listState=rememberLazyListState()
    val gridState=rememberLazyGridState()
    LaunchedEffect(state.namedQuery) {listState.scrollToItem(0)}
    LaunchedEffect(person?.id) {gridState.scrollToItem(0)}
    LaunchedEffect(state.namedPeople.size,state.namedHasNext,browsing,state.error,person?.id) {
        if(person==null && browsing && state.error==null && state.namedHasNext) snapshotFlow {
            val info=listState.layoutInfo
            (info.visibleItemsInfo.lastOrNull()?.index ?: -1)>=info.totalItemsCount-3
        }.collect { if(it) vm.moreNamedPeople() }
    }
    LaunchedEffect(person?.faces?.size,person?.count,browsing,state.error,person?.id) {
        if(person!=null && browsing && state.error==null && person.faces.size<person.count) snapshotFlow {
            val info=gridState.layoutInfo
            (info.visibleItemsInfo.lastOrNull()?.index ?: -1)>=info.totalItemsCount-7
        }.collect {if(it) vm.morePersonFaces()}
    }
    LaunchedEffect(person?.id,person?.revision,person?.offset) { held=null;accessible=false }
    BackHandler {
        if(held!=null) { if(accessible) held=null else heldDismissed=true }
        else if(state.removeFace!=null) vm.cancelUnassign()
        else if(state.naming) vm.closeNaming()
        else if(person!=null) vm.closePerson() else vm.closeDirectory()
    }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={ TopAppBar(title={Text(text(R.string.people_directory))},navigationIcon={
            TextButton(onClick={if(person!=null) vm.closePerson() else vm.closeDirectory()},enabled=enabled) { Text(text(R.string.photos_back)) }
        },actions={
            if(state.error!=null) TextButton(onClick=vm::switchConnection,enabled=!state.busy && held==null) { Text(text(R.string.connection_title)) }
        }) }) { padding ->
            Column(Modifier.fillMaxSize().padding(padding).imePadding()) {
                Box(Modifier.fillMaxWidth().height(4.dp)) { if(state.busy) LinearProgressIndicator(Modifier.fillMaxSize()) }
                state.error?.let { error ->
                    Card(Modifier.fillMaxWidth().padding(12.dp),colors=CardDefaults.cardColors(containerColor=MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(12.dp)) {
                            Text(text(error),Modifier.semantics { liveRegion=LiveRegionMode.Polite })
                            TextButton(onClick=vm::retry,enabled=!state.busy && held==null) {
                                Text(if(state.unresolved) text(R.string.people_check_pending) else text(R.string.photos_retry))
                            }
                        }
                    }
                }
                if(person==null) {
                    OutlinedTextField(state.namedQuery,vm::namedQueryChanged,label={Text(text(R.string.people_search))},singleLine=true,
                        enabled=state.namedSearch && !state.unresolved,modifier=Modifier.fillMaxWidth().padding(horizontal=16.dp,vertical=8.dp),
                        trailingIcon={if(state.namedQuery.isNotEmpty()) TextButton(onClick={vm.namedQueryChanged("")}) {Text(text(R.string.common_clear))}})
                    if(!state.namedSearch && !state.busy) Text(text(R.string.people_search_version),Modifier.padding(horizontal=16.dp),style=MaterialTheme.typography.bodySmall)
                    LazyColumn(state=listState,modifier=Modifier.fillMaxSize().testTag("named-people"),contentPadding=PaddingValues(16.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                        item {
                            Row(Modifier.fillMaxWidth(),verticalAlignment=Alignment.CenterVertically) {
                                Text(text(R.string.people_named),Modifier.weight(1f),style=MaterialTheme.typography.titleLarge)
                                TextButton(onClick=vm::openDirectory,enabled=browsing) { Text(text(R.string.common_refresh)) }
                            }
                        }
                        items(state.namedPeople,key={it.id}) { p ->
                            Surface(onClick={vm.openPerson(p)},enabled=browsing,shape=RoundedCornerShape(16.dp),color=MaterialTheme.colorScheme.surfaceContainer) {
                                Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.spacedBy(16.dp)) {
                                    Box(Modifier.size(72.dp).clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.surfaceContainerHighest)) {
                                        vm.images?.let { AsyncImage(vm.image(p.faceId),null,imageLoader=it,modifier=Modifier.fillMaxSize()) }
                                    }
                                    Column(Modifier.weight(1f)) {
                                        Text(p.name,style=MaterialTheme.typography.titleMedium)
                                        Text(text(R.string.people_face_count_id,text.faces(p.count),p.id),style=MaterialTheme.typography.bodySmall)
                                    }
                                }
                            }
                        }
                        if(state.namedPeople.isEmpty() && !state.busy && state.error==null && state.namedQuery==state.loadedNamedQuery) item {
                            Text(if(state.namedQuery.isBlank()) text(R.string.people_no_named) else text(R.string.people_no_results))
                        }
                        if(state.namedHasNext) item { Text(text(R.string.people_more_people),style=MaterialTheme.typography.bodySmall) }
                    }
                } else {
                    LazyVerticalGrid(columns=GridCells.Fixed(2),state=gridState,modifier=Modifier.fillMaxSize().testTag("person-faces"),
                        contentPadding=PaddingValues(16.dp),horizontalArrangement=Arrangement.spacedBy(8.dp),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                        item(key="heading",span={GridItemSpan(maxLineSpan)}) {
                            Column(verticalArrangement=Arrangement.spacedBy(12.dp)) {
                                Text(person.name,style=MaterialTheme.typography.headlineSmall)
                                Text(text.faces(person.count))
                                OutlinedButton(onClick=vm::startNaming,enabled=browsing) { Text(text(R.string.people_rename)) }
                                OutlinedButton(onClick={
                                    vm.gallery(person.name)?.let { url ->
                                        try {
                                            context.startActivity(Intent(Intent.ACTION_VIEW,Uri.parse(url)).addCategory(Intent.CATEGORY_BROWSABLE))
                                            browserError=null
                                        } catch(_: ActivityNotFoundException) { browserError=UiText(R.string.people_browser_unavailable) }
                                    }
                                },enabled=browsing) { Text(text(R.string.people_browser_search)) }
                                browserError?.let { Text(text(it),color=MaterialTheme.colorScheme.error) }
                                Text(text(R.string.people_manage_help),style=MaterialTheme.typography.bodySmall)
                            }
                        }
                        itemsIndexed(person.faces,key={_,face -> face}) {index,face ->
                            FaceGrid(person.copy(faces=listOf(face),offset=index),browsing,vm.images,vm::image,onDetach=vm::requestUnassign,
                                onHold={held=it;heldDismissed=false;zoom=0f},
                                onZoom={held=it;heldDismissed=false;zoom=0f;accessible=true},
                                onZoomDrag=zoomDrag,managing=true,onFavorite=vm::favorite,onPrefetch=vm::prefetchOriginals)
                        }
                        item(key="footer",span={GridItemSpan(maxLineSpan)}) {
                            Text(if(person.faces.size<person.count) text(R.string.people_more_photos) else text(R.string.people_all_photos),
                                style=MaterialTheme.typography.bodySmall,modifier=Modifier.padding(vertical=8.dp))
                        }
                    }
                }
            }
        }
        held?.takeUnless { heldDismissed }?.let { face ->
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                vm.images?.let { images ->
                    OriginalPhoto(vm.original(face),images,person?.faceBounds?.get(face),zoom,cacheKey=vm.originalKey(face),onZoom={zoom=it},onDrag=zoomDrag,
                        modifier=Modifier.weight(1f).fillMaxWidth(),onNewTouch=if(accessible) null else { {heldDismissed=true} })
                }
                person?.facePaths?.get(face)?.takeIf { it.isNotEmpty() }?.let {
                    Text(text.photoPath(it),color=Color.White,style=MaterialTheme.typography.bodySmall,modifier=Modifier.fillMaxWidth().testTag("original-photo-path"))
                }
                if(accessible) Button(onClick={held=null;accessible=false}) { Text(text(R.string.people_close_preview)) }
            }
        }
    }
    if(state.naming) NamingDialog(state,vm,enabled)
    state.removeFace?.let {face ->
        AlertDialog(onDismissRequest=vm::cancelUnassign,title={Text(text(R.string.people_unassign_title))},
            text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(8.dp)) {
                Text(text(R.string.people_unassign_confirmation,person?.name.orEmpty()))
                person?.facePaths?.get(face)?.takeIf {it.isNotEmpty()}?.let {Text(it)}
            }},confirmButton={TextButton(onClick=vm::confirmUnassign,enabled=enabled) {Text(text(R.string.common_remove))}},
            dismissButton={TextButton(onClick=vm::cancelUnassign,enabled=!state.busy) {Text(text(R.string.photos_cancel))}})
    }
}
