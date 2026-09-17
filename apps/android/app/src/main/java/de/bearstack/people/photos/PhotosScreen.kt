package de.bearstack.people.photos

import de.bearstack.people.text.*
import android.text.Html
import android.widget.TextView
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.*
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
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import coil.ImageLoader
import coil.compose.AsyncImage
import coil.request.ImageRequest
import de.bearstack.people.R
import de.bearstack.people.data.remote.*
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun ServerPhotosScreen(controller: PhotosController, images: ImageLoader, canManage: Boolean,
    onPeople: () -> Unit, search: String, onSearchChange: (String) -> Unit,
    onSettings: () -> Unit, onDevice: (() -> Unit)?) {
    val text=uiStrings()
    val state by controller.state.collectAsStateWithLifecycle()
    val tab = state.tab
    var sortMenu by remember(state.query, tab) { mutableStateOf(false) }
    var mapOpen by rememberSaveable(state.query) {mutableStateOf(false)}
    val locale = LocalConfiguration.current.locales[0]
    BackHandler(state.query.path.isNotEmpty() && state.selected==null && state.blog==null) {
        controller.open(state.query.copy(path=state.parent))
    }
    Scaffold(topBar={
        Column {
            TopAppBar(title={ Text(if(state.query.path.isBlank()) stringResource(R.string.app_name) else state.name.ifBlank { text(photoFolderTitle(state.query.path)) },
                maxLines=1,overflow=TextOverflow.Ellipsis) },navigationIcon={
                if(state.query.path.isNotBlank()) IconButton(onClick={controller.open(state.query.copy(path=state.parent))}) {
                    Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back))
                }
            },actions={
                if(tab==0) PhotoDateAction(controller,state)
                Box {
                    IconButton(onClick={sortMenu=true},enabled=!state.loading) {
                        Icon(painterResource(R.drawable.ic_sort),stringResource(R.string.photos_sort))
                    }
                    DropdownMenu(sortMenu,{sortMenu=false}) {
                        photoSortChoices(state.query,tab,controller.session.peopleCountSort).forEach { option ->
                            DropdownMenuItem(text={Text(stringResource(option.label))},
                                modifier=Modifier.semantics {selected=state.query.sort==option.value},
                                trailingIcon={if(state.query.sort==option.value) Text("✓")},
                                onClick={sortMenu=false;controller.open(state.query.copy(sort=option.value))})
                        }
                    }
                }
                key(state.query, tab) {
                    PhotosMenu(onSettings,
                        onMap=({ mapOpen=true }).takeIf { !state.loading && !state.query.path.startsWith(".people") },
                        onFrame=controller::startFrame.takeIf { !state.loading && (!state.query.path.startsWith(".people") || state.query.path.count {it=='/'}==2) },
                        onPeople=onPeople.takeIf { canManage },
                        onDirectoryPeople=({ controller.open(PhotoQuery(path=state.peoplePath,sort="ascending_name")) })
                            .takeIf { state.peoplePath.isNotEmpty() && !state.loading },
                        onOpen={ sortMenu=false })
                }
            })
            if(tab==2) {
                OutlinedTextField(search,onSearchChange,Modifier.fillMaxWidth().padding(horizontal=16.dp),singleLine=true,
                    shape=RoundedCornerShape(28.dp),placeholder={Text(stringResource(R.string.photos_search_hint))},
                    leadingIcon={Icon(painterResource(R.drawable.ic_search),null)},
                    keyboardOptions=KeyboardOptions(imeAction=ImeAction.Search),
                    keyboardActions=KeyboardActions(onSearch={controller.open(PhotoQuery(query=search.trim(),recursive=true))}))
                Text(stringResource(R.string.photos_search_help),Modifier.padding(horizontal=20.dp,vertical=8.dp),style=MaterialTheme.typography.bodySmall)
            }
        }
    },bottomBar={
        Box(Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal=20.dp,vertical=8.dp),contentAlignment=Alignment.Center) {
            Surface(shape=RoundedCornerShape(32.dp),shadowElevation=3.dp,modifier=Modifier.widthIn(max=440.dp)) {
            NavigationBar(windowInsets=WindowInsets(0,0,0,0),containerColor=MaterialTheme.colorScheme.surfaceContainer) {
                listOf(R.string.photos_title to R.drawable.ic_photos,R.string.photos_folders to R.drawable.ic_folder,R.string.photos_search to R.drawable.ic_search)
                    .forEachIndexed { index,(label,icon) ->
                        NavigationBarItem(selected=tab==index,onClick={
                            if(tab!=index) { controller.open(PhotoQuery(query=if(index==2) search.trim() else "",recursive=index!=1),tab=index) }
                        },icon={Icon(painterResource(icon),null)},label={Text(stringResource(label))})
                    }
            }
            }
        }
    }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            PhotoDateStatus(controller,state)
            Box(Modifier.fillMaxWidth().height(3.dp)) { if(state.loading || state.loadingSections.isNotEmpty()) LinearProgressIndicator(Modifier.fillMaxSize()) }
            state.error?.let { error ->
                Surface(color=MaterialTheme.colorScheme.errorContainer) {
                    Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically) {
                        Text(text(error),Modifier.weight(1f),style=MaterialTheme.typography.bodyMedium)
                        TextButton(onClick={controller.open(state.query)}) {Text(stringResource(R.string.photos_retry))}
                    }
                }
            }
            PhotoGallery(controller,images,state,onDevice=onDevice.takeIf { tab==1 && state.query.path.isEmpty() })
        }
    }
    state.selected?.let { path -> PhotoViewer(controller,images,state.media,path) }
    if(mapOpen) FolderMap(controller,images,state.query) {mapOpen=false}
    if(state.frame && state.selected==null) Dialog(onDismissRequest=controller::closeViewer,properties=DialogProperties(usePlatformDefaultWidth=false)) {
        Surface(Modifier.fillMaxSize()) {
            Column(Modifier.safeDrawingPadding().padding(24.dp),horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.Center) {
                if(state.loading) CircularProgressIndicator() else Text(state.error?.let {text(it)} ?: stringResource(R.string.photos_empty))
                TextButton(onClick=controller::closeViewer) {Text(stringResource(R.string.photos_close))}
            }
        }
    }
    state.blog?.let { post ->
        Dialog(onDismissRequest=controller::closeBlog,properties=DialogProperties(usePlatformDefaultWidth=false)) {
            Surface(Modifier.fillMaxSize()) {
                Column(Modifier.safeDrawingPadding()) {
                    TopAppBar(title={Text(post.name,maxLines=1,overflow=TextOverflow.Ellipsis)},navigationIcon={
                        IconButton(onClick=controller::closeBlog) { Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back)) }
                    })
                    if(state.blogLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
                    state.blogError?.let {error ->
                        Row(Modifier.padding(16.dp),verticalAlignment=Alignment.CenterVertically) {
                            Text(text(error),Modifier.weight(1f),color=MaterialTheme.colorScheme.error)
                            TextButton(onClick={controller.openBlog(post)}) {Text(stringResource(R.string.photos_retry))}
                        }
                    }
                    if(!state.blogLoading && state.blogError==null && post.html.isEmpty() && post.text.isEmpty()) {
                        Text(stringResource(R.string.photos_text_empty),Modifier.padding(24.dp))
                    }
                    val textColor=MaterialTheme.colorScheme.onSurface.toArgb()
                    // Server HTML contains escaped text and a small formatting vocabulary.
                    // TextView renders it without a browser, script or network loader.
                    AndroidView(factory={TextView(it).apply {textSize=18f;setTextIsSelectable(true)}},
                        update={it.text=Html.fromHtml(post.html,Html.FROM_HTML_MODE_COMPACT);it.setTextColor(textColor)},
                        modifier=Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp))
                }
            }
        }
    }
}

@Composable internal fun SectionTitle(title: String) {
    Text(title,Modifier.padding(horizontal=16.dp,vertical=14.dp),style=MaterialTheme.typography.titleMedium)
}
@Composable internal fun FolderTile(folder: PhotoFolder, controller: PhotosController, images: ImageLoader, onClick: () -> Unit) {
    Column(Modifier.padding(6.dp).clip(RoundedCornerShape(20.dp)).clickable(onClick=onClick)
        .background(MaterialTheme.colorScheme.surfaceContainerLow)) {
        val cells=folder.previews.size.coerceIn(1,8)
        val columns=if(cells>4) 4 else if(cells>1) 2 else 1
        val rows=(cells+columns-1)/columns
        Column(Modifier.fillMaxWidth().height(104.dp),verticalArrangement=Arrangement.spacedBy(2.dp)) {
            repeat(rows) { row ->
                Row(Modifier.fillMaxWidth().weight(1f),horizontalArrangement=Arrangement.spacedBy(2.dp)) {
                    repeat(columns) { column ->
                        val photo=folder.previews.getOrNull(row*columns+column)
                        if(photo!=null) PhotoThumbnail(photo,controller,images,controller.session.folderThumbnailSize,Modifier.weight(1f).fillMaxHeight())
                        else Box(Modifier.weight(1f).fillMaxHeight().background(MaterialTheme.colorScheme.surfaceContainerHighest),contentAlignment=Alignment.Center) {
                            Icon(painterResource(R.drawable.ic_folder),null,tint=MaterialTheme.colorScheme.outline)
                        }
                    }
                }
            }
        }
        Column(Modifier.padding(12.dp),verticalArrangement=Arrangement.spacedBy(4.dp)) {
            Text(folder.name,style=MaterialTheme.typography.titleSmall,maxLines=2,overflow=TextOverflow.Ellipsis)
            Text(if(folder.virtual && folder.path.count {it=='/'}<2) pluralStringResource(R.plurals.photos_person_count,folder.folders,folder.folders) else pluralStringResource(if(folder.approximate) R.plurals.photos_count_approximate else R.plurals.photos_count,folder.count,folder.count),
                style=MaterialTheme.typography.bodySmall,color=MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
@Composable internal fun PhotoThumbnail(photo: Photo, controller: PhotosController, images: ImageLoader, size: Int, modifier: Modifier) {
    val context=LocalContext.current
    val request=remember(photo.path,photo.version,photo.faceId,size,controller) {
        val data = if(controller.thumbnailCache != null)
            de.bearstack.people.media.cachedThumbnail(controller.service, controller.session, photo, size)
            else controller.service.thumbnail(photo,size)
        ImageRequest.Builder(context).data(data).size(size).build()
    }
    AsyncImage(request,photo.name,imageLoader=images,contentScale=ContentScale.Crop,
        modifier=modifier.background(MaterialTheme.colorScheme.surfaceContainerHighest))
}
internal fun photoDateLabel(date: String, locale: Locale): String = runCatching {
    OffsetDateTime.parse(date).toLocalDate().format(DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL).withLocale(locale))
}.getOrDefault(date.take(10))

// Virtual paths are routing keys, never user-facing labels. A name from the
// tapped tile or a previous visit wins; deep links use translated placeholders.
internal fun photoFolderTitle(path: String): UiText = when {
    path == ".people" -> UiText(R.string.photos_people_folder)
    path == ".people/all" -> UiText(R.string.photos_all_people)
    path.startsWith(".people/f-") && path.count {it=='/'} == 1 -> UiText(R.string.photos_directory_people)
    path.startsWith(".people/") -> UiText(R.string.photos_loading)
    else -> UiText(R.string.photos_folder_name, path.substringAfterLast('/'))
}
