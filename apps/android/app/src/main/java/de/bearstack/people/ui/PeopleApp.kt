package de.bearstack.people.ui

import de.bearstack.people.text.uiStrings
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.*
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitLongPressOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalViewConfiguration
import androidx.compose.ui.platform.ViewConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.repeatOnLifecycle
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.semantics.*
import androidx.compose.ui.text.input.*
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.ImageLoader
import coil.compose.AsyncImage
import de.bearstack.people.R
import de.bearstack.people.statistics.StatisticsPanel
import de.bearstack.people.data.remote.Person
import de.bearstack.people.people.*

@Composable
fun PeopleApp(vm: PeopleViewModel) {
    val text=uiStrings()
    val state by vm.state.collectAsStateWithLifecycle()
    BackHandler(state.connected && !state.showGallery && vm.photos!=null) { vm.openGallery() }
    BearStackTheme {
        Surface(Modifier.fillMaxSize()) {
            if (!state.connected) ConnectionScreen(state,vm)
            else if(state.showGallery && vm.photos!=null && vm.images!=null)
                de.bearstack.people.photos.PhotosScreen(vm.photos!!,vm.images!!,state.canManagePeople,vm::openPeople,vm::switchConnection)
            else if(state.mergeReview) MergeReviewScreen(state,vm)
            else if(state.directory) PeopleDirectoryScreen(state,vm)
            else LabelingScreen(state,vm)
            if(state.undoIgnores.isNotEmpty()) key(state.naming) { IgnoreUndoToast(state.undoIgnores.size,vm::undoIgnore) }
            state.certificate?.let { certificate ->
                AlertDialog(onDismissRequest={ if(!state.busy) vm.cancelCertificate() },title={ Text(text(R.string.connection_certificate_title)) },
                    text={ Column(Modifier.verticalScroll(rememberScrollState())) {
                        Text(text(R.string.connection_certificate_help))
                        Spacer(Modifier.height(16.dp)); Text(certificate.fingerprint)
                        Spacer(Modifier.height(16.dp)); Text(text(R.string.connection_certificate_validity,certificate.subject,text.date(certificate.expires)))
                        state.error?.let { Text(text(it),color=MaterialTheme.colorScheme.error,
                            modifier=Modifier.semantics {liveRegion=LiveRegionMode.Polite}) }
                    } }, confirmButton={ TextButton(onClick=vm::confirmCertificate,enabled=!state.busy) { Text(text(R.string.connection_certificate_accept)) } },
                    dismissButton={ TextButton(onClick=vm::cancelCertificate,enabled=!state.busy) { Text(text(R.string.photos_cancel)) } })
            }
        }
    }
}
@Composable
private fun ConnectionScreen(state: PeopleState, vm: PeopleViewModel) {
    val text=uiStrings()
    var url by rememberSaveable { mutableStateOf("") }
    var username by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement=Arrangement.spacedBy(16.dp)) {
        Text(stringResource(R.string.app_name), style=MaterialTheme.typography.headlineMedium)
        Text(text(R.string.connection_help))
        OutlinedTextField(url,{url=it},Modifier.fillMaxWidth(),label={Text(text(R.string.connection_address))},singleLine=true,
            enabled=!state.busy,keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Uri),placeholder={Text("https://bearstack.example/")})
        OutlinedTextField(username,{username=it},Modifier.fillMaxWidth(),label={Text(text(R.string.connection_username))},singleLine=true,enabled=!state.busy)
        OutlinedTextField(password,{password=it},Modifier.fillMaxWidth(),label={Text(text(R.string.connection_password))},singleLine=true,enabled=!state.busy,
            visualTransformation=PasswordVisualTransformation(),keyboardOptions=KeyboardOptions(keyboardType=KeyboardType.Password))
        Button(onClick={vm.connect(url,username,password)},enabled=!state.busy && url.isNotBlank() && username.isNotBlank() && password.isNotEmpty(),
            modifier=Modifier.fillMaxWidth()) { Text(text(R.string.connection_connect)) }
        if (state.busy) LinearProgressIndicator(Modifier.fillMaxWidth())
        state.error?.let { Text(text(it),color=MaterialTheme.colorScheme.error,modifier=Modifier.semantics { liveRegion=LiveRegionMode.Polite }) }
    }
}
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LabelingScreen(state: PeopleState, vm: PeopleViewModel) {
    val text=uiStrings()
    var menu by remember { mutableStateOf(false) }
    var actions by remember { mutableStateOf(false) }
    var help by remember { mutableStateOf(false) }
    var statistics by rememberSaveable { mutableStateOf(false) }
    var held by remember { mutableStateOf<Long?>(null) }
    var heldDismissed by remember { mutableStateOf(false) }
    var accessibleZoom by remember { mutableStateOf(false) }
    var zoom by remember { mutableFloatStateOf(0f) }
    val zoomDistance=with(LocalDensity.current) { 240.dp.toPx() }
    val zoomDrag: (Float) -> Unit = { dy -> zoom=zoomAfterDrag(zoom,dy,zoomDistance) }
    val enabled = !state.busy && !state.unresolved && held == null
    LaunchedEffect(state.person?.id,state.person?.revision) { held=null;accessibleZoom=false }
    Box(Modifier.fillMaxSize()) {
        Scaffold(topBar={ TopAppBar(title={Text(if(statistics) text(R.string.people_statistics) else text(R.string.people_labeling))},actions={
            TextButton(onClick={menu=true}) { Text(text(R.string.common_menu)) }
            DropdownMenu(menu,{menu=false}) {
                if(vm.photos!=null) DropdownMenuItem(text={Text(stringResource(R.string.photos_title))},onClick={vm.openGallery();menu=false},enabled=enabled)
                DropdownMenuItem(text={Text(text(R.string.people_directory))},onClick={vm.openDirectory();menu=false},enabled=enabled)
                DropdownMenuItem(text={Text(text(R.string.people_similar_groups))},onClick={vm.openMergeReview();menu=false},enabled=enabled)
                DropdownMenuItem(text={Text(if(statistics) text(R.string.people_return_labeling) else text(R.string.people_statistics))},onClick={statistics=!statistics;menu=false},enabled=enabled)
                DropdownMenuItem(text={Text(text(R.string.people_review_skipped,state.skipped))},onClick={vm.newPass(true);menu=false},enabled=enabled && state.person==null && state.skipped>0)
                DropdownMenuItem(text={Text(text(R.string.photos_connection))},onClick={vm.switchConnection();menu=false},enabled=!state.busy)
            }
        }) },bottomBar={
            if(!statistics) Surface(color=MaterialTheme.colorScheme.surfaceContainer,tonalElevation=2.dp) {
                Row(Modifier.fillMaxWidth().windowInsetsPadding(WindowInsets.safeDrawing.only(WindowInsetsSides.Horizontal+WindowInsetsSides.Bottom))
                    .heightIn(min=56.dp).padding(horizontal=12.dp).testTag("labeling-action-bar"),
                    horizontalArrangement=Arrangement.SpaceBetween,verticalAlignment=Alignment.CenterVertically) {
                    Box {
                        IconButton(onClick={actions=true},enabled=enabled && !state.naming,
                            modifier=Modifier.semantics {contentDescription=text(R.string.people_group_actions)}) {
                            Icon(painterResource(R.drawable.ic_more_horiz),contentDescription=null,modifier=Modifier.size(24.dp))
                        }
                        DropdownMenu(actions,{actions=false}) {
                            DropdownMenuItem(text={Text(text(R.string.people_ignore_group))},onClick={actions=false;vm.ignore()},enabled=enabled && state.person!=null)
                            DropdownMenuItem(text={Text(text(R.string.people_skip_group))},onClick={actions=false;vm.skip()},enabled=enabled && state.person!=null)
                            DropdownMenuItem(text={Text(text(R.string.people_undo_skip))},onClick={actions=false;vm.back()},enabled=enabled && state.canGoBack)
                        }
                    }
                    IconButton(onClick={help=true},enabled=held==null && !state.naming,
                        modifier=Modifier.semantics {contentDescription=text(R.string.people_naming_help)}) {
                        Icon(painterResource(R.drawable.ic_question_mark),contentDescription=null,modifier=Modifier.size(24.dp))
                    }
                    IconButton(onClick=vm::startNaming,enabled=enabled && !state.naming && state.person!=null,
                        modifier=Modifier.semantics {contentDescription=text(R.string.people_name_person)}) {
                        Icon(painterResource(R.drawable.ic_edit),contentDescription=null,modifier=Modifier.size(24.dp))
                    }
                }
            }
        }) { padding ->
            PersonSwipeArea(gestureKey=state.person?.let { it.id to it.revision },
                enabled=enabled && !statistics && !menu && !actions && !help && !state.naming && (state.person!=null || state.canGoBack),
                onSwipe={when(it) { SwipeAction.Ignore -> vm.ignore(); SwipeAction.Skip -> vm.skip(); SwipeAction.Back -> vm.back() }},
                modifier=Modifier.fillMaxSize().padding(padding).imePadding()) {
                // Keep the progress slot and column spacing stable across loading transitions.
                Box(Modifier.fillMaxWidth().height(4.dp)) {
                    if(state.busy) LinearProgressIndicator(Modifier.fillMaxSize())
                }
                state.error?.let { error ->
                    Card(colors=CardDefaults.cardColors(containerColor=MaterialTheme.colorScheme.errorContainer)) {
                        Column(Modifier.padding(16.dp)) {
                            Text(text(error),Modifier.semantics { liveRegion=LiveRegionMode.Polite })
                            TextButton(onClick=vm::retry,enabled=!state.busy) { Text(if(state.unresolved) text(R.string.people_check_pending) else text(R.string.photos_retry)) }
                        }
                    }
                }
                if(statistics) StatisticsPanel(state.stats)
                else state.person?.let { person ->
                    FaceGrid(person,enabled,vm.images,vm::image,onDetach=vm::detach,
                        onHold={ held=it;heldDismissed=false;zoom=0f },
                        onZoom={held=it;heldDismissed=false;zoom=0f;accessibleZoom=true},onZoomDrag=zoomDrag,onPrefetch=vm::prefetchOriginals)
                    if(person.count>4) Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.SpaceBetween) {
                        OutlinedButton(onClick={vm.page(-1)},enabled=enabled && person.offset>0) { Text(text(R.string.photos_back)) }
                        OutlinedButton(onClick={vm.page(1)},enabled=enabled && person.offset+4<person.count) { Text(text(R.string.common_next)) }
                    }
                } ?: run {
                    if(!state.busy && !state.unresolved) {
                        Text(text(R.string.people_pass_complete),style=MaterialTheme.typography.headlineSmall)
                        if(state.canGoBack) OutlinedButton(onClick=vm::back,enabled=enabled) { Text(text(R.string.people_undo_skip)) }
                        Button(onClick={vm.newPass(false)}) { Text(text(R.string.people_new_pass)) }
                        if(state.skipped>0) OutlinedButton(onClick={vm.newPass(true)}) { Text(text(R.string.people_review_skipped,state.skipped)) }
                    }
                }
            }
        }
        held?.takeUnless { heldDismissed }?.let { face ->
            Column(Modifier.fillMaxSize().background(Color.Black.copy(alpha=.94f)).safeDrawingPadding().padding(16.dp),
                horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(8.dp)) {
                vm.images?.let { images ->
                    OriginalPhoto(vm.original(face),images,state.person?.faceBounds?.get(face),zoom,cacheKey=vm.originalKey(face),
                        onZoom={zoom=it},onDrag=zoomDrag,modifier=Modifier.weight(1f).fillMaxWidth(),
                        // Keep editing blocked until the original held pointer is released.
                        onNewTouch=if(accessibleZoom) null else { { heldDismissed=true } })
                }
                state.person?.facePaths?.get(face)?.takeIf {it.isNotEmpty()}?.let {
                    Text(text.photoPath(it),color=Color.White,style=MaterialTheme.typography.bodySmall,
                        modifier=Modifier.fillMaxWidth().testTag("original-photo-path"))
                }
                if(accessibleZoom) Button(onClick={held=null;accessibleZoom=false}) { Text(text(R.string.people_close_preview)) }
            }
        }
    }
    if(help) AlertDialog(onDismissRequest={help=false},title={Text(text(R.string.people_naming_help))},
        text={Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(12.dp)) {
            Text(text(R.string.people_help_name))
            Text(text(R.string.people_help_preview))
            Text(text(R.string.people_help_swipe))
            Text(text(R.string.people_help_undo))
        }},confirmButton={TextButton(onClick={help=false}) {Text(text(R.string.photos_close))}})
    if(state.naming) NamingDialog(state,vm,enabled)
}

@Composable
fun FaceGrid(person: Person, enabled: Boolean, images: ImageLoader?, image: (Long, Boolean) -> String?,
    onDetach: (Long) -> Unit, onHold: (Long?) -> Unit, onZoom: (Long) -> Unit,
    onZoomDrag: (Float) -> Unit = {}, managing: Boolean = false, onFavorite: (Long) -> Unit = {},
    onPrefetch: suspend (List<Long>) -> Unit = {}, allowDetach: Boolean = true, showPaths: Boolean = true) {
    val text=uiStrings()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val prefetch by rememberUpdatedState(onPrefetch)
    LaunchedEffect(person.id,person.faces,images,lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) { prefetch(person.faces) }
    }
    val systemConfiguration = LocalViewConfiguration.current
    val previewConfiguration = remember(systemConfiguration) {
        object : ViewConfiguration by systemConfiguration {
            override val longPressTimeoutMillis = minOf(250L,systemConfiguration.longPressTimeoutMillis)
        }
    }
    CompositionLocalProvider(LocalViewConfiguration provides previewConfiguration) {
        var holding by remember { mutableStateOf(false) }
        val active by rememberUpdatedState(enabled)
        val hold by rememberUpdatedState(onHold)
        val zoomDrag by rememberUpdatedState(onZoomDrag)
        Column(Modifier.fillMaxWidth().testTag("face-grid"),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            person.faces.chunked(2).forEach { row ->
                Row(Modifier.fillMaxWidth(),horizontalArrangement=Arrangement.spacedBy(8.dp)) {
                    row.forEach { face ->
                        key(face) {
                            Column(Modifier.weight(1f)) {
                                Box(Modifier.fillMaxWidth().testTag("face-$face").aspectRatio(1f).clip(RoundedCornerShape(16.dp))
                                    .background(MaterialTheme.colorScheme.surfaceContainerHighest)
                                    .semantics { contentDescription=text(R.string.people_face_number,person.faces.indexOf(face)+person.offset+1)
                                        customActions=listOf(CustomAccessibilityAction(text(R.string.people_show_original)) { if(active) {onZoom(face);true} else false }) }
                                    .pointerInput(face) {
                                        awaitEachGesture {
                                            val down=awaitFirstDown()
                                            if(!active) return@awaitEachGesture
                                            val pressed=awaitLongPressOrCancellation(down.id) ?: return@awaitEachGesture
                                            if(!active) return@awaitEachGesture
                                            holding=true
                                            try {
                                                hold(face)
                                                // The original tile owns this pointer even while the overlay is visible.
                                                // Keep tracking outside its bounds, but cancel on competing/multiple pointers.
                                                while(true) {
                                                    val event=awaitPointerEvent()
                                                    val change=event.changes.singleOrNull { it.id==pressed.id } ?: break
                                                    if(event.changes.size!=1 || change.isConsumed) break
                                                    if(!change.pressed) { change.consume();break }
                                                    zoomDrag(change.position.y-change.previousPosition.y)
                                                    change.consume()
                                                }
                                            } finally { holding=false;hold(null) }
                                        }
                                    }) {
                                    if(images!=null) AsyncImage(image(face,false),null,imageLoader=images,
                                        modifier=Modifier.fillMaxSize(),contentScale=ContentScale.Crop)
                                    if(allowDetach && (managing || person.count>1)) FilledTonalIconButton(onClick={onDetach(face)},enabled=enabled && !holding,
                                        modifier=Modifier.align(Alignment.BottomStart).padding(4.dp).size(48.dp)
                                            .padding(if(managing) 8.dp else 0.dp)
                                            .semantics { contentDescription=if(managing) text(R.string.people_unassign) else text(R.string.people_name_face) }) { Text("×",style=if(managing) MaterialTheme.typography.titleMedium else MaterialTheme.typography.headlineMedium) }
                                    if(managing) IconButton(onClick={onFavorite(face)},enabled=enabled && !holding,
                                        modifier=Modifier.align(Alignment.BottomEnd).padding(4.dp).size(48.dp).semantics {
                                            contentDescription=if(face in person.favorites) text(R.string.people_unfavorite) else text(R.string.people_favorite)
                                            stateDescription=if(face in person.favorites) text(R.string.people_favorited) else text(R.string.people_not_favorited)
                                        }) { Text(if(face in person.favorites) "★" else "☆",style=MaterialTheme.typography.titleMedium) }
                                }
                                person.facePaths[face]?.takeIf {showPaths && it.isNotEmpty()}?.let {
                                    Text(text.photoPath(it),style=MaterialTheme.typography.bodySmall,
                                        modifier=Modifier.fillMaxWidth().padding(top=6.dp).testTag("face-path-$face"))
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
internal fun NamingDialog(state: PeopleState, vm: PeopleViewModel, enabled: Boolean) {
    val text=uiStrings()
    val focus = remember { FocusRequester() }
    val keyboard = LocalSoftwareKeyboardController.current
    LaunchedEffect(Unit) { focus.requestFocus(); keyboard?.show() }
    AlertDialog(onDismissRequest=vm::closeNaming,title={Text(if(state.duplicates.isNotEmpty()) text(R.string.people_name_exists) else if(state.directory) text(R.string.people_rename) else text(R.string.people_name_person))},
        properties=DialogProperties(usePlatformDefaultWidth=false),modifier=Modifier.fillMaxWidth().padding(16.dp).imePadding(),
        text={ Column(Modifier.verticalScroll(rememberScrollState()),verticalArrangement=Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(state.name,vm::nameChanged,label={Text(text(R.string.people_name))},singleLine=true,enabled=enabled,
                modifier=Modifier.fillMaxWidth().focusRequester(focus),keyboardOptions=KeyboardOptions(imeAction=ImeAction.Done),
                keyboardActions=KeyboardActions(onDone={vm.submitName()}))
            if(state.duplicates.isNotEmpty()) Text(if(state.directory) text(R.string.people_duplicate_rename) else text(R.string.people_duplicate_assign))
            (if(state.directory) emptyList() else state.duplicates.ifEmpty { state.suggestions }).forEach { person ->
                Surface(onClick={vm.assign(person)},enabled=enabled,shape=RoundedCornerShape(12.dp),color=MaterialTheme.colorScheme.surfaceContainer) {
                    Row(Modifier.fillMaxWidth().padding(8.dp),verticalAlignment=Alignment.CenterVertically,horizontalArrangement=Arrangement.spacedBy(12.dp)) {
                        vm.images?.let { AsyncImage(vm.image(person.faceId),null,imageLoader=it,modifier=Modifier.size(48.dp).clip(RoundedCornerShape(8.dp))) }
                        Column(Modifier.weight(1f)) { Text(person.name);Text(text(R.string.people_face_count_id,text.faces(person.count),person.id),style=MaterialTheme.typography.bodySmall) }
                    }
                }
            }
            state.error?.let { Text(text(it),color=MaterialTheme.colorScheme.error) }
            if(state.unresolved) {
                TextButton(onClick=vm::retry,enabled=!state.busy) { Text(text(R.string.people_check_pending)) }
                TextButton(onClick=vm::switchConnection,enabled=!state.busy) { Text(text(R.string.connection_check)) }
            }
        } },confirmButton={ TextButton(onClick={vm.submitName(state.duplicates.isNotEmpty())},enabled=enabled && state.name.isNotBlank()) {
            Text(if(state.duplicates.isNotEmpty()) text(R.string.people_name_separately) else text(R.string.photos_save))
        } },dismissButton={ TextButton(onClick=vm::closeNaming,enabled=enabled) { Text(text(R.string.photos_cancel)) } })
}
