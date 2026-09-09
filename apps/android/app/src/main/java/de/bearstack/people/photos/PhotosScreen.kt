package de.bearstack.people.photos

import de.bearstack.people.text.*
import android.text.Html
import android.widget.TextView
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.*
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
fun PhotosScreen(controller: PhotosController, images: ImageLoader, canManage: Boolean,
    onPeople: () -> Unit, onConnection: () -> Unit) {
    val text=uiStrings()
    val state by controller.state.collectAsStateWithLifecycle()
    val tab = state.tab
    var search by rememberSaveable { mutableStateOf(state.query.query) }
    var menu by remember { mutableStateOf(false) }
    var mapOpen by rememberSaveable(state.query) {mutableStateOf(false)}
    val locale = LocalConfiguration.current.locales[0]
    BackHandler(state.query.path.isNotEmpty() && state.selected==null && state.blog==null) {
        controller.open(state.query.copy(path=state.query.path.substringBeforeLast('/',"")))
    }
    Scaffold(topBar={
        Column {
            TopAppBar(title={ Text(if(state.query.path.isBlank()) stringResource(R.string.app_name) else state.query.path.substringAfterLast('/'),
                maxLines=1,overflow=TextOverflow.Ellipsis) },navigationIcon={
                if(state.query.path.isNotBlank()) IconButton(onClick={controller.open(state.query.copy(path=state.query.path.substringBeforeLast('/',"")))}) {
                    Icon(painterResource(R.drawable.ic_back),stringResource(R.string.photos_back))
                }
            },actions={
                IconButton(onClick={menu=true}) { Icon(painterResource(R.drawable.ic_more_horiz),stringResource(R.string.photos_menu)) }
                DropdownMenu(menu,{menu=false}) {
                    DropdownMenuItem(text={Text(stringResource(R.string.photos_map))},onClick={menu=false;mapOpen=true},enabled=!state.loading)
                    DropdownMenuItem(text={Text(stringResource(R.string.photos_frame))},onClick={menu=false;controller.startFrame()},enabled=!state.loading)
                    if(canManage) DropdownMenuItem(text={Text(stringResource(R.string.photos_people))},onClick={menu=false;onPeople()})
                    DropdownMenuItem(text={Text(stringResource(R.string.photos_connection))},onClick={menu=false;onConnection()})
                }
            })
            if(tab==2) {
                OutlinedTextField(search,{search=it},Modifier.fillMaxWidth().padding(horizontal=16.dp),singleLine=true,
                    shape=RoundedCornerShape(28.dp),placeholder={Text(stringResource(R.string.photos_search_hint))},
                    leadingIcon={Icon(painterResource(R.drawable.ic_search),null)},
                    keyboardOptions=KeyboardOptions(imeAction=ImeAction.Search),
                    keyboardActions=KeyboardActions(onSearch={controller.open(PhotoQuery(query=search.trim(),recursive=true))}))
                Text(stringResource(R.string.photos_search_help),Modifier.padding(horizontal=20.dp,vertical=8.dp),style=MaterialTheme.typography.bodySmall)
            }
        }
    },bottomBar={
        NavigationBar {
            listOf(R.string.photos_title to R.drawable.ic_photos,R.string.photos_folders to R.drawable.ic_folder,R.string.photos_search to R.drawable.ic_search)
                .forEachIndexed { index,(label,icon) ->
                    NavigationBarItem(selected=tab==index,onClick={
                        if(tab!=index) { controller.open(PhotoQuery(query=if(index==2) search.trim() else "",recursive=index!=1),tab=index) }
                    },icon={Icon(painterResource(icon),null)},label={Text(stringResource(label))})
                }
        }
    }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            Box(Modifier.fillMaxWidth().height(3.dp)) { if(state.loading) LinearProgressIndicator(Modifier.fillMaxSize()) }
            state.error?.let { error ->
                Surface(color=MaterialTheme.colorScheme.errorContainer) {
                    Row(Modifier.fillMaxWidth().padding(12.dp),verticalAlignment=Alignment.CenterVertically) {
                        Text(text(error),Modifier.weight(1f),style=MaterialTheme.typography.bodyMedium)
                        TextButton(onClick={controller.open(state.query)}) {Text(stringResource(R.string.photos_retry))}
                    }
                }
            }
            key(state.query) {
                val grid = rememberLazyGridState()
                LazyVerticalGrid(columns=GridCells.Fixed(6),state=grid,modifier=Modifier.fillMaxSize(),
                    horizontalArrangement=Arrangement.spacedBy(2.dp),verticalArrangement=Arrangement.spacedBy(2.dp),contentPadding=PaddingValues(bottom=16.dp)) {
                    items(state.folders,key={"folder:${it.path}"},span={GridItemSpan(3)},contentType={"folder"}) { folder ->
                        FolderTile(folder,controller,images) { controller.open(PhotoQuery(path=folder.path),tab=1) }
                    }
                    if(state.folderHasNext) item(key="more-folders",span={GridItemSpan(maxLineSpan)}) {
                        LoadSection("folders",state,controller)
                    }
                    if(state.blogs.isNotEmpty()) {
                        item(span={GridItemSpan(maxLineSpan)}) { SectionTitle(stringResource(R.string.photos_texts)) }
                        items(state.blogs,key={"blog:${it.path}"},span={GridItemSpan(maxLineSpan)},contentType={"blog"}) { post ->
                            ListItem(headlineContent={Text(post.name)},supportingContent={Text(photoDateLabel(post.date ?: post.modified,locale))},
                                modifier=Modifier.clickable {controller.openBlog(post)})
                        }
                        if(state.blogHasNext) item(key="more-blogs",span={GridItemSpan(maxLineSpan)}) { LoadSection("blogs",state,controller) }
                    }
                    state.media.forEachIndexed { index,photo ->
                        val day=photo.date.take(10)
                        if(index==0 || state.media[index-1].date.take(10)!=day) item(key="date:${photo.path}",span={GridItemSpan(maxLineSpan)},contentType="date") {
                            SectionTitle(photoDateLabel(photo.date,locale))
                        }
                        item(key="photo:${photo.path}",span={GridItemSpan(2)},contentType="photo") {
                            PhotoThumbnail(photo,controller,images,controller.session.thumbnailSize,
                                Modifier.fillMaxWidth().aspectRatio(1f).clickable {controller.select(photo.path)})
                        }
                    }
                    if(state.hasNext) item(key="more-media",span={GridItemSpan(maxLineSpan)}) { LoadSection("media",state,controller) }
                    if(!state.loading && state.media.isEmpty() && state.folders.isEmpty() && state.blogs.isEmpty() && state.error==null) {
                        item(span={GridItemSpan(maxLineSpan)}) {
                            Column(Modifier.fillMaxWidth().padding(40.dp),horizontalAlignment=Alignment.CenterHorizontally,verticalArrangement=Arrangement.spacedBy(16.dp)) {
                                Icon(painterResource(R.drawable.ic_photos),null,Modifier.size(48.dp),tint=MaterialTheme.colorScheme.outline)
                                Text(stringResource(if(state.query.query.isBlank()) R.string.photos_empty else R.string.photos_no_results))
                            }
                        }
                    }
                }
            }
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

@Composable
private fun LoadSection(section: String, state: PhotosState, controller: PhotosController) {
    val count=when(section) { "media" -> state.media.size; "folders" -> state.folders.size; else -> state.blogs.size }
    LaunchedEffect(section,count) { controller.more(section) }
    Box(Modifier.fillMaxWidth().padding(12.dp),contentAlignment=Alignment.Center) {
        if(section in state.loadingSections) CircularProgressIndicator(Modifier.size(24.dp))
        else TextButton(onClick={controller.more(section)}) {Text(stringResource(R.string.photos_more))}
    }
}
@Composable private fun SectionTitle(title: String) {
    Text(title,Modifier.padding(horizontal=16.dp,vertical=14.dp),style=MaterialTheme.typography.titleMedium)
}
@Composable private fun FolderTile(folder: PhotoFolder, controller: PhotosController, images: ImageLoader, onClick: () -> Unit) {
    Column(Modifier.padding(6.dp).clip(RoundedCornerShape(20.dp)).clickable(onClick=onClick)
        .background(MaterialTheme.colorScheme.surfaceContainerLow)) {
        Row(Modifier.fillMaxWidth().height(104.dp),horizontalArrangement=Arrangement.spacedBy(2.dp)) {
            repeat(2) { index ->
                val photo=folder.previews.getOrNull(index)
                if(photo!=null) PhotoThumbnail(photo,controller,images,controller.session.folderThumbnailSize,Modifier.weight(1f).fillMaxHeight())
                else Box(Modifier.weight(1f).fillMaxHeight().background(MaterialTheme.colorScheme.surfaceContainerHighest),contentAlignment=Alignment.Center) {
                    Icon(painterResource(R.drawable.ic_folder),null,tint=MaterialTheme.colorScheme.outline)
                }
            }
        }
        Column(Modifier.padding(12.dp),verticalArrangement=Arrangement.spacedBy(4.dp)) {
            Text(folder.name,style=MaterialTheme.typography.titleSmall,maxLines=2,overflow=TextOverflow.Ellipsis)
            Text(pluralStringResource(if(folder.approximate) R.plurals.photos_count_approximate else R.plurals.photos_count,folder.count,folder.count),
                style=MaterialTheme.typography.bodySmall,color=MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
@Composable internal fun PhotoThumbnail(photo: Photo, controller: PhotosController, images: ImageLoader, size: Int, modifier: Modifier) {
    val context=LocalContext.current
    val request=remember(photo.path,photo.version,size,controller) { ImageRequest.Builder(context).data(controller.service.thumbnail(photo,size)).size(size).build() }
    AsyncImage(request,photo.name,imageLoader=images,contentScale=ContentScale.Crop,
        modifier=modifier.background(MaterialTheme.colorScheme.surfaceContainerHighest))
}
internal fun photoDateLabel(date: String, locale: Locale): String = runCatching {
    OffsetDateTime.parse(date).toLocalDate().format(DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL).withLocale(locale))
}.getOrDefault(date.take(10))
