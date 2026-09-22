package de.bearstack.people.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import de.bearstack.people.R
import de.bearstack.people.people.PeopleState
import de.bearstack.people.people.PeopleViewModel
import de.bearstack.people.text.uiStrings

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun PersonFoldersScreen(state: PeopleState, vm: PeopleViewModel) {
    val text=uiStrings()
    val page=state.folderPage
    var menu by remember {mutableStateOf(false)}
    var help by remember {mutableStateOf(false)}
    var held by remember {mutableStateOf<Long?>(null)}
    var dismissed by remember {mutableStateOf(false)}
    var accessible by remember {mutableStateOf(false)}
    var zoom by remember {mutableFloatStateOf(0f)}
    val distance=with(LocalDensity.current) {240.dp.toPx()}
    val zoomDrag: (Float)->Unit = {zoom=zoomAfterDrag(zoom,it,distance)}
    val enabled=!state.busy && !state.unresolved && held==null
    val browsing=enabled && state.folderReady && !state.naming && state.folderConfirmation==null && !help && !menu
    val list=rememberLazyListState()
    LaunchedEffect(page?.page,page?.revision) {list.scrollToItem(0);held=null;accessible=false}
    BackHandler {
        if(held!=null) {if(accessible) held=null else dismissed=true}
        else if(help) help=false
        else if(state.naming) vm.closeNaming()
        else if(state.folderConfirmation!=null) vm.cancelFolderAction()
        else vm.closePersonFolders()
    }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={TopAppBar(title={Text(text(R.string.people_folders_title),maxLines=1,overflow=TextOverflow.Ellipsis)},
            navigationIcon={BackAction(vm::closePersonFolders,enabled=enabled && !state.naming && state.folderConfirmation==null)},actions={
            PeopleOptionsMenu(menu,{menu=it},state,vm,onHelp={help=true},enabled=held==null && !state.naming && state.folderConfirmation==null) {
                DropdownMenuItem(text={Text(text(R.string.common_refresh))},enabled=enabled,onClick={menu=false;vm.personFolderPage(page?.page ?: 1)})
                HorizontalDivider()
            }
        })}) {padding ->
            Column(Modifier.fillMaxSize().padding(padding)) {
                Box(Modifier.fillMaxWidth().height(4.dp)) {if(state.busy) LinearProgressIndicator(Modifier.fillMaxSize())}
                state.error?.let {error ->
                    Card(Modifier.fillMaxWidth().padding(12.dp),colors=CardDefaults.cardColors(containerColor=MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(12.dp)) {
                            Text(text(error),Modifier.semantics {liveRegion=LiveRegionMode.Polite})
                            TextButton(onClick=vm::retry,enabled=!state.busy && held==null) {
                                Text(text(if(state.unresolved) R.string.people_check_pending else R.string.photos_retry))
                            }
                        }
                    }
                }
                LazyColumn(state=list,modifier=Modifier.fillMaxSize().testTag("person-folders"),contentPadding=PaddingValues(16.dp),verticalArrangement=Arrangement.spacedBy(16.dp)) {
                    item {
                        Column(verticalArrangement=Arrangement.spacedBy(8.dp)) {
                            Text((page?.name ?: state.folderSource?.name).orEmpty().ifBlank {text(R.string.people_folders_unnamed_person)},style=MaterialTheme.typography.headlineSmall)
                            Text(text(R.string.people_folders_scope))
                        }
                    }
                    items(page?.folders.orEmpty(),key={it.directory}) {folder ->
                        Card(Modifier.fillMaxWidth()) {
                            Column(Modifier.padding(12.dp),verticalArrangement=Arrangement.spacedBy(8.dp)) {
                                Text(text.photoPath(folder.displayPath),style=MaterialTheme.typography.titleMedium)
                                Text(if(folder.excluded) text(R.string.people_folders_excluded) else text.faces(folder.count))
                                if(folder.preview.faces.isNotEmpty()) LazyRow(horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                                    itemsIndexed(folder.preview.faces,key={_,face -> face}) {index,face ->
                                        Box(Modifier.width(112.dp)) {
                                            FaceGrid(folder.preview.copy(faces=listOf(face),offset=index),browsing,vm.images,vm::image,
                                                onDetach={},allowDetach=false,showPaths=false,
                                                onHold={held=it;dismissed=false;zoom=0f},
                                                onZoom={held=it;dismissed=false;zoom=0f;accessible=true},onZoomDrag=zoomDrag,
                                                onPrefetch=vm::prefetchOriginals)
                                        }
                                    }
                                }
                                for(action in if(folder.excluded) listOf("include") else listOf("move","unnamed","ignore","exclude")) {
                                    OutlinedButton(onClick={vm.requestFolderAction(folder,action)},enabled=browsing,modifier=Modifier.fillMaxWidth()) {
                                        Text(text(folderActionLabel(action)))
                                    }
                                }
                            }
                        }
                    }
                    if(page?.folders?.isEmpty()==true && !state.busy && state.error==null) item {Text(text(R.string.people_folders_empty))}
                    if(page!=null) item {
                        Column(Modifier.fillMaxWidth(),horizontalAlignment=Alignment.CenterHorizontally) {
                            Text(text(R.string.people_folders_page,page.page))
                            Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.SpaceBetween) {
                                TextButton(onClick={vm.personFolderPage(page.page-1)},enabled=browsing && page.page>1) {Text(text(R.string.photos_back))}
                                TextButton(onClick={vm.personFolderPage(page.page+1)},enabled=browsing && page.hasNext) {Text(text(R.string.people_folders_next))}
                            }
                        }
                    }
                }
            }
        }
        held?.takeUnless {dismissed}?.let {face ->
            val preview=page?.folders?.firstOrNull {face in it.preview.faces}?.preview
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                vm.images?.let {images ->
                    OriginalPhoto(vm.original(face),images,preview?.faceBounds?.get(face),zoom,cacheKey=vm.originalKey(face),onZoom={zoom=it},onDrag=zoomDrag,
                        modifier=Modifier.weight(1f).fillMaxWidth(),onNewTouch=if(accessible) null else {{dismissed=true}})
                }
                preview?.facePaths?.get(face)?.takeIf {it.isNotEmpty()}?.let {
                    Text(text.photoPath(it),color=Color.White,style=MaterialTheme.typography.bodySmall,modifier=Modifier.fillMaxWidth().testTag("original-photo-path"))
                }
                if(accessible) Button(onClick={held=null;accessible=false}) {Text(text(R.string.people_close_preview))}
            }
        }
    }
    if(help) AlertDialog(onDismissRequest={help=false},title={Text(text(R.string.common_help))},
        text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(text(R.string.people_folders_help))
        }},confirmButton={TextButton(onClick={help=false}) {Text(text(R.string.photos_close))}})
    if(state.naming) NamingDialog(state,vm,enabled)
    state.folderConfirmation?.let {action ->
        AlertDialog(onDismissRequest={if(enabled) vm.cancelFolderAction()},title={Text(text(folderActionLabel(action)))},
            text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(12.dp)) {
                state.folderSelection?.let {Text(text.photoPath(it.displayPath));Text(text.faces(it.count))}
                Text(text(if(action=="include") R.string.people_folders_include_help else if(action=="exclude") R.string.people_folders_exclude_help else R.string.people_folders_scope))
                state.error?.let {Text(text(it),color=MaterialTheme.colorScheme.error)}
                if(state.unresolved) {
                    TextButton(onClick=vm::retry,enabled=!state.busy) {Text(text(R.string.people_check_pending))}
                    PeopleConnectionAction(state,vm,enabled=!state.busy)
                }
            }},confirmButton={TextButton(onClick=vm::confirmFolderAction,enabled=enabled) {Text(text(R.string.people_folders_confirm))}},
            dismissButton={TextButton(onClick=vm::cancelFolderAction,enabled=enabled) {Text(text(R.string.photos_cancel))}})
    }
}

private fun folderActionLabel(action: String): Int = when(action) {
    "move" -> R.string.people_folders_move
    "unnamed" -> R.string.people_folders_unnamed
    "ignore" -> R.string.people_folders_ignore
    "exclude" -> R.string.people_folders_exclude
    else -> R.string.people_folders_include
}
